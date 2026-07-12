package unit

import (
	"testing"

	"goravel/bootstrap"
)

func TestModuleRegistryInstallsScheduledTask(t *testing.T) {
	registry := bootstrap.Modules()

	hasScheduledTask := false
	hasPlatformTenant := false
	hasPlatformObservability := false
	hasPlatformRBAC := false
	hasTenantRBAC := false
	hasSecurity := false
	hasDataCenter := false
	for _, id := range registry.IDs() {
		if id == "scheduled-task" {
			hasScheduledTask = true
		}
		if id == "platform-tenant" {
			hasPlatformTenant = true
		}
		if id == "platform-observability" {
			hasPlatformObservability = true
		}
		if id == "platform-rbac" {
			hasPlatformRBAC = true
		}
		if id == "tenant-rbac" {
			hasTenantRBAC = true
		}
		if id == "security" {
			hasSecurity = true
		}
		if id == "data-center" {
			hasDataCenter = true
		}
	}
	if !hasScheduledTask {
		t.Fatal("module registry missing scheduled-task")
	}
	if !hasPlatformTenant {
		t.Fatal("module registry missing platform-tenant")
	}
	if !hasPlatformObservability {
		t.Fatal("module registry missing platform-observability")
	}
	if !hasPlatformRBAC {
		t.Fatal("module registry missing platform-rbac")
	}
	if !hasTenantRBAC {
		t.Fatal("module registry missing tenant-rbac")
	}
	if !hasSecurity {
		t.Fatal("module registry missing security")
	}
	if !hasDataCenter {
		t.Fatal("module registry missing data-center")
	}

	hasScheduledRoute := false
	hasTenantRoute := false
	hasPlatformUserRoute := false
	hasPlatformObservabilityRoute := false
	hasTenantUserRoute := false
	hasSSORoute := false
	hasAttachmentRoute := false
	for _, route := range registry.Routes() {
		if route.Method == "GET" && route.Path == "/admin/platform/scheduled-task/list" {
			hasScheduledRoute = true
		}
		if route.Method == "GET" && route.Path == "/admin/platform/tenant/list" {
			hasTenantRoute = true
		}
		if route.Method == "GET" && route.Path == "/admin/platform/user/list" {
			hasPlatformUserRoute = true
		}
		if route.Method == "GET" && route.Path == "/admin/platform/observability/slow-requests" {
			hasPlatformObservabilityRoute = true
		}
		if route.Method == "GET" && route.Path == "/admin/user/list" {
			hasTenantUserRoute = true
		}
		if route.Method == "GET" && route.Path == "/admin/sso-provider/list" {
			hasSSORoute = true
		}
		if route.Method == "GET" && route.Path == "/admin/attachment/list" {
			hasAttachmentRoute = true
		}
	}
	if !hasScheduledRoute {
		t.Fatal("module registry missing scheduled-task list route")
	}
	if !hasTenantRoute {
		t.Fatal("module registry missing platform tenant list route")
	}
	if !hasPlatformUserRoute {
		t.Fatal("module registry missing platform user list route")
	}
	if !hasPlatformObservabilityRoute {
		t.Fatal("module registry missing platform observability route")
	}
	if !hasTenantUserRoute {
		t.Fatal("module registry missing tenant user list route")
	}
	if !hasSSORoute {
		t.Fatal("module registry missing sso provider list route")
	}
	if !hasAttachmentRoute {
		t.Fatal("module registry missing attachment list route")
	}

	_ = registry.Migrations()
	_ = registry.Seeders()
	if err := registry.Validate(); err != nil {
		t.Fatalf("module registry invalid: %v", err)
	}
}

func TestBootstrapMigrationsKeepModuleSignatureOrder(t *testing.T) {
	migrations := bootstrap.Migrations()
	signatures := make([]string, 0, len(migrations))
	for _, migration := range migrations {
		signatures = append(signatures, migration.Signature())
	}

	storageIndex := indexOfSignature(signatures, "202607030002_add_storage_config_id_to_attachment_table")
	scheduledIndex := indexOfSignature(signatures, "202607040001_create_scheduled_task_tables")
	governanceTaskIndex := indexOfSignature(signatures, "202607110010_upsert_tenant_governance_tasks")
	mfaIndex := indexOfSignature(signatures, "202607050001_create_user_mfa_table")

	if storageIndex == -1 || scheduledIndex == -1 || governanceTaskIndex == -1 || mfaIndex == -1 {
		t.Fatalf("missing expected migration signatures: %#v", signatures)
	}
	if !(storageIndex < scheduledIndex && scheduledIndex < governanceTaskIndex && governanceTaskIndex < mfaIndex) {
		t.Fatalf("scheduled-task migration order invalid: storage=%d scheduled=%d governance=%d mfa=%d", storageIndex, scheduledIndex, governanceTaskIndex, mfaIndex)
	}
}

func indexOfSignature(signatures []string, target string) int {
	for index, signature := range signatures {
		if signature == target {
			return index
		}
	}

	return -1
}
