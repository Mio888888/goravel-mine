package testsupport

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"

	"goravel/app/services"
)

type SSOTestFixtureCommand struct{}

func (r *SSOTestFixtureCommand) Signature() string { return "sso:test-fixture" }

func (r *SSOTestFixtureCommand) Description() string {
	return "Upsert a loopback OIDC provider for controlled E2E tests"
}

func (r *SSOTestFixtureCommand) Extend() command.Extend {
	return command.Extend{Category: "test", Flags: []command.Flag{
		&command.StringFlag{Name: "tenant", Value: "default"},
		&command.StringFlag{Name: "provider", Value: "oidc-e2e"},
		&command.StringFlag{Name: "issuer", Usage: "Loopback test IdP issuer"},
		&command.StringFlag{Name: "redirect-uri", Usage: "Browser callback URI"},
		&command.StringFlag{Name: "host", Value: "default.localhost", Usage: "Loopback tenant host"},
		&command.StringFlag{Name: "client-id", Value: "goravel-oidc-e2e"},
	}}
}

func (r *SSOTestFixtureCommand) Handle(ctx console.Context) error {
	issuer, redirectURI, host, err := validateSSOTestFixtureEnvironment(ctx.Option("issuer"), ctx.Option("redirect-uri"), ctx.Option("host"))
	if err != nil {
		return err
	}
	tenant, err := services.NewTenantService().Resolve(ctx.Option("tenant"))
	if err != nil {
		return err
	}
	service := services.NewSSOProviderServiceForTenant(tenant)
	providerName := strings.TrimSpace(ctx.Option("provider"))
	payload := services.SSOProviderPayload{
		Name: providerName, DisplayName: "OIDC E2E", Scene: "admin", Type: "oidc",
		Issuer: issuer, DiscoveryURL: issuer + "/.well-known/openid-configuration",
		AuthorizationEndpoint: issuer + "/authorize", TokenEndpoint: issuer + "/token",
		JWKSURI: issuer + "/jwks", ClientID: ctx.Option("client-id"), Scope: "openid profile email",
		RedirectURI: redirectURI, EnablePKCE: fixtureBool(true), EnableNonce: fixtureBool(true), AutoCreate: fixtureBool(true),
	}
	listed, err := service.List(map[string]string{"name": providerName, "scene": "admin"}, 1, 1)
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
	ctx.Success("OIDC test fixture ready")
	return nil
}

func validateSSOTestFixtureEnvironment(issuerValue, redirectURI, hostValue string) (string, string, string, error) {
	if os.Getenv("APP_ENV") != "testing" || os.Getenv("SSO_TEST_ALLOW_LOOPBACK") != "true" {
		return "", "", "", fmt.Errorf("sso:test-fixture requires APP_ENV=testing and SSO_TEST_ALLOW_LOOPBACK=true")
	}
	issuer := strings.TrimRight(strings.TrimSpace(issuerValue), "/")
	redirectURI = strings.TrimSpace(redirectURI)
	if !strings.HasPrefix(issuer, "http://127.0.0.1:") || redirectURI == "" {
		return "", "", "", fmt.Errorf("issuer must be loopback HTTP and redirect-uri is required")
	}
	host := services.TenantHostCode(hostValue)
	if host != "localhost" && !strings.HasSuffix(host, ".localhost") {
		return "", "", "", fmt.Errorf("host must be localhost or a localhost subdomain")
	}
	return issuer, redirectURI, host, nil
}

func fixtureBool(value bool) *bool { return &value }
