package unit

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/services"
)

func TestProtectionEngineChoosesMostSpecificRuleAndLimitsRequests(t *testing.T) {
	engine := services.NewProtectionEngine()
	require.NoError(t, engine.ReplaceRules([]services.PublishedProtectionRuleSet{
		{
			RuleSetID: 1, Version: 1, Scope: services.ProtectionScopeGlobal,
			ResourcePattern: "*",
			Rules:           []services.ProtectionRule{{Type: services.ProtectionRuleTypeRateLimit, Limit: 100, WindowMS: 1000}},
		},
		{
			RuleSetID: 2, Version: 1, Scope: services.ProtectionScopeEndpoint,
			ResourcePattern: "/orders/*",
			Rules:           []services.ProtectionRule{{Type: services.ProtectionRuleTypeRateLimit, Limit: 1, WindowMS: 1000}},
		},
	}))

	request := services.ProtectionRequestContext{Endpoint: "/orders/create", RateLimitKey: "tenant-1"}
	first := engine.Evaluate("/orders/create", request)
	require.True(t, first.Allowed)
	require.Equal(t, uint64(2), first.RuleSetID)

	second := engine.Evaluate("/orders/create", request)
	require.False(t, second.Allowed)
	require.Equal(t, services.ProtectionRejectionRateLimited, second.Rejection)
	require.ErrorIs(t, second.Error, services.ErrProtectionRateLimited)
}

func TestProtectionEngineEnforcesConcurrencyIsolation(t *testing.T) {
	engine := services.NewProtectionEngine()
	require.NoError(t, engine.ReplaceRules([]services.PublishedProtectionRuleSet{{
		RuleSetID: 3, Version: 1, Scope: services.ProtectionScopeCustom,
		ResourcePattern: "billing.charge",
		Rules:           []services.ProtectionRule{{Type: services.ProtectionRuleTypeConcurrency, MaxConcurrency: 1}},
	}}))

	request := services.ProtectionRequestContext{CustomResource: "billing.charge"}
	require.True(t, engine.Evaluate("billing.charge", request).Allowed)
	rejected := engine.Evaluate("billing.charge", request)
	require.False(t, rejected.Allowed)
	require.ErrorIs(t, rejected.Error, services.ErrProtectionConcurrencyLimited)

	engine.RecordSuccess("billing.charge", 10*time.Millisecond)
	require.True(t, engine.Evaluate("billing.charge", request).Allowed)
}

func TestProtectionDecisionReleasesMatchedEndpointConcurrency(t *testing.T) {
	engine := services.NewProtectionEngine()
	require.NoError(t, engine.ReplaceRules([]services.PublishedProtectionRuleSet{{
		RuleSetID: 31, Version: 1, Scope: services.ProtectionScopeEndpoint,
		ResourcePattern: "/orders/*",
		Rules:           []services.ProtectionRule{{Type: services.ProtectionRuleTypeConcurrency, MaxConcurrency: 1}},
	}}))

	request := services.ProtectionRequestContext{Endpoint: "/orders/create"}
	decision := engine.Evaluate("orders.create", request)
	require.True(t, decision.Allowed)
	require.False(t, engine.Evaluate("orders.create", request).Allowed)

	engine.RecordDecisionSuccess(decision, 10*time.Millisecond)
	require.True(t, engine.Evaluate("orders.create", request).Allowed)
}

func TestProtectionDecisionDoesNotReleaseNewRuleVersionConcurrency(t *testing.T) {
	engine := services.NewProtectionEngine()
	require.NoError(t, engine.ReplaceRules([]services.PublishedProtectionRuleSet{{
		RuleSetID: 32, Version: 1, Scope: services.ProtectionScopeService,
		ResourcePattern: "payment",
		Rules:           []services.ProtectionRule{{Type: services.ProtectionRuleTypeConcurrency, MaxConcurrency: 1}},
	}}))
	oldDecision := engine.Evaluate("payment", services.ProtectionRequestContext{Service: "payment"})
	require.True(t, oldDecision.Allowed)

	require.NoError(t, engine.ReplaceRules([]services.PublishedProtectionRuleSet{{
		RuleSetID: 32, Version: 2, Scope: services.ProtectionScopeService,
		ResourcePattern: "payment",
		Rules:           []services.ProtectionRule{{Type: services.ProtectionRuleTypeConcurrency, MaxConcurrency: 1}},
	}}))
	newDecision := engine.Evaluate("payment", services.ProtectionRequestContext{Service: "payment"})
	require.True(t, newDecision.Allowed)

	engine.RecordDecisionSuccess(oldDecision, 10*time.Millisecond)
	require.False(t, engine.Evaluate("payment", services.ProtectionRequestContext{Service: "payment"}).Allowed)
	engine.RecordDecisionSuccess(newDecision, 10*time.Millisecond)
	require.True(t, engine.Evaluate("payment", services.ProtectionRequestContext{Service: "payment"}).Allowed)
}

