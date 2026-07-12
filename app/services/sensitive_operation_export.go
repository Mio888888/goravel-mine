package services

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type tenantExportSelector struct {
	TenantID     uint64
	Dataset      string
	Format       string
	FilterDigest string
}

func resolveTenantExportPlan(ctx context.Context, tenantID uint64, selector string) (string, []string, []string, error) {
	parsed, err := parseTenantExportSelector(selector)
	if err != nil || (tenantID != 0 && parsed.TenantID != tenantID) {
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
	tenant, err := NewTenantService().WithContext(ctx).FindByID(parsed.TenantID)
	if err != nil {
		return "", nil, nil, err
	}
	policy, err := NewTenantGovernanceService().WithContext(ctx).Policy(tenant)
	if err != nil {
		return "", nil, nil, err
	}
	if !policy.DataExport.Enabled {
		return "", nil, nil, ErrTenantDataActionDenied
	}
	before, err := sensitiveSnapshot(struct {
		TenantID     uint64 `json:"tenant_id"`
		TenantCode   string `json:"tenant_code"`
		Dataset      string `json:"dataset"`
		Format       string `json:"format"`
		FilterDigest string `json:"filter_digest"`
		Policy       string `json:"policy_version"`
	}{tenant.ID, tenant.Code, parsed.Dataset, parsed.Format, parsed.FilterDigest, tenantGovernancePolicyVersion(policy)})
	if err != nil {
		return "", nil, nil, err
	}
	return strings.TrimSpace(selector), []string{before}, []string{"tenant-export:queued"}, nil
}

func parseTenantExportSelector(value string) (tenantExportSelector, error) {
	const prefix = "tenant-data:export:"
	value = strings.TrimSpace(value)
	parts := strings.Split(strings.TrimPrefix(value, prefix), ":")
	if !strings.HasPrefix(value, prefix) || len(parts) != 5 || parts[3] != "sha256" {
		return tenantExportSelector{}, ErrSensitiveOperationPolicy
	}
	tenantID, err := strconv.ParseUint(parts[0], 10, 64)
	parsed := tenantExportSelector{TenantID: tenantID, Dataset: parts[1], Format: parts[2], FilterDigest: "sha256:" + parts[4]}
	if err != nil || tenantID == 0 || !tenantExportDatasetAllowed(parsed.Dataset) || !tenantExportFormatAllowed(parsed.Format) || !isSHA256(parsed.FilterDigest) {
		return tenantExportSelector{}, ErrSensitiveOperationPolicy
	}
	return parsed, nil
}

func tenantExportResource(tenantID uint64, dataset, format, filterDigest string) string {
	return fmt.Sprintf("tenant-data:export:%d:%s:%s:%s", tenantID, dataset, format, filterDigest)
}

func tenantExportDatasetAllowed(dataset string) bool { return dataset == "users" }

func tenantExportFormatAllowed(format string) bool { return format == "jsonl" || format == "csv" }
