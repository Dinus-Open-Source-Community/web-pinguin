package main

import (
	"context"
	"errors"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/adminjwt"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/analytics"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/catalog"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/http"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/listing"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/platform/config"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/platform/postgres"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/survey"
	"github.com/gin-gonic/gin"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("API stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	settings, err := config.Load()
	if err != nil {
		return err
	}
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	listingService, err := listing.New(settings.ListingDir)
	if err != nil {
		return err
	}
	defer listingService.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dbContext, cancelDB := context.WithTimeout(ctx, time.Duration(settings.DatabaseTimeoutSecs)*time.Second)
	pool, err := postgres.Open(dbContext, settings.Database)
	cancelDB()
	if err != nil {
		return err
	}
	defer pool.Close()

	engine, err := httpapi.New(httpapi.Options{
		Listing:        listingService,
		Catalog:        catalog.NewStore(pool),
		Analytics:      analytics.NewStore(pool),
		Pool:           pool,
		AllowedOrigins: settings.AllowedOrigins,
		Logger:         logger,
		Mirrors:        settings.Mirrors,
		Surveys:        survey.NewStore(pool),
		Worker:         analytics.NewWorker(pool),
		AdminJWT:       adminjwt.Config{Secret: settings.JWTSecret, PreviousSecret: settings.JWTPreviousSecret, Issuer: settings.JWTIssuer, Audience: settings.JWTAudience},
	})
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              settings.Address,
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	serverError := make(chan error, 1)
	go func() {
		logger.Info("API listening", "address", settings.Address, "listing_dir", settings.ListingDir)
		serverError <- server.ListenAndServe()
	}()

	select {
	case err := <-serverError:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownContext)
	}
}
