package middleware

import (
	"net/http"
	"testing"

	httpctx "github.com/goravel/framework/http"
	"github.com/stretchr/testify/require"

	"goravel/app/services"
)

func TestAuditOutcomeFromStatus(t *testing.T) {
	ctx := httpctx.Background()

	require.Equal(t, services.AuditOutcomeSuccess, auditOutcome(ctx, 0))
	require.Equal(t, services.AuditOutcomeSuccess, auditOutcome(ctx, http.StatusOK))
	require.Equal(t, services.AuditOutcomeFailure, auditOutcome(ctx, http.StatusUnprocessableEntity))
	require.Equal(t, services.AuditOutcomeFailure, auditOutcome(ctx, http.StatusBadRequest))
	require.Equal(t, services.AuditOutcomeFailure, auditOutcome(ctx, http.StatusInternalServerError))
}

func TestAuditOutcomeUsesBusinessFailureFromContext(t *testing.T) {
	ctx := httpctx.Background()
	ctx.WithContext(services.WithAuditOutcome(ctx.Context(), services.AuditOutcomeFailure))

	require.Equal(t, services.AuditOutcomeFailure, auditOutcome(ctx, http.StatusOK))
}

func TestMarkAuditPanicForcesFailureOutcome(t *testing.T) {
	ctx := httpctx.Background()

	markAuditPanic(ctx)

	require.Equal(t, http.StatusInternalServerError, auditResponseStatus(ctx))
	require.Equal(t, services.AuditOutcomeFailure, auditOutcome(ctx, http.StatusOK))
}

func TestAuditOutcomeUsesBusinessCodeFromResponseBody(t *testing.T) {
	code, ok := auditBusinessCodeFromBody([]byte(`{"code":422,"message":"请求参数错误","data":[]}`))

	require.True(t, ok)
	require.Equal(t, 422, code)
	require.Equal(t, services.AuditOutcomeFailure, auditOutcomeFromSignals(http.StatusOK, "", code, ok))
}

func TestShouldRecordAuditOnlyMutatingNonPassportRoutes(t *testing.T) {
	require.False(t, shouldRecordAudit("GET", "/admin/platform/tenant/list"))
	require.False(t, shouldRecordAudit("POST", "/admin/platform/passport/login"))
	require.True(t, shouldRecordAudit("POST", "/admin/platform/tenant"))
	require.True(t, shouldRecordAudit("DELETE", "/admin/user"))
}

func TestPlatformOperationNameUsesHumanLabelThenPermissionFallback(t *testing.T) {
	require.Equal(t, "创建租户", platformOperationName("POST", "/admin/platform/tenant"))
	require.Equal(t, "平台 MFA 设置", platformOperationName("POST", "/admin/platform/security/mfa/setup"))
	require.Equal(t, "platform:observability:list", platformOperationName("GET", "/admin/platform/observability/slow-requests"))
	require.Equal(t, "平台操作", platformOperationName("POST", "/admin/platform/unknown"))
}

func TestOperationServiceNameNamesTenantMFAChanges(t *testing.T) {
	require.Equal(t, "设置 MFA", operationServiceName("POST", "/admin/security/mfa/setup"))
	require.Equal(t, "关闭 MFA", operationServiceName("POST", "/admin/security/mfa/disable"))
}
