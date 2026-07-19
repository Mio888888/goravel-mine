package main

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateEnvironmentRequiresTestingMode(t *testing.T) {
	t.Setenv("APP_ENV", "local")
	t.Setenv("SSO_TEST_ALLOW_LOOPBACK", "true")

	_, _, _, err := validateEnvironment(
		"http://127.0.0.1:19090",
		"http://127.0.0.1:2889/#/login",
		"default.localhost",
	)

	require.ErrorContains(t, err, "APP_ENV=testing")
}

func TestValidateEnvironmentRejectsUnsafeEndpoints(t *testing.T) {
	t.Setenv("APP_ENV", "testing")
	t.Setenv("SSO_TEST_ALLOW_LOOPBACK", "true")

	_, _, _, err := validateEnvironment(
		"https://idp.example.test",
		"http://127.0.0.1:2889/#/login",
		"default.localhost",
	)
	require.ErrorContains(t, err, "loopback HTTP")

	_, _, _, err = validateEnvironment(
		"http://127.0.0.1:19090",
		"http://127.0.0.1:2889/#/login",
		"tenant.example.test",
	)
	require.ErrorContains(t, err, "localhost")
}

func TestParseOptionsRejectsEmptyRequiredValues(t *testing.T) {
	_, err := parseOptions([]string{"--provider=", "--client-id=client"}, io.Discard)
	require.ErrorContains(t, err, "provider and client-id are required")
}
