package config

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/mirror"
)

type Database struct {
	Host     string
	Port     uint16
	Name     string
	User     string
	Password string
	SSLMode  string
}

type Config struct {
	Address             string
	ListingDir          string
	AllowedOrigins      []string
	Database            Database
	DatabaseTimeoutSecs int
	Mirrors             []mirror.Target
	JWTSecret           string
	JWTPreviousSecret   string
	JWTIssuer           string
	JWTAudience         string
}

func Load() (Config, error) {
	database, err := LoadDatabase()
	if err != nil {
		return Config{}, err
	}

	listingDir := strings.TrimSpace(os.Getenv("LISTING_DIR"))
	if listingDir == "" {
		return Config{}, fmt.Errorf("LISTING_DIR is required")
	}

	timeout, err := intEnv("DB_TIMEOUT_SECONDS", 5)
	if err != nil || timeout < 1 || timeout > 60 {
		return Config{}, fmt.Errorf("DB_TIMEOUT_SECONDS must be between 1 and 60")
	}

	return Config{
		Address:             env("API_ADDR", "127.0.0.1:8080"),
		ListingDir:          listingDir,
		AllowedOrigins:      csvEnv("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000", "http://127.0.0.1:3000"}),
		Database:            database,
		DatabaseTimeoutSecs: timeout,
		Mirrors:             loadMirrors(),
		JWTSecret:           firstEnv("JWT_SECRET", "JWT_SCREET"),
		JWTPreviousSecret:   strings.TrimSpace(os.Getenv("JWT_SECRET_PREVIOUS")),
		JWTIssuer:           env("JWT_ISSUER", "web-pinguin"),
		JWTAudience:         env("JWT_AUDIENCE", "web-pinguin-admin"),
	}, nil
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

// MIRROR_TARGETS is a comma-separated list of base64url(name)|base64url(https-url) pairs.
func loadMirrors() []mirror.Target {
	items := make([]mirror.Target, 0)
	for _, entry := range strings.Split(os.Getenv("MIRROR_TARGETS"), ",") {
		parts := strings.SplitN(strings.TrimSpace(entry), "|", 2)
		if len(parts) != 2 {
			continue
		}
		name, e1 := base64.RawURLEncoding.DecodeString(parts[0])
		target, e2 := base64.RawURLEncoding.DecodeString(parts[1])
		if e1 != nil || e2 != nil || len(strings.TrimSpace(string(name))) == 0 || !mirror.IsValidTargetURL(string(target)) {
			continue
		}
		items = append(items, mirror.Target{Name: strings.TrimSpace(string(name)), URL: string(target)})
	}
	return items
}

func LoadDatabase() (Database, error) {
	port, err := strconv.ParseUint(env("DB_PORT", "5432"), 10, 16)
	if err != nil || port == 0 {
		return Database{}, fmt.Errorf("DB_PORT must be between 1 and 65535")
	}

	user := strings.TrimSpace(os.Getenv("DB_USER"))
	password := os.Getenv("DB_PASSWORD")
	if user == "" || password == "" {
		return Database{}, fmt.Errorf("DB_USER and DB_PASSWORD are required")
	}

	sslMode := env("DB_SSLMODE", "disable")
	if sslMode != "disable" && sslMode != "require" && sslMode != "verify-ca" && sslMode != "verify-full" {
		return Database{}, fmt.Errorf("DB_SSLMODE has an unsupported value")
	}

	return Database{
		Host:     env("DB_HOST", "127.0.0.1"),
		Port:     uint16(port),
		Name:     env("DB_NAME", "pinguin_dinus"),
		User:     user,
		Password: password,
		SSLMode:  sslMode,
	}, nil
}

func (d Database) ConnectionString() string {
	query := url.Values{}
	query.Set("sslmode", d.SSLMode)
	endpoint := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(d.User, d.Password),
		Host:     net.JoinHostPort(d.Host, strconv.Itoa(int(d.Port))),
		Path:     "/" + d.Name,
		RawQuery: query.Encode(),
	}
	return endpoint.String()
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func intEnv(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}

func csvEnv(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	return result
}
