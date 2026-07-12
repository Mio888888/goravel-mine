package feature

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/tests"
)

type HealthTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestHealthTestSuite(t *testing.T) {
	suite.Run(t, new(HealthTestSuite))
}

func (s *HealthTestSuite) TestLivenessProbeReturnsOkWithoutDependencyChecks() {
	res, err := s.Http(s.T()).Get("/health/live")
	require.NoError(s.T(), err)

	res.AssertOk()
	var body map[string]any
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), "ok", body["status"])
	require.Equal(s.T(), "live", body["check"])
}

func (s *HealthTestSuite) TestReadinessProbeReturnsDependencyStatus() {
	res, err := s.Http(s.T()).Get("/health/ready")
	require.NoError(s.T(), err)

	res.AssertOk()
	var body map[string]any
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), "ok", body["status"])
	require.Equal(s.T(), "ready", body["check"])
	require.Contains(s.T(), body, "dependencies")
}
