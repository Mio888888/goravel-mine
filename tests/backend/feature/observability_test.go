package feature

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	contractshttp "github.com/goravel/framework/contracts/http"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"goravel/app/facades"
	"goravel/app/http/controllers"
	"goravel/app/services"
	"goravel/tests/backend/testcase"
)

const observabilityProbeRoute = "/__observability_context_probe"

type ObservabilityTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestObservabilityTestSuite(t *testing.T) {
	suite.Run(t, new(ObservabilityTestSuite))
}

func (s *ObservabilityTestSuite) SetupSuite() {
	facades.Route().Get(observabilityProbeRoute, func(ctx contractshttp.Context) contractshttp.Response {
		return ctx.Response().Json(http.StatusOK, map[string]string{
			"request_id": services.RequestID(ctx.Context()),
			"trace_id":   services.TraceID(ctx.Context()),
		})
	})
}

func (s *ObservabilityTestSuite) TestRequestIDAndTraceIDHeadersPropagate() {
	res, err := s.Http(s.T()).
		WithHeader("X-Request-Id", "req-feature").
		WithHeader("X-Trace-Id", "trace-feature").
		Get("/health/live")
	require.NoError(s.T(), err)

	res.AssertOk()
	res.AssertHeader("X-Request-Id", "req-feature")
	res.AssertHeader("X-Trace-Id", "trace-feature")
}

func (s *ObservabilityTestSuite) TestCustomTraceIDUsesOTelSpanTraceIDWhenTracingEnabled() {
	services.ResetObservabilityMetricsForTest()
	services.ConfigureObservabilityRecorder(time.Nanosecond, 100)
	previousProvider := otel.GetTracerProvider()
	provider := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() {
		_ = provider.Shutdown(s.T().Context())
		otel.SetTracerProvider(previousProvider)
		services.ResetObservabilityMetricsForTest()
	}()

	res, err := s.Http(s.T()).
		WithHeader("X-Trace-Id", "custom-trace").
		Get(observabilityProbeRoute)
	require.NoError(s.T(), err)

	res.AssertOk()
	traceID := res.Headers().Get("X-Trace-Id")
	require.NotEmpty(s.T(), traceID)
	require.NotEqual(s.T(), "custom-trace", traceID)
	var body map[string]string
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), traceID, body["trace_id"])
	snapshot := services.ObservabilityMetrics()
	require.NotEmpty(s.T(), snapshot.SlowRequests)
	require.Equal(s.T(), traceID, snapshot.SlowRequests[0].TraceID)
}

func (s *ObservabilityTestSuite) TestGeneratedIDsAreStoredInRequestContext() {
	res, err := s.Http(s.T()).Get(observabilityProbeRoute)
	require.NoError(s.T(), err)

	res.AssertOk()
	requestID := res.Headers().Get("X-Request-Id")
	traceID := res.Headers().Get("X-Trace-Id")
	require.NotEmpty(s.T(), requestID)
	require.NotEmpty(s.T(), traceID)
	var body map[string]string
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), requestID, body["request_id"])
	require.Equal(s.T(), traceID, body["trace_id"])
}

func (s *ObservabilityTestSuite) TestTraceparentTraceIDMatchesResponseHeader() {
	services.ResetObservabilityMetricsForTest()
	services.ConfigureObservabilityRecorder(time.Nanosecond, 100)
	defer services.ResetObservabilityMetricsForTest()
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	traceparent := "00-" + traceID + "-00f067aa0ba902b7-01"

	res, err := s.Http(s.T()).
		WithHeader("traceparent", traceparent).
		WithHeader("X-Trace-Id", "custom-trace").
		Get("/health/live")
	require.NoError(s.T(), err)

	res.AssertOk()
	res.AssertHeader("X-Trace-Id", traceID)
	snapshot := services.ObservabilityMetrics()
	require.NotEmpty(s.T(), snapshot.SlowRequests)
	require.Equal(s.T(), traceID, snapshot.SlowRequests[0].TraceID)
}

func (s *ObservabilityTestSuite) TestGeneratedTraceIDMatchesSlowRequestSample() {
	services.ResetObservabilityMetricsForTest()
	services.ConfigureObservabilityRecorder(time.Nanosecond, 100)
	defer services.ResetObservabilityMetricsForTest()

	res, err := s.Http(s.T()).Get("/health/live")
	require.NoError(s.T(), err)

	res.AssertOk()
	traceID := res.Headers().Get("X-Trace-Id")
	require.NotEmpty(s.T(), traceID)
	snapshot := services.ObservabilityMetrics()
	require.NotEmpty(s.T(), snapshot.SlowRequests)
	require.Equal(s.T(), traceID, snapshot.SlowRequests[0].TraceID)
}

func (s *ObservabilityTestSuite) TestMetricsEndpointDisabledByDefault() {
	res, err := s.Http(s.T()).Get("/metrics")
	require.NoError(s.T(), err)

	res.AssertNotFound()
}

func (s *ObservabilityTestSuite) TestPanicRequestIsRecordedAsServerError() {
	services.ResetObservabilityMetricsForTest()
	path := fmt.Sprintf("/__observability_panic_test_%d", time.Now().UnixNano())
	facades.Route().Get(path, func(ctx contractshttp.Context) contractshttp.Response {
		panic("observability panic test")
	})

	res, err := s.Http(s.T()).Get(path)
	require.NoError(s.T(), err)

	res.AssertInternalServerError()
	snapshot := services.ObservabilityMetrics()
	require.Equal(s.T(), uint64(1), snapshot.TotalRequests)
	require.Equal(s.T(), int64(0), snapshot.Inflight)
	require.Len(s.T(), snapshot.ByRoute, 1)
	require.Equal(s.T(), http.StatusInternalServerError, snapshot.ByRoute[0].Status)
}

func (s *ObservabilityTestSuite) TestMetricsRequiresBearerTokenOnly() {
	facades.Config().Add("observability.metrics.token", "metric-secret")
	defer facades.Config().Add("observability.metrics.token", "")
	path := fmt.Sprintf("/__observability_metrics_auth_test_%d", time.Now().UnixNano())
	facades.Route().Get(path, controllers.NewObservabilityController().Metrics)

	queryRes, err := s.Http(s.T()).Get(path + "?token=metric-secret")
	require.NoError(s.T(), err)
	queryRes.AssertUnauthorized()

	rawTokenRes, err := s.Http(s.T()).
		WithHeader("Authorization", "metric-secret").
		Get(path)
	require.NoError(s.T(), err)
	rawTokenRes.AssertUnauthorized()

	bearerRes, err := s.Http(s.T()).
		WithHeader("Authorization", "Bearer metric-secret").
		Get(path)
	require.NoError(s.T(), err)
	bearerRes.AssertOk()
}

func (s *ObservabilityTestSuite) TestUnknownRouteUsesBoundedMetricLabel() {
	services.ResetObservabilityMetricsForTest()
	path := fmt.Sprintf("/__observability_missing_%d", time.Now().UnixNano())

	res, err := s.Http(s.T()).Get(path)
	require.NoError(s.T(), err)

	res.AssertNotFound()
	snapshot := services.ObservabilityMetrics()
	require.Equal(s.T(), uint64(1), snapshot.TotalRequests)
	require.Len(s.T(), snapshot.ByRoute, 1)
	require.Equal(s.T(), "unknown", snapshot.ByRoute[0].Route)
}
