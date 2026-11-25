package adminjwt

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestIssueAndValidate(t *testing.T) {
	now := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	cfg := Config{Secret: "test-signing-secret-01234567890123456789", Issuer: "issuer", Audience: "audience"}
	token, err := Issue(cfg, "operator-7", time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := Validate(cfg, token, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "operator-7" || claims.Role != RoleAdmin {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}

func TestRoleBoundaries(t *testing.T) {
	now := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	cfg := Config{Secret: "test-signing-secret-01234567890123456789", Issuer: "issuer", Audience: "audience"}
	token, err := IssueForRoles(cfg, "operator-8", []string{RoleResponseReader, RoleAuditReader}, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := Validate(cfg, token, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !claims.HasAnyRole(RoleResponseReader) || !claims.HasAnyRole(RoleAuditReader) || claims.HasAnyRole(RoleSurveyManager) {
		t.Fatalf("role grants do not match token: %#v", claims.Roles)
	}
	if _, err := IssueForRoles(cfg, "operator-8", []string{"unknown"}, time.Hour, now); err == nil {
		t.Fatal("unsupported role was accepted")
	}
}

func TestPreviousSigningKeyRotationWindow(t *testing.T) {
	now := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	oldKey := "old-signing-secret-012345678901234567890"
	newKey := "new-signing-secret-012345678901234567890"
	oldConfig := Config{Secret: oldKey, Issuer: "issuer", Audience: "audience"}
	token, err := Issue(oldConfig, "operator-during-rotation", time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}

	rotated := Config{Secret: newKey, PreviousSecret: oldKey, Issuer: "issuer", Audience: "audience"}
	if _, err := Validate(rotated, token, now.Add(time.Minute)); err != nil {
		t.Fatalf("old signing key was rejected during the overlap window: %v", err)
	}
	rotated.PreviousSecret = ""
	if _, err := Validate(rotated, token, now.Add(time.Minute)); err == nil {
		t.Fatal("old signing key was accepted after the overlap window ended")
	}
}

func TestValidateRejectsInvalidTokens(t *testing.T) {
	now := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	cfg := Config{Secret: "test-signing-secret-01234567890123456789", Issuer: "issuer", Audience: "audience"}
	for name, makeToken := range map[string]func() string{
		"expired": func() string {
			return mustSign(t, jwt.SigningMethodHS256, cfg.Secret, Claims{Role: RoleAdmin, RegisteredClaims: jwt.RegisteredClaims{Issuer: cfg.Issuer, Subject: "operator", Audience: jwt.ClaimStrings{cfg.Audience}, IssuedAt: jwt.NewNumericDate(now.Add(-2 * time.Hour)), NotBefore: jwt.NewNumericDate(now.Add(-2 * time.Hour)), ExpiresAt: jwt.NewNumericDate(now.Add(-time.Hour))}})
		},
		"wrong audience": func() string {
			return mustSign(t, jwt.SigningMethodHS256, cfg.Secret, Claims{Role: RoleAdmin, RegisteredClaims: jwt.RegisteredClaims{Issuer: cfg.Issuer, Subject: "operator", Audience: jwt.ClaimStrings{"other"}, IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))}})
		},
		"wrong algorithm": func() string {
			return mustSign(t, jwt.SigningMethodHS384, cfg.Secret, Claims{Role: RoleAdmin, RegisteredClaims: jwt.RegisteredClaims{Issuer: cfg.Issuer, Subject: "operator", Audience: jwt.ClaimStrings{cfg.Audience}, IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Validate(cfg, makeToken(), now); err == nil {
				t.Fatal("expected invalid JWT to be rejected")
			}
		})
	}
}

func mustSign(t *testing.T, method jwt.SigningMethod, secret string, claims Claims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(method, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return token
}
