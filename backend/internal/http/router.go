package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/adminjwt"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/analytics"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/catalog"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/listing"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/mirror"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/survey"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	Listing        *listing.Service
	Catalog        *catalog.Store
	Analytics      *analytics.Store
	Pool           *pgxpool.Pool
	AllowedOrigins []string
	Logger         *slog.Logger
	Mirrors        []mirror.Target
	Surveys        *survey.Store
	Worker         *analytics.Worker
	AdminJWT       adminjwt.Config
}

func New(options Options) (*gin.Engine, error) {
	if options.Listing == nil || options.Catalog == nil || options.Analytics == nil || options.Pool == nil || options.Surveys == nil {
		return nil, fmt.Errorf("listing, catalog, analytics, and database dependencies are required")
	}
	if err := options.Pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("database is not ready: %w", err)
	}

	engine := gin.New()
	engine.Use(gin.Recovery(), requestID(), cors(options.AllowedOrigins))
	engine.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	engine.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := options.Pool.Ping(ctx); err != nil {
			writeError(c, http.StatusServiceUnavailable, "dependency_unavailable", "API dependency is unavailable")
			return
		}
		if err := options.Listing.Check(); err != nil {
			writeError(c, http.StatusServiceUnavailable, "listing_unavailable", "listing directory is unavailable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := engine.Group("/api/v1")
	api.GET("/repositories", func(c *gin.Context) {
		page, pageSize, err := parsePagination(c.Query("page"), c.Query("page_size"))
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_query", "page must be positive and page_size must be between 1 and 100")
			return
		}
		category := strings.TrimSpace(c.Query("category"))
		if category != "" && category != "iso" && category != "package" && category != "tool" {
			writeError(c, http.StatusBadRequest, "invalid_query", "category must be one of iso, package, or tool")
			return
		}
		sortBy := c.DefaultQuery("sort", "name")
		if sortBy != "name" && sortBy != "updated" {
			writeError(c, http.StatusBadRequest, "invalid_query", "sort must be one of name or updated")
			return
		}
		order := c.DefaultQuery("order", "asc")
		if order != "asc" && order != "desc" {
			writeError(c, http.StatusBadRequest, "invalid_query", "order must be asc or desc")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		items, total, err := options.Catalog.List(ctx, catalog.Filters{
			Query: c.Query("q"), Category: category, Distribution: c.Query("distribution"),
			Sort: sortBy, Order: order, Page: page, PageSize: pageSize,
		})
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "could not load repositories")
			return
		}
		pages := (total + pageSize - 1) / pageSize
		c.JSON(http.StatusOK, gin.H{"data": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total, "total_pages": pages}})
	})
	api.GET("/repositories/:id", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		item, err := options.Catalog.Detail(ctx, c.Param("id"))
		if err != nil {
			if errors.Is(err, catalog.ErrNotFound) {
				writeError(c, http.StatusNotFound, "not_found", "repository was not found")
				return
			}
			writeError(c, http.StatusInternalServerError, "internal_error", "could not load repository")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": item})
	})
	api.GET("/listing", func(c *gin.Context) {
		page, pageSize, err := parsePagination(c.Query("page"), c.Query("page_size"))
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_query", "page must be positive and page_size must be between 1 and 100")
			return
		}
		entries, err := options.Listing.List(c.Query("path"))
		if err != nil {
			writeListingError(c, err)
			return
		}
		total := len(entries)
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		pages := (total + pageSize - 1) / pageSize
		c.JSON(http.StatusOK, gin.H{
			"data":       gin.H{"path": c.Query("path"), "entries": entries[start:end]},
			"pagination": gin.H{"page": page, "page_size": pageSize, "total": total, "total_pages": pages},
		})
	})
	api.GET("/download", func(c *gin.Context) {
		relativePath := c.Query("path")
		request, bytesSent, fileSize, err := options.Listing.ServeDownload(c.Writer, c.Request, relativePath)
		if err != nil {
			writeListingError(c, err)
			return
		}

		status := c.Writer.Status()
		completed := (status == http.StatusOK || status == http.StatusPartialContent) && fileSize > 0 && bytesSent == fileSize
		event := analytics.DownloadEvent{
			ID:                newID(),
			OccurredAt:        time.Now().UTC(),
			FilePath:          request.Path,
			HTTPStatus:        status,
			BytesSent:         bytesSent,
			RangeRequested:    request.RangeRequested,
			CompletedEstimate: completed,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := options.Analytics.RecordDownload(ctx, event); err != nil && options.Logger != nil {
			options.Logger.Error("could not record download event", "request_id", requestIDFrom(c), "error", err)
		}
	})

	api.GET("/stats/summary", func(c *gin.Context) {
		period, duration, err := parsePeriod(c.Query("period"))
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_query", "period must be one of 24h, 7d, or 30d")
			return
		}
		filter := analytics.StatsFilter{RepositoryID: strings.TrimSpace(c.Query("repository")), Distribution: strings.TrimSpace(c.Query("distribution"))}
		if !validStatsRepositoryID(filter.RepositoryID) {
			writeError(c, http.StatusBadRequest, "invalid_query", "repository must be a UUID")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		result, err := options.Analytics.Summary(ctx, period, time.Now().UTC().Add(-duration), filter)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "could not load statistics")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": result})
	})
	api.GET("/stats/timeseries", func(c *gin.Context) {
		period, duration, err := parsePeriod(c.Query("period"))
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_query", "period must be one of 24h, 7d, or 30d")
			return
		}
		filter := analytics.StatsFilter{RepositoryID: strings.TrimSpace(c.Query("repository")), Distribution: strings.TrimSpace(c.Query("distribution"))}
		if !validStatsRepositoryID(filter.RepositoryID) {
			writeError(c, http.StatusBadRequest, "invalid_query", "repository must be a UUID")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		points, err := options.Analytics.Timeseries(ctx, period, time.Now().UTC().Add(-duration), filter)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "could not load statistics")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": points})
	})
	api.GET("/stats/regions", func(c *gin.Context) {
		period, duration, err := parsePeriod(c.Query("period"))
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_query", "period must be one of 24h, 7d, or 30d")
			return
		}
		filter := analytics.StatsFilter{RepositoryID: strings.TrimSpace(c.Query("repository")), Distribution: strings.TrimSpace(c.Query("distribution"))}
		if !validStatsRepositoryID(filter.RepositoryID) {
			writeError(c, http.StatusBadRequest, "invalid_query", "repository must be a UUID")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		regions, err := options.Analytics.Regions(ctx, period, time.Now().UTC().Add(-duration), filter)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "could not load regional statistics")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": regions})
	})
	api.GET("/mirrors/status", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		c.JSON(http.StatusOK, gin.H{"data": mirror.Check(ctx, options.Mirrors)})
	})
	adminStatus := api.Group("/admin")
	adminStatus.Use(adminAuth(options.AdminJWT))
	adminStatus.GET("/workers/analytics", requireRoles(adminjwt.RoleAdmin), func(c *gin.Context) {
		if options.Worker == nil {
			writeError(c, http.StatusServiceUnavailable, "worker_status_unavailable", "worker status is unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		status, err := options.Worker.Status(ctx)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "could not load worker status")
			return
		}
		health := "ok"
		if status.ConsecutiveFailures > 0 {
			health = "degraded"
		}
		if status.LastSucceededAt == nil || time.Since(*status.LastSucceededAt) > 5*time.Minute {
			health = "stale"
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"health": health, "status": status}})
	})
	api.GET("/surveys/active", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		result, err := options.Surveys.Active(ctx)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "could not load active survey")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": result})
	})
	api.POST("/surveys/:id/responses", func(c *gin.Context) {
		key := c.GetHeader("Idempotency-Key")
		if len(key) < 16 || len(key) > 128 {
			writeError(c, http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key must contain 16 to 128 characters")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
		var submission survey.Submission
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&submission); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_body", "request body is invalid")
			return
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			writeError(c, http.StatusBadRequest, "invalid_body", "request body must contain one JSON value")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		receipt, err := options.Surveys.Submit(ctx, c.Param("id"), key, submission)
		if err != nil {
			switch {
			case errors.Is(err, survey.ErrInvalid):
				writeError(c, http.StatusUnprocessableEntity, "invalid_response", "response does not match the active survey")
			case errors.Is(err, survey.ErrNotFound):
				writeError(c, http.StatusNotFound, "not_found", "active survey was not found")
			case errors.Is(err, survey.ErrConflict):
				writeError(c, http.StatusConflict, "idempotency_conflict", "Idempotency-Key was already used with a different response")
			default:
				writeError(c, http.StatusInternalServerError, "internal_error", "could not submit survey response")
			}
			return
		}
		c.JSON(http.StatusCreated, gin.H{"data": receipt})
	})
	admin := api.Group("/admin")
	admin.Use(adminAuth(options.AdminJWT))
	admin.GET("/surveys", requireRoles(adminjwt.RoleSurveyManager, adminjwt.RoleResponseReader, adminjwt.RoleFulfillmentManager), func(c *gin.Context) {
		page, pageSize, err := parsePagination(c.Query("page"), c.Query("page_size"))
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_query", "invalid pagination")
			return
		}
		items, total, err := options.Surveys.AdminList(c.Request.Context(), c.Query("status"), page, pageSize)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "could not list surveys")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total, "total_pages": (total + pageSize - 1) / pageSize}})
	})
	admin.POST("/surveys", requireRoles(adminjwt.RoleSurveyManager), func(c *gin.Context) {
		var input survey.DraftInput
		if err := decodeJSON(c, &input); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_body", "request body is invalid")
			return
		}
		item, err := options.Surveys.CreateDraft(c.Request.Context(), input)
		if err != nil {
			writeAdminSurveyError(c, err)
			return
		}
		auditAdmin(options, c, "create_survey", "survey", item.ID, map[string]any{"status": item.Status})
		c.JSON(http.StatusCreated, gin.H{"data": item})
	})
	admin.GET("/surveys/:id", requireRoles(adminjwt.RoleSurveyManager, adminjwt.RoleResponseReader, adminjwt.RoleFulfillmentManager), func(c *gin.Context) {
		item, err := options.Surveys.AdminDetail(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeAdminSurveyError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": item})
	})
	admin.PATCH("/surveys/:id", requireRoles(adminjwt.RoleSurveyManager), func(c *gin.Context) {
		var input survey.DraftInput
		if err := decodeJSON(c, &input); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_body", "request body is invalid")
			return
		}
		item, err := options.Surveys.UpdateDraft(c.Request.Context(), c.Param("id"), input)
		if err != nil {
			writeAdminSurveyError(c, err)
			return
		}
		auditAdmin(options, c, "update_survey", "survey", item.ID, map[string]any{"status": item.Status})
		c.JSON(http.StatusOK, gin.H{"data": item})
	})
	admin.POST("/surveys/:id/:action", requireRoles(adminjwt.RoleSurveyManager), func(c *gin.Context) {
		action := c.Param("action")
		if action != "publish" && action != "close" && action != "archive" {
			c.Status(http.StatusNotFound)
			return
		}
		item, err := options.Surveys.Transition(c.Request.Context(), c.Param("id"), action)
		if err != nil {
			writeAdminSurveyError(c, err)
			return
		}
		auditAdmin(options, c, action+"_survey", "survey", item.ID, map[string]any{"status": item.Status})
		c.JSON(http.StatusOK, gin.H{"data": item})
	})
	admin.GET("/surveys/:id/results", requireRoles(adminjwt.RoleSurveyManager, adminjwt.RoleResponseReader), func(c *gin.Context) {
		result, err := options.Surveys.Results(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeAdminSurveyError(c, err)
			return
		}
		if err := auditAdmin(options, c, "read_survey_results", "survey", c.Param("id"), nil); err != nil {
			writeError(c, http.StatusInternalServerError, "audit_unavailable", "could not record access to survey results")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": result})
	})
	admin.GET("/surveys/:id/responses", requireRoles(adminjwt.RoleResponseReader), func(c *gin.Context) {
		page, pageSize, err := parsePagination(c.Query("page"), c.Query("page_size"))
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_query", "invalid pagination")
			return
		}
		items, total, err := options.Surveys.Responses(c.Request.Context(), c.Param("id"), page, pageSize)
		if err != nil {
			writeAdminSurveyError(c, err)
			return
		}
		if err := auditAdmin(options, c, "read_survey_responses", "survey", c.Param("id"), map[string]any{"page": page, "page_size": pageSize}); err != nil {
			writeError(c, http.StatusInternalServerError, "audit_unavailable", "could not record access to survey responses")
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total, "total_pages": (total + pageSize - 1) / pageSize}})
	})
	admin.PATCH("/survey-options/:id/fulfillment", requireRoles(adminjwt.RoleFulfillmentManager), func(c *gin.Context) {
		var input survey.FulfillmentInput
		if err := decodeJSON(c, &input); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_body", "request body is invalid")
			return
		}
		item, err := options.Surveys.UpdateFulfillment(c.Request.Context(), c.Param("id"), input, adminActor(c))
		if err != nil {
			writeAdminSurveyError(c, err)
			return
		}
		auditAdmin(options, c, "update_fulfillment", "survey_option", c.Param("id"), map[string]any{"status": input.Status, "priority": input.Priority})
		c.JSON(http.StatusOK, gin.H{"data": item})
	})
	admin.GET("/audit-logs", requireRoles(adminjwt.RoleAuditReader), func(c *gin.Context) {
		page, pageSize, err := parsePagination(c.Query("page"), c.Query("page_size"))
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_query", "invalid pagination")
			return
		}
		items, total, err := options.Surveys.AuditList(c.Request.Context(), c.Query("actor_id"), c.Query("action"), c.Query("resource_id"), c.Query("from"), c.Query("to"), page, pageSize)
		if err != nil {
			writeAdminSurveyError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": items, "pagination": gin.H{"page": page, "page_size": pageSize, "total": total, "total_pages": (total + pageSize - 1) / pageSize}})
	})
	return engine, nil
}

