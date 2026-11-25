package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/adminjwt"
)

func main() {
	subject := flag.String("subject", "", "administrator identity recorded in audit logs")
	ttl := flag.Duration("ttl", time.Hour, "token lifetime (maximum 24h)")
	rolesArg := flag.String("roles", adminjwt.RoleAdmin, "comma-separated roles: admin,survey_manager,response_reader,fulfillment_manager,audit_reader")
	previous := flag.Bool("use-previous", false, "sign with JWT_SECRET_PREVIOUS")
	generate := flag.Bool("generate-secret", false, "print a new random 256-bit hex JWT secret")
	flag.Parse()
	if *generate {
		secret, err := adminjwt.GenerateSecret()
		fatalIf(err)
		fmt.Println(secret)
		return
	}
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" && !*previous {
		secret = strings.TrimSpace(os.Getenv("JWT_SCREET"))
	}
	if *previous {
		secret = strings.TrimSpace(os.Getenv("JWT_SECRET_PREVIOUS"))
	}
	if !adminjwt.KeyConfigured(secret) {
		fatalIf(fmt.Errorf("selected JWT signing key must be at least 32 bytes"))
	}
	issuer := env("JWT_ISSUER", "web-pinguin")
	audience := env("JWT_AUDIENCE", "web-pinguin-admin")
	roles := strings.Split(*rolesArg, ",")
	token, err := adminjwt.IssueForRoles(adminjwt.Config{Secret: secret, Issuer: issuer, Audience: audience}, *subject, roles, *ttl, time.Now())
	fatalIf(err)
	fmt.Println(token)
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
