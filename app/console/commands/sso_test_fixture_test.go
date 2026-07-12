package commands

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSOTestFixtureRequiresTestingEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SSO_TEST_ALLOW_LOOPBACK", "true")
	_, _, _, err := validateSSOTestFixtureEnvironment("http://127.0.0.1:19090", "http://127.0.0.1:2889/#/login", "default.localhost")
	require.ErrorContains(t, err, "APP_ENV=testing")
}

func TestSSOTestFixtureRejectsNonLoopbackIssuer(t *testing.T) {
	t.Setenv("APP_ENV", "testing")
	t.Setenv("SSO_TEST_ALLOW_LOOPBACK", "true")
	_, _, _, err := validateSSOTestFixtureEnvironment("https://idp.example.test", "http://127.0.0.1:2889/#/login", "default.localhost")
	require.ErrorContains(t, err, "loopback")
}

func TestSSOTestFixtureRejectsNonLoopbackTenantHost(t *testing.T) {
	t.Setenv("APP_ENV", "testing")
	t.Setenv("SSO_TEST_ALLOW_LOOPBACK", "true")
	_, _, _, err := validateSSOTestFixtureEnvironment("http://127.0.0.1:19090", "http://127.0.0.1:2889/#/login", "tenant.example.test")
	require.ErrorContains(t, err, "localhost")
}
