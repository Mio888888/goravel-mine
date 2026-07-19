package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"goravel/app/services"
	"goravel/bootstrap"
)

type options struct {
	tenant      string
	provider    string
	issuer      string
	redirectURI string
	host        string
	clientID    string
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string, stdout, stderr io.Writer) error {
	values, err := parseOptions(arguments, stderr)
	if err != nil {
		return err
	}
	issuer, redirectURI, host, err := validateEnvironment(values.issuer, values.redirectURI, values.host)
	if err != nil {
		return err
	}

	_ = bootstrap.Boot()
	tenant, err := services.NewTenantService().Resolve(values.tenant)
	if err != nil {
		return err
	}
	service := services.NewSSOProviderServiceForTenant(tenant)
	payload := services.SSOProviderPayload{
		Name:                  strings.TrimSpace(values.provider),
		DisplayName:           "OIDC E2E",
		Scene:                 "admin",
		Type:                  "oidc",
		Issuer:                issuer,
		DiscoveryURL:          issuer + "/.well-known/openid-configuration",
		AuthorizationEndpoint: issuer + "/authorize",
		TokenEndpoint:         issuer + "/token",
		JWKSURI:               issuer + "/jwks",
		ClientID:              values.clientID,
		Scope:                 "openid profile email",
		RedirectURI:           redirectURI,
		EnablePKCE:            boolPointer(true),
		EnableNonce:           boolPointer(true),
		AutoCreate:            boolPointer(true),
	}
	listed, err := service.List(map[string]string{"name": payload.Name, "scene": "admin"}, 1, 1)
	if err != nil {
		return err
	}
	if len(listed.List) == 0 {
		_, err = service.Create(payload, 1)
	} else {
		_, err = service.Update(listed.List[0].ID, payload, 1)
	}
	if err != nil {
		return err
	}
	if _, err := services.OrmForConnectionWithContext(context.Background(), services.PlatformConnection()).
		Query().Table("tenant").Where("id", tenant.ID).
		Update(map[string]any{"custom_domain": host, "updated_at": time.Now()}); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "OIDC test fixture ready")
	return err
}

func parseOptions(arguments []string, stderr io.Writer) (options, error) {
	values := options{}
	flags := flag.NewFlagSet("sso-fixture", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&values.tenant, "tenant", "default", "tenant code")
	flags.StringVar(&values.provider, "provider", "oidc-e2e", "provider name")
	flags.StringVar(&values.issuer, "issuer", "", "loopback test IdP issuer")
	flags.StringVar(&values.redirectURI, "redirect-uri", "", "browser callback URI")
	flags.StringVar(&values.host, "host", "default.localhost", "loopback tenant host")
	flags.StringVar(&values.clientID, "client-id", "goravel-oidc-e2e", "OIDC client ID")
	if err := flags.Parse(arguments); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if strings.TrimSpace(values.provider) == "" || strings.TrimSpace(values.clientID) == "" {
		return options{}, errors.New("provider and client-id are required")
	}
	return values, nil
}

func validateEnvironment(issuerValue, redirectURI, hostValue string) (string, string, string, error) {
	if os.Getenv("APP_ENV") != "testing" || os.Getenv("SSO_TEST_ALLOW_LOOPBACK") != "true" {
		return "", "", "", errors.New("sso-fixture requires APP_ENV=testing and SSO_TEST_ALLOW_LOOPBACK=true")
	}
	issuer := strings.TrimRight(strings.TrimSpace(issuerValue), "/")
	redirectURI = strings.TrimSpace(redirectURI)
	if !strings.HasPrefix(issuer, "http://127.0.0.1:") || redirectURI == "" {
		return "", "", "", errors.New("issuer must be loopback HTTP and redirect-uri is required")
	}
	host := services.TenantHostCode(hostValue)
	if host != "localhost" && !strings.HasSuffix(host, ".localhost") {
		return "", "", "", errors.New("host must be localhost or a localhost subdomain")
	}
	return issuer, redirectURI, host, nil
}

func boolPointer(value bool) *bool {
	return &value
}