func TestProtectionDecisionRecordsMatchedEndpointCircuitFailure(t *testing.T) {
	engine := services.NewProtectionEngine()
	require.NoError(t, engine.ReplaceRules([]services.PublishedProtectionRuleSet{{
		RuleSetID: 33, Version: 1, Scope: services.ProtectionScopeEndpoint,
		ResourcePattern: "/payments/*",
		Rules: []services.ProtectionRule{{
			Type:             services.ProtectionRuleTypeFailureRateCircuit,
			ThresholdPercent: 100, MinimumRequests: 1, StatisticalWindowMS: 10_000,
			OpenDurationMS: 1_000, HalfOpenProbes: 1, HalfOpenSuccesses: 1,
		}},
	}}))

	decision := engine.Evaluate("payments.capture", services.ProtectionRequestContext{Endpoint: "/payments/capture"})
	require.True(t, decision.Allowed)
	engine.RecordDecisionFailure(decision, 20*time.Millisecond)

	rejected := engine.Evaluate("payments.capture", services.ProtectionRequestContext{Endpoint: "/payments/capture"})
	require.False(t, rejected.Allowed)
	require.ErrorIs(t, rejected.Error, services.ErrProtectionCircuitOpen)
}

func TestProtectionEngineTransitionsClosedOpenHalfOpenClosed(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	engine := services.NewProtectionEngine()
	engine.SetNowForTest(func() time.Time { return now })
	require.NoError(t, engine.ReplaceRules([]services.PublishedProtectionRuleSet{{
		RuleSetID: 4, Version: 1, Scope: services.ProtectionScopeService,
		ResourcePattern: "payment",
		Rules: []services.ProtectionRule{{
			Type:             services.ProtectionRuleTypeFailureRateCircuit,
			ThresholdPercent: 50, MinimumRequests: 2, StatisticalWindowMS: 10_000,
			OpenDurationMS: 1_000, HalfOpenProbes: 1, HalfOpenSuccesses: 1,
		}},
	}}))

	request := services.ProtectionRequestContext{Service: "payment"}
	require.True(t, engine.Evaluate("payment", request).Allowed)
	engine.RecordFailure("payment", 20*time.Millisecond)
	require.True(t, engine.Evaluate("payment", request).Allowed)
	engine.RecordSuccess("payment", 20*time.Millisecond)

	rejected := engine.Evaluate("payment", request)
	require.False(t, rejected.Allowed)
	require.Equal(t, services.ProtectionCircuitOpen, rejected.State)
	require.True(t, errors.Is(rejected.Error, services.ErrProtectionCircuitOpen))

	now = now.Add(time.Second)
	probe := engine.Evaluate("payment", request)
	require.True(t, probe.Allowed)
	require.Equal(t, services.ProtectionCircuitHalfOpen, probe.State)
	engine.RecordSuccess("payment", 5*time.Millisecond)

	state := engine.State(4)
	require.Len(t, state.Circuits, 1)
	require.Equal(t, services.ProtectionCircuitClosed, state.Circuits[0].State)
}

func TestProtectionRuleValidationRejectsConflictingAndInvalidRules(t *testing.T) {
	result := services.ValidateProtectionRuleSet(
		services.ProtectionScopeGlobal,
		"*",
		map[string]any{"rules": []any{
			map[string]any{"type": services.ProtectionRuleTypeRateLimit, "limit": 0, "window_ms": 10},
		}},
	)
	require.False(t, result.Valid)
	require.NotEmpty(t, result.Errors)

	err := services.ValidatePublishedProtectionConflicts([]services.PublishedProtectionRuleSet{
		{RuleSetID: 1, Scope: services.ProtectionScopeService, ResourcePattern: "orders"},
		{RuleSetID: 2, Scope: services.ProtectionScopeService, ResourcePattern: "orders"},
	})
	require.ErrorContains(t, err, "冲突")
}