func adminAuth(cfg adminjwt.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !adminjwt.KeyConfigured(cfg.Secret) {
			writeError(c, http.StatusServiceUnavailable, "admin_disabled", "admin API is disabled until JWT_SECRET is configured with at least 32 bytes")
			return
		}
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(c, http.StatusUnauthorized, "unauthorized", "valid bearer token required")
			return
		}
		claims, err := adminjwt.Validate(cfg, strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")), time.Now())
		if err != nil {
			writeError(c, http.StatusUnauthorized, "unauthorized", "valid administrator JWT required")
			return
		}
		c.Set("admin_actor", claims.Subject)
		c.Set("admin_roles", claims.Roles)
		c.Next()
	}
}

func requireRoles(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		value, ok := c.Get("admin_roles")
		roles, _ := value.([]string)
		claims := adminjwt.Claims{Role: "", Roles: roles}
		if !ok || !claims.HasAnyRole(append(allowed, adminjwt.RoleAdmin)...) {
			writeError(c, http.StatusForbidden, "forbidden", "administrator role does not allow this operation")
			return
		}
		c.Next()
	}
}

func adminActor(c *gin.Context) string {
	value, ok := c.Get("admin_actor")
	if !ok {
		return "admin:unknown"
	}
	actor, _ := value.(string)
	return actor
}

