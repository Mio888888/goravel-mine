package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	contractshttp "github.com/goravel/framework/contracts/http"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"goravel/app/facades"
	"goravel/app/services"
)

const instrumentationName = "goravel-mine/http"

func Observability() contractshttp.Middleware {
	return func(ctx contractshttp.Context) {
		requestID := requestIDFromHeader(ctx)
		traceID := traceIDFromHeader(ctx)
		if requestID == "" {
			requestID = services.NewRequestID()
		}
		if traceID == "" {
			traceID = services.NewTraceID()
		}

		base := services.WithRequestID(ctx.Context(), requestID)
		base = services.WithTraceID(base, traceID)
		base = otel.GetTextMapPropagator().Extract(base, propagation.HeaderCarrier(ctx.Request().Headers()))
		spanCtx, span := otel.Tracer(instrumentationName).Start(base, spanName(ctx))
		defer span.End()

		if span.SpanContext().HasTraceID() {
			traceID = span.SpanContext().TraceID().String()
		}
		spanCtx = services.WithRequestID(spanCtx, requestID)
		spanCtx = services.WithTraceID(spanCtx, traceID)
		ctx.WithContext(spanCtx)
		ctx.Response().Header(requestIDHeader(), requestID)
		ctx.Response().Header(traceIDHeader(), traceID)

		start := time.Now()
		services.RecordHTTPObservationStart()
		defer func() {
			recovered := recover()
			services.RecordHTTPObservationFinish()
			status := responseStatus(ctx)
			if recovered != nil {
				status = http.StatusInternalServerError
			}
			duration := time.Since(start)
			route := observationRoute(ctx)
			span.SetAttributes(
				attribute.String("http.request.method", ctx.Request().Method()),
				attribute.String("url.path", ctx.Request().Path()),
				attribute.String("http.route", route),
				attribute.Int("http.response.status_code", status),
				attribute.String("request_id", requestID),
				attribute.String("trace_id", traceID),
			)
			if status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(status))
			}

			observation := services.HTTPObservation{
				Method:     ctx.Request().Method(),
				Route:      route,
				Path:       ctx.Request().Path(),
				Status:     status,
				Duration:   duration,
				RequestID:  requestID,
				TraceID:    traceID,
				IP:         ctx.Request().Ip(),
				RecordedAt: time.Now(),
			}
			services.RecordHTTPObservation(observation)
			logHTTPObservation(ctx.Context(), observation)
			if recovered != nil {
				panic(recovered)
			}
		}()
		ctx.Request().Next()
	}
}

func requestIDFromHeader(ctx contractshttp.Context) string {
	value := ctx.Request().Header(requestIDHeader(), "")
	return services.NormalizeObservabilityID(value)
}

func traceIDFromHeader(ctx contractshttp.Context) string {
	if value := traceIDFromTraceparent(ctx.Request().Header("traceparent", "")); value != "" {
		return value
	}
	return traceIDFromCustomHeader(ctx)
}

func traceIDFromCustomHeader(ctx contractshttp.Context) string {
	value := ctx.Request().Header(traceIDHeader(), "")
	return services.NormalizeObservabilityID(value)
}

func requestIDHeader() string {
	return facades.Config().GetString("observability.request_id.header", "X-Request-Id")
}

func traceIDHeader() string {
	return facades.Config().GetString("observability.trace_id.header", "X-Trace-Id")
}

func spanName(ctx contractshttp.Context) string {
	return ctx.Request().Method() + " " + observationRoute(ctx)
}

func observationRoute(ctx contractshttp.Context) string {
	if route := ctx.Request().OriginPath(); route != "" {
		return route
	}
	return "unknown"
}

func responseStatus(ctx contractshttp.Context) int {
	status := ctx.Response().Origin().Status()
	if status == 0 {
		return http.StatusOK
	}
	return status
}

func traceIDFromTraceparent(traceparent string) string {
	parts := strings.Split(traceparent, "-")
	if len(parts) < 4 {
		return ""
	}
	value := strings.TrimSpace(parts[1])
	if len(value) != 32 {
		return ""
	}
	if _, err := trace.TraceIDFromHex(value); err != nil {
		return ""
	}
	return value
}

func logHTTPObservation(ctx context.Context, obs services.HTTPObservation) {
	data := map[string]any{
		"event":       "http_request",
		"method":      obs.Method,
		"route":       obs.Route,
		"path":        obs.Path,
		"status":      obs.Status,
		"duration_ms": obs.Duration.Milliseconds(),
		"request_id":  obs.RequestID,
		"trace_id":    obs.TraceID,
		"ip":          obs.IP,
	}
	facades.Log().WithContext(ctx).With(data).Info("http request")
}

func TraceIDFromSpanContext(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.HasTraceID() {
		return services.TraceID(ctx)
	}
	return spanCtx.TraceID().String()
}
