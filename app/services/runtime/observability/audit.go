package observability

import (
	"context"
	"time"

	"goravel/app/facades"
	"goravel/app/support/redaction"
)

type AuditEvent struct {
	Action    string         `json:"action"`
	Outcome   string         `json:"outcome"`
	Actor     string         `json:"actor"`
	Method    string         `json:"method"`
	Route     string         `json:"route"`
	Path      string         `json:"path"`
	IP        string         `json:"ip"`
	RequestID string         `json:"request_id"`
	TraceID   string         `json:"trace_id"`
	Fields    map[string]any `json:"fields"`
	Time      time.Time      `json:"time"`
}

func RecordAuditEvent(ctx context.Context, event AuditEvent) {
	config := facades.Config()
	logger := facades.Log()
	if config == nil || logger == nil || !config.GetBool("observability.audit.enabled", true) {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	if event.RequestID == "" {
		event.RequestID = RequestID(ctx)
	}
	if event.TraceID == "" {
		event.TraceID = TraceID(ctx)
	}
	logger.WithContext(ctx).With(map[string]any{
		"event":      "audit",
		"action":     event.Action,
		"outcome":    event.Outcome,
		"actor":      event.Actor,
		"method":     event.Method,
		"route":      event.Route,
		"path":       event.Path,
		"ip":         event.IP,
		"request_id": event.RequestID,
		"trace_id":   event.TraceID,
		"fields":     redaction.SensitiveData(event.Fields),
		"time":       event.Time.Format(time.RFC3339Nano),
	}).Info("audit event")
}

func RecordTenantGovernanceEvent(ctx context.Context, fields map[string]any) {
	config := facades.Config()
	logger := facades.Log()
	if config == nil || logger == nil || !config.GetBool("observability.audit.enabled", true) {
		return
	}
	redacted, ok := redaction.SensitiveData(fields).(map[string]any)
	if !ok {
		redacted = map[string]any{}
	}
	fields = redacted
	fields["event"] = "tenant_governance"
	fields["request_id"] = RequestID(ctx)
	fields["trace_id"] = TraceID(ctx)
	fields["time"] = time.Now().UTC().Format(time.RFC3339Nano)
	logger.WithContext(ctx).With(fields).Info("tenant governance event")
}