func auditAdmin(options Options, c *gin.Context, action, resourceType, resourceID string, changes any) error {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	err := options.Surveys.Audit(ctx, adminActor(c), action, resourceType, resourceID, requestIDFrom(c), changes)
	if err != nil && options.Logger != nil {
		options.Logger.Error("admin audit write failed", "action", action, "request_id", requestIDFrom(c), "error", err)
	}
	return err
}

func decodeJSON(c *gin.Context, target any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}

func writeAdminSurveyError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, survey.ErrNotFound):
		writeError(c, http.StatusNotFound, "not_found", "survey was not found")
	case errors.Is(err, survey.ErrConflict):
		writeError(c, http.StatusConflict, "conflict", "survey cannot be changed in its current state")
	case errors.Is(err, survey.ErrInvalid):
		writeError(c, http.StatusUnprocessableEntity, "invalid_survey", "survey data is invalid")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "could not process survey")
	}
}

func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if len(id) > 80 || strings.ContainsAny(id, "\r\n") {
			id = ""
		}
		if id == "" {
			id = newID()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func cors(origins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		allowed[strings.TrimRight(origin, "/")] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := strings.TrimRight(c.GetHeader("Origin"), "/")
		if _, ok := allowed[origin]; ok && origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, HEAD, POST, PATCH, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Range, If-Range, If-Modified-Since, If-None-Match, X-Request-ID, Idempotency-Key")
			c.Header("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges, Last-Modified, ETag, X-Request-ID")
		}
		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		c.Next()
	}
}

