package modulecatalog

import (
	"context"
	"errors"

	"goravel/app/services"
)

type adminSecurityGate struct {
	enabled              bool
	confirmToken         string
	expectedConfirmToken string
	operation            string
	resource             string
	reAuthToken          string
	approvalID           string
	operatorID           uint64
	confirmError         string
	reAuthError          string
	approvalError        string
}

func (g adminSecurityGate) preflight(ctx context.Context) error {
	return g.validate(ctx, false)
}

func (g adminSecurityGate) consume(ctx context.Context) error {
	return g.execute(ctx, func() error { return nil })
}

func (g adminSecurityGate) validate(ctx context.Context, consume bool) error {
	if !g.enabled {
		return nil
	}
	if g.confirmToken != g.expectedConfirmToken {
		return services.BusinessError{Message: g.confirmError}
	}
	guard := services.NewSensitiveOperationGuard(nil)
	plan, err := guard.PrepareCanonical(ctx, g.operation, g.operatorID, 0, services.SensitiveOperationPlanSelector{Resource: g.resource})
	if err == nil {
		evidence := services.SensitiveOperationEvidence{ReAuthToken: g.reAuthToken, ApprovalID: g.approvalID}
		if consume {
			err = guard.Execute(ctx, plan, evidence, func() error { return nil })
		} else {
			err = guard.Validate(ctx, plan, evidence)
		}
	}
	return g.mapError(err)
}

func (g adminSecurityGate) execute(ctx context.Context, mutate func() error) error {
	if !g.enabled {
		return mutate()
	}
	if g.confirmToken != g.expectedConfirmToken {
		return services.BusinessError{Message: g.confirmError}
	}
	guard := services.NewSensitiveOperationGuard(nil)
	plan, err := guard.PrepareCanonical(ctx, g.operation, g.operatorID, 0, services.SensitiveOperationPlanSelector{Resource: g.resource})
	if err == nil {
		err = guard.Execute(ctx, plan, services.SensitiveOperationEvidence{
			ReAuthToken: g.reAuthToken,
			ApprovalID:  g.approvalID,
		}, mutate)
	}
	return g.mapError(err)
}

func (g adminSecurityGate) mapError(err error) error {
	if errors.Is(err, services.ErrReAuthRequired) {
		return services.BusinessError{Message: g.reAuthError}
	}
	if err == nil || !isSecurityApprovalGateError(err) {
		return err
	}
	return services.BusinessError{Message: g.approvalError}
}

func newAdminExecuteSecurityGate(payload AdminExecutePayload, action string) adminSecurityGate {
	moduleID := payload.ModuleID
	if moduleID == "" {
		moduleID = "all"
	}
	return adminSecurityGate{
		enabled: payload.Execute, confirmToken: payload.ConfirmToken,
		expectedConfirmToken: moduleID + ":" + action,
		operation:            "module.lifecycle.execute",
		resource:             "module-lifecycle:" + moduleID + ":" + action,
		reAuthToken:          payload.ReAuthToken, approvalID: payload.ApprovalID, operatorID: payload.OperatorID,
		confirmError:  "module lifecycle execute requires confirm token",
		reAuthError:   "module lifecycle execute requires valid re-auth token",
		approvalError: "module lifecycle execute requires approved approval record",
	}
}

func newAdminLockReleaseSecurityGate(payload AdminLockReleasePayload) adminSecurityGate {
	return adminSecurityGate{
		enabled: true, confirmToken: payload.ConfirmToken, expectedConfirmToken: "release-stale-locks",
		operation: "module.lifecycle.release-lock", resource: staleLockReleaseResource(payload.Key),
		reAuthToken: payload.ReAuthToken, approvalID: payload.ApprovalID, operatorID: payload.OperatorID,
		confirmError:  "stale lock release requires confirm token",
		reAuthError:   "stale lock release requires valid re-auth token",
		approvalError: "stale lock release requires approved approval record",
	}
}

func isSecurityApprovalGateError(err error) bool {
	return errors.Is(err, services.ErrApprovalRequired) ||
		errors.Is(err, services.ErrApprovalSelfApproved) ||
		errors.Is(err, services.ErrSensitiveOperationBinding) ||
		errors.Is(err, services.ErrSensitiveOperationPolicy)
}
