package services

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/models"
)

func TestTenantDataExportRequestCanonicalizesFilters(t *testing.T) {
	_, _, empty, emptyDigest, err := normalizeTenantExportRequest(TenantDataExportRequest{Dataset: "users", Format: "jsonl"})
	require.NoError(t, err)
	require.Empty(t, empty)
	require.Equal(t, digestBytes(nil), emptyDigest)

	_, _, filters, filteredDigest, err := normalizeTenantExportRequest(TenantDataExportRequest{
		Dataset: "users", Format: "csv", Filters: map[string]string{"status": "2"},
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"status": "2"}, filters)
	require.Equal(t, digestBytes([]byte("status=2")), filteredDigest)

	_, _, _, _, err = normalizeTenantExportRequest(TenantDataExportRequest{Dataset: "audit_logs", Format: "jsonl"})
	require.ErrorIs(t, err, ErrTenantExportInvalid)
}

func TestTenantExportRunBindsOperator(t *testing.T) {
	run := models.TenantGovernanceRun{IdempotencyKey: "tenant-data:export:7:users:jsonl:sha256:abc:v1:approval:sha256:def:operator:42"}
	require.Equal(t, uint64(42), tenantExportRunOperatorID(run))
	require.Zero(t, tenantExportRunOperatorID(models.TenantGovernanceRun{IdempotencyKey: "unbound"}))
}

func TestTenantExportIdempotencyKeyBindsApproval(t *testing.T) {
	first := tenantExportIdempotencyKey("tenant-data:export:7:users:csv:sha256:abc", "v1", "approval:approval-a", 42)
	retry := tenantExportIdempotencyKey("tenant-data:export:7:users:csv:sha256:abc", "v1", " approval:approval-a ", 42)
	next := tenantExportIdempotencyKey("tenant-data:export:7:users:csv:sha256:abc", "v1", "approval:approval-b", 42)

	require.Equal(t, first, retry)
	require.NotEqual(t, first, next)
	require.LessOrEqual(t, len(next), 255)
	require.Equal(t, uint64(42), tenantExportRunOperatorID(models.TenantGovernanceRun{IdempotencyKey: next}))
}

func TestTenantExportIdempotencyFactorUsesFreshReAuthWithoutApproval(t *testing.T) {
	request := TenantDataExportRequest{ApprovalID: "approval-a", ReAuthToken: "reauth-a"}
	retry := TenantDataExportRequest{ApprovalID: "approval-b", ReAuthToken: " reauth-a "}
	next := TenantDataExportRequest{ApprovalID: "approval-a", ReAuthToken: "reauth-b"}

	require.Equal(t, "approval:approval-a", tenantExportIdempotencyFactor(true, request))
	require.Equal(t, tenantExportIdempotencyFactor(false, request), tenantExportIdempotencyFactor(false, retry))
	require.NotEqual(t, tenantExportIdempotencyFactor(false, request), tenantExportIdempotencyFactor(false, next))
	require.NotContains(t, tenantExportIdempotencyFactor(false, request), request.ReAuthToken)
}

func TestTenantExportSensitiveGuardUsesGovernanceApprovalPolicy(t *testing.T) {
	for _, test := range []struct {
		name             string
		requiresApproval bool
	}{
		{name: "approval enabled", requiresApproval: true},
		{name: "approval disabled", requiresApproval: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy, ok := newTenantExportSensitiveGuard(test.requiresApproval).registry.Policy("tenant.data.export")
			require.True(t, ok)
			require.True(t, policy.RequiresReAuth)
			require.Equal(t, test.requiresApproval, policy.RequiresApproval)
		})
	}

	defaultPolicy, ok := NewSensitiveOperationPolicyRegistry().Policy("tenant.data.export")
	require.True(t, ok)
	require.True(t, defaultPolicy.RequiresApproval)
}

func TestTenantExportCSVNeutralizesSpreadsheetFormulas(t *testing.T) {
	payload, err := tenantExportCSV([]tenantExportUser{{
		ID: 1, Username: "=HYPERLINK(\"https://example.test\")", Nickname: " @SUM(1,1)",
		Email: "\tcmd@example.test", Phone: "\r-1+1", Status: 1, CreatedAt: time.Unix(0, 0).UTC(),
	}})
	require.NoError(t, err)
	records, err := csv.NewReader(strings.NewReader(string(payload))).ReadAll()
	require.NoError(t, err)
	require.Equal(t, "'=HYPERLINK(\"https://example.test\")", records[1][1])
	require.Equal(t, "' @SUM(1,1)", records[1][2])
	require.Equal(t, "'\tcmd@example.test", records[1][3])
	require.Equal(t, "'\r-1+1", records[1][4])
}

func TestTenantExportDownloadTokenFailsClosedWithoutCache(t *testing.T) {
	_, _, err := issueTenantExportDownloadToken(9, 7, 5)
	require.ErrorIs(t, err, ErrTenantExportInvalid)
}