func writeListingError(c *gin.Context, err error) {
	status := listing.DownloadErrorStatus(err)
	code, message := "internal_error", "could not access listing"
	switch {
	case errors.Is(err, listing.ErrInvalidPath):
		code, message = "invalid_path", "path is invalid"
	case errors.Is(err, listing.ErrNotFound):
		code, message = "not_found", "listing item was not found"
	case errors.Is(err, listing.ErrNotFile):
		code, message = "not_a_file", "path does not identify a regular file"
	}
	writeError(c, status, code, message)
}

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{
		"code":       code,
		"message":    message,
		"request_id": requestIDFrom(c),
	}})
}

func requestIDFrom(c *gin.Context) string {
	value, _ := c.Get("request_id")
	id, _ := value.(string)
	return id
}

func parsePeriod(value string) (string, time.Duration, error) {
	if value == "" {
		value = "30d"
	}
	switch value {
	case "24h":
		return value, 24 * time.Hour, nil
	case "7d":
		return value, 7 * 24 * time.Hour, nil
	case "30d":
		return value, 30 * 24 * time.Hour, nil
	default:
		return "", 0, errors.New("unsupported period")
	}
}

func validStatsRepositoryID(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded, err := hex.DecodeString(compact)
	return err == nil && len(decoded) == 16
}

func parsePagination(pageValue, pageSizeValue string) (int, int, error) {
	page, pageSize := 1, 20
	if pageValue != "" {
		parsed, err := strconv.Atoi(pageValue)
		if err != nil || parsed < 1 {
			return 0, 0, errors.New("invalid page")
		}
		page = parsed
	}
	if pageSizeValue != "" {
		parsed, err := strconv.Atoi(pageSizeValue)
		if err != nil || parsed < 1 || parsed > 100 {
			return 0, 0, errors.New("invalid page size")
		}
		pageSize = parsed
	}
	if page > 1_000_000 {
		return 0, 0, errors.New("page is too large")
	}
	return page, pageSize, nil
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}
