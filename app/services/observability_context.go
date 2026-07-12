package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
)

type observabilityContextKey string

const (
	requestIDContextKey observabilityContextKey = "request_id"
	traceIDContextKey   observabilityContextKey = "trace_id"
)

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey, requestID)
}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey).(string)
	return value
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDContextKey, traceID)
}

func TraceID(ctx context.Context) string {
	value, _ := ctx.Value(traceIDContextKey).(string)
	return value
}

func NewTraceID() string {
	return randomHex(16)
}

func NewRequestID() string {
	return randomHex(16)
}

func NormalizeObservabilityID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		return value[:128]
	}
	return value
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}
