package services

import (
	"github.com/goravel/framework/contracts/log"
)

type ObservabilityLogDriver struct{}

type observabilityLogHandler struct{}

func (d *ObservabilityLogDriver) Handle(channel string) (log.Handler, error) {
	return &observabilityLogHandler{}, nil
}

func (h *observabilityLogHandler) Enabled(level log.Level) bool {
	return level >= log.LevelWarning
}

func (h *observabilityLogHandler) Handle(entry log.Entry) error {
	ctx := entry.Context()
	requestID := ""
	traceID := ""
	if ctx != nil {
		requestID = RequestID(ctx)
		traceID = TraceID(ctx)
	}
	RecordSlowSQLFromContext(ctx, entry.Message(), requestID, traceID)
	return nil
}
