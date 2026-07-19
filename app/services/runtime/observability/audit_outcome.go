package observability

import (
	"context"

	"goravel/app/support/contextutil"
)

type auditOutcomeContextKey struct{}

const (
	AuditOutcomeSuccess = "success"
	AuditOutcomeFailure = "failure"
)

func WithAuditOutcome(ctx context.Context, outcome string) context.Context {
	return context.WithValue(contextutil.OrBackground(ctx), auditOutcomeContextKey{}, outcome)
}

func AuditOutcome(ctx context.Context) string {
	value, _ := contextutil.OrBackground(ctx).Value(auditOutcomeContextKey{}).(string)
	return value
}
