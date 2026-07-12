package services

import "context"

type auditOutcomeContextKey struct{}

const (
	AuditOutcomeSuccess = "success"
	AuditOutcomeFailure = "failure"
)

func WithAuditOutcome(ctx context.Context, outcome string) context.Context {
	return context.WithValue(contextOrBackground(ctx), auditOutcomeContextKey{}, outcome)
}

func AuditOutcome(ctx context.Context) string {
	value, _ := contextOrBackground(ctx).Value(auditOutcomeContextKey{}).(string)
	return value
}
