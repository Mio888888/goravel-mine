package services

import (
	"context"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
)

func (g *SensitiveOperationGuard) ExecutePlatformTransaction(
	ctx context.Context,
	plan SensitiveOperationPlan,
	evidence SensitiveOperationEvidence,
	mutate func(contractsorm.Query) error,
) error {
	policy, err := g.validPolicy(plan)
	if err != nil || mutate == nil {
		return ErrSensitiveOperationPolicy
	}
	request := SensitiveOperationRequest{
		UserID: plan.ActorID, TenantID: plan.TenantID, Operation: plan.Scope,
		Resource: plan.Resource, ReAuthToken: evidence.ReAuthToken,
	}
	operation := func() error {
		return OrmForConnectionWithContext(contextOrBackground(ctx), PlatformConnection()).Transaction(func(query contractsorm.Query) error {
			if policy.RequiresApproval {
				approval, ok, loadErr := g.security.loadPermissionApproval(ctx, evidence.ApprovalID)
				if loadErr != nil || !ok || validRegisteredPermissionApprovalBinding(approval, plan) != nil {
					return ErrApprovalRequired
				}
				if consumeErr := consumePermissionApprovalBindingWithQuery(query, evidence.ApprovalID, approval, plan); consumeErr != nil {
					return consumeErr
				}
			}
			return mutate(query)
		})
	}
	if policy.RequiresReAuth {
		err = g.security.ExecuteSensitiveOperationNoRestore(request, nil, operation)
	} else {
		err = operation()
	}
	if err != nil {
		g.recordAudit(ctx, plan, evidence.ApprovalID, "operation_failed")
		return err
	}
	g.recordAudit(ctx, plan, evidence.ApprovalID, "success")
	return nil
}
