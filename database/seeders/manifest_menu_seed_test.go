package seeders

import "testing"

func TestParentMenuForPermissionUsesLongestMatchingMenu(t *testing.T) {
	parent, ok := parentMenuForPermission(ManifestPermission{Key: "audit-log:item:list"}, []ManifestMenu{
		{Key: "audit-log"},
		{Key: "audit-log:item"},
	})

	if !ok {
		t.Fatal("expected matching parent menu")
	}
	if parent != "audit-log:item" {
		t.Fatalf("parentMenuForPermission() = %q, want audit-log:item", parent)
	}
}

func TestParentMenuForPermissionDoesNotFallbackToFirstMenu(t *testing.T) {
	parent, ok := parentMenuForPermission(ManifestPermission{Key: "log:ssoLogin:list"}, []ManifestMenu{
		{Key: "security:ssoProvider"},
		{Key: "security:ssoUserBinding"},
	})

	if ok {
		t.Fatalf("parentMenuForPermission() matched %q, want no match", parent)
	}
}

func TestMenuCatalogSeedsIncludeMFAPermissions(t *testing.T) {
	if !catalogContains(TenantMenuCatalogSeeds(), "security:mfa") {
		t.Fatal("tenant menu seeds missing security:mfa")
	}
	if !catalogContains(PlatformMenuCatalogSeeds(), "platform:security:mfa") {
		t.Fatal("platform menu seeds missing platform:security:mfa")
	}
	if !catalogContains(PlatformMenuCatalogSeeds(), "platform:security:control") {
		t.Fatal("platform menu seeds missing platform:security:control")
	}
}

func TestPlatformMenuCatalogSeedIDsAreUnique(t *testing.T) {
	seen := make(map[uint64]string)
	for _, item := range PlatformMenuCatalogSeeds() {
		if previous, ok := seen[item.ID]; ok {
			t.Fatalf("duplicate platform menu seed id %d for %s and %s", item.ID, previous, item.Name)
		}
		seen[item.ID] = item.Name
	}
}

func catalogContains(items []MenuCatalogSeed, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}

	return false
}
