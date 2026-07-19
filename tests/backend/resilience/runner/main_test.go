package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func validConfig() Config {
	scenarios := make([]Scenario, 0, len(requiredScenarios))
	for _, name := range requiredScenarios {
		scenarios = append(scenarios, Scenario{Name: name, Profiles: []string{"quick"}, Action: "act", Assertions: "assert", Cleanup: "clean", Timeout: "1m", Thresholds: map[string]float64{"error_rate": 0.005}, Evidence: []string{"evidence.json"}})
	}
	return Config{Schema: "resilience-scenarios/v1", Scenarios: scenarios}
}

func TestValidateConfigRejectsMissingScenarioThresholdAndCleanup(t *testing.T) {
	config := validConfig()
	config.Scenarios = config.Scenarios[1:]
	require.ErrorContains(t, validateConfig(config), "missing required scenario")

	config = validConfig()
	config.Scenarios[0].Thresholds = nil
	require.ErrorContains(t, validateConfig(config), "requires thresholds")

	config = validConfig()
	config.Scenarios[0].Cleanup = ""
	require.ErrorContains(t, validateConfig(config), "requires cleanup")
}

func TestRunScenarioDoesNotMaskCommandOrCleanupFailure(t *testing.T) {
	scenario := validConfig().Scenarios[0]
	calls := 0
	runner := func(context.Context, string) error {
		calls++
		if calls == 2 {
			return errors.New("action failed")
		}
		if calls == 3 {
			return errors.New("cleanup failed")
		}
		return nil
	}
	result := runScenario(scenario, runner)
	require.Equal(t, "failed", result.Status)
	require.Contains(t, result.Error, "action failed")
	require.Contains(t, result.Error, "cleanup failed")
}

func TestRunScenarioRejectsStaleEvidence(t *testing.T) {
	dir := t.TempDir()
	evidence := filepath.Join(dir, "evidence.json")
	require.NoError(t, os.WriteFile(evidence, []byte("{}"), 0o644))
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(evidence, old, old))
	scenario := validConfig().Scenarios[0]
	scenario.Evidence = []string{evidence}
	result := runScenario(scenario, func(context.Context, string) error { return nil })
	require.Equal(t, "failed", result.Status)
	require.Contains(t, result.Error, "stale evidence")
}
