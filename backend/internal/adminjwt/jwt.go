package adminjwt

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	RoleAdmin              = "admin"
	RoleSurveyManager      = "survey_manager"
	RoleResponseReader     = "response_reader"
	RoleFulfillmentManager = "fulfillment_manager"
	RoleAuditReader        = "audit_reader"
	MaxTTL                 = 24 * time.Hour
)

type Claims struct {
	Role  string   `json:"role"`
	Roles []string `json:"roles,omitempty"`
	jwt.RegisteredClaims
}

type Config struct {
	Secret         string
	PreviousSecret string
	Issuer         string
	Audience       string
}

func KeyConfigured(secret string) bool { return len(strings.TrimSpace(secret)) >= 32 }

func GenerateSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func Issue(cfg Config, subject string, ttl time.Duration, now time.Time) (string, error) {
	return IssueForRoles(cfg, subject, []string{RoleAdmin}, ttl, now)
}

func IssueForRoles(cfg Config, subject string, roles []string, ttl time.Duration, now time.Time) (string, error) {
	if !KeyConfigured(cfg.Secret) {
		return "", errors.New("JWT secret must be at least 32 bytes")
	}
	if strings.TrimSpace(subject) == "" {
		return "", errors.New("subject is required")
	}
	if ttl <= 0 || ttl > MaxTTL {
		return "", fmt.Errorf("token lifetime must be between 1 second and %s", MaxTTL)
	}
	if cfg.Issuer == "" || cfg.Audience == "" {
		return "", errors.New("JWT issuer and audience are required")
	}
	roles, err := normalizeRoles(roles)
	if err != nil {
		return "", err
	}
	claims := Claims{Role: roles[0], Roles: roles, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: cfg.Issuer, Subject: subject, Audience: jwt.ClaimStrings{cfg.Audience},
		IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.Secret))
}

func Validate(cfg Config, raw string, now time.Time) (*Claims, error) {
	if !KeyConfigured(cfg.Secret) {
		return nil, errors.New("admin JWT is disabled")
	}
	keys := []string{cfg.Secret}
	if KeyConfigured(cfg.PreviousSecret) {
		keys = append(keys, cfg.PreviousSecret)
	}
	var lastErr error
	for _, key := range keys {
		claims := new(Claims)
		parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(cfg.Issuer), jwt.WithAudience(cfg.Audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithNotBeforeRequired(), jwt.WithLeeway(30*time.Second), jwt.WithTimeFunc(func() time.Time { return now }))
		token, err := parser.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(key), nil
		})
		if err != nil {
			lastErr = err
			continue
		}
		roles, roleErr := normalizeRoles(claims.effectiveRoles())
		if !token.Valid || claims.Issuer != cfg.Issuer || strings.TrimSpace(claims.Subject) == "" || roleErr != nil || claims.Role != roles[0] || claims.IssuedAt == nil || claims.ExpiresAt == nil || claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) > MaxTTL || !claims.ExpiresAt.Time.After(claims.IssuedAt.Time) {
			lastErr = errors.New("invalid administrator claims")
			continue
		}
		claims.Roles = roles
		return claims, nil
	}
	if lastErr == nil {
		lastErr = errors.New("invalid token")
	}
	return nil, lastErr
}

func (c Claims) effectiveRoles() []string {
	if len(c.Roles) > 0 {
		return c.Roles
	}
	if c.Role != "" {
		return []string{c.Role}
	}
	return nil
}

func (c Claims) HasAnyRole(allowed ...string) bool {
	for _, role := range c.effectiveRoles() {
		for _, candidate := range allowed {
			if role == candidate {
				return true
			}
		}
	}
	return false
}

func normalizeRoles(roles []string) ([]string, error) {
	valid := map[string]bool{RoleAdmin: true, RoleSurveyManager: true, RoleResponseReader: true, RoleFulfillmentManager: true, RoleAuditReader: true}
	if len(roles) == 0 {
		return nil, errors.New("at least one administrator role is required")
	}
	result := make([]string, 0, len(roles))
	seen := make(map[string]bool, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(role)
		if !valid[role] {
			return nil, fmt.Errorf("unsupported administrator role %q", role)
		}
		if !seen[role] {
			result = append(result, role)
			seen[role] = true
		}
	}
	return result, nil
}
