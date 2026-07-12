package modulecatalog

import (
	"context"
	"strings"
	"time"

	"goravel/app/modules"
)

func (s *AdminService) Execute(payload AdminExecutePayload) (LifecycleResult, error) {
	ctx := contextOrBackground(s.ctx)
	action := payload.Action
	if action == "" {
		action = LifecycleActionUpgrade
	}
	emergencyRemoval := requiresEmergencyReplacementRemoval(s.registry, payload.ModuleID, action, time.Now())
	gate := newAdminExecuteSecurityGate(payload, action)
	if emergencyRemoval {
		gate.operation = "module.replacement.emergency-remove"
		gate.resource = replacementEmergencyRemovalResource(payload.ModuleID, s.registry.ReplacementPlans()[payload.ModuleID].ToModule)
		gate.approvalError = "module replacement removal before end of support requires emergency approval"
	}
	if err := gate.preflight(ctx); err != nil {
		return LifecycleResult{}, err
	}
	return NewLifecycleService(s.registry).Execute(ctx, action, LifecycleOptions{
		ModuleID: payload.ModuleID, Execute: payload.Execute, Owner: payload.Owner, Reason: payload.Reason,
		ConfirmToken: payload.ConfirmToken, ReAuthToken: payload.ReAuthToken, ApprovalID: payload.ApprovalID,
		securityGate: func(gateCtx context.Context, mutate func() error) error {
			return gate.execute(gateCtx, mutate)
		},
		emergencyRemovalApproved: emergencyRemoval,
	})
}

func requiresEmergencyReplacementRemoval(registry modules.Registry, moduleID, action string, now time.Time) bool {
	if action != LifecycleActionUninstall || strings.TrimSpace(moduleID) == "" {
		return false
	}
	plan, ok := registry.ReplacementPlans()[strings.TrimSpace(moduleID)]
	return ok && now.Before(plan.EndOfSupport)
}

func replacementEmergencyRemovalResource(fromModule, toModule string) string {
	return "module-replacement:" + strings.TrimSpace(fromModule) + ":emergency-remove:" + strings.TrimSpace(toModule)
}
