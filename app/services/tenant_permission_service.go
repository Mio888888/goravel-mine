package services

import (
	"sort"
	"strings"

	"goravel/app/models"
	"goravel/database/seeders"
)

const tenantPermissionsFeatureKey = "permissions"

type TenantPermissionPayload struct {
	Allowed []string `json:"allowed"`
}

type TenantPermissionPlanDiff struct {
	Plan       string   `json:"plan"`
	Allowed    []string `json:"allowed"`
	Added      []string `json:"added"`
	Removed    []string `json:"removed"`
	Unchanged  []string `json:"unchanged"`
	Permission []string `json:"permission"`
}

type TenantPermissionSnapshot struct {
	LegacyFullAccess bool
	Allowed          map[string]struct{}
}

func TenantAllowsRoute(tenant Tenant, method, path string) bool {
	permission := PermissionForRoute(method, path)
	if permission == "" {
		return true
	}
	return TenantAllowsPermission(tenant, permission)
}

func TenantAllowsPermission(tenant Tenant, permission string) bool {
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return true
	}
	snapshot := TenantPermissionSnapshotFromFeatures(tenant.Features)
	if snapshot.LegacyFullAccess {
		return true
	}
	if _, allowed := snapshot.Allowed[permission]; !allowed {
		return false
	}
	return tenantPermissionAncestorsAllowed(snapshot.Allowed, permission)
}

func TenantAllowedPermissionNames(tenant Tenant) []string {
	snapshot := TenantPermissionSnapshotFromFeatures(tenant.Features)
	if snapshot.LegacyFullAccess {
		return nil
	}
	names := make([]string, 0, len(snapshot.Allowed))
	for name := range snapshot.Allowed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TenantFullPermissionNames() []string {
	names := make([]string, 0)
	for _, menu := range flattenAdminMenus(TenantPermissionCatalogMenus()) {
		names = append(names, menu.Name)
	}
	for _, permission := range routePermissionMap() {
		if strings.HasPrefix(permission, "platform:") {
			continue
		}
		names = append(names, permission)
	}
	return normalizeStrings(names)
}

func TenantPermissionPayloadFromTenant(tenant Tenant) TenantPermissionPayload {
	payload, _ := tenantPermissionPayloadFromFeatures(tenant.Features)
	return normalizePermissionPayload(payload)
}

func TenantEffectivePermissionPayload(tenant Tenant) TenantPermissionPayload {
	if TenantPermissionSnapshotFromFeatures(tenant.Features).LegacyFullAccess {
		return TenantPermissionPayload{
			Allowed: TenantFullPermissionNames(),
		}
	}
	return TenantPermissionPayloadFromTenant(tenant)
}

func TenantPermissionSnapshotFromFeatures(features models.JSONMap) TenantPermissionSnapshot {
	payload, ok := tenantPermissionPayloadFromFeatures(features)
	if !ok {
		return TenantPermissionSnapshot{
			LegacyFullAccess: true,
			Allowed:          map[string]struct{}{},
		}
	}
	return TenantPermissionSnapshot{
		Allowed: stringSet(payload.Allowed),
	}
}

func SnapshotFeaturesForPlan(planFeatures, input models.JSONMap) models.JSONMap {
	features := platformManagedFeatures(input)
	if planPayload, ok := tenantPermissionPayloadFromFeatures(planFeatures); ok {
		features[tenantPermissionsFeatureKey] = permissionPayloadMap(planPayload)
	}
	if inputPayload, ok := tenantPermissionPayloadFromFeatures(input); ok {
		features[tenantPermissionsFeatureKey] = permissionPayloadMap(inputPayload)
	}
	return features
}

func featuresWithoutTenantPermissions(input models.JSONMap) models.JSONMap {
	features := platformManagedFeatures(input)
	delete(features, tenantPermissionsFeatureKey)
	return features
}

func preserveTenantPermissionFeature(features, existing models.JSONMap) models.JSONMap {
	if features == nil {
		features = models.JSONMap{}
	}
	if existing == nil {
		delete(features, tenantPermissionsFeatureKey)
		return features
	}
	if raw, ok := existing[tenantPermissionsFeatureKey]; ok {
		features[tenantPermissionsFeatureKey] = raw
		return features
	}
	delete(features, tenantPermissionsFeatureKey)
	return features
}

func FilterAdminMenusByTenantPermissions(tenant Tenant, menus []AdminMenuItem) []AdminMenuItem {
	if TenantPermissionSnapshotFromFeatures(tenant.Features).LegacyFullAccess {
		return menus
	}
	allowedIDs := tenantAllowedMenuIDs(tenant, menus)
	filtered := make([]AdminMenuItem, 0, len(allowedIDs))
	for _, menu := range menus {
		if _, ok := allowedIDs[menu.ID]; ok {
			filtered = append(filtered, menu)
		}
	}
	return filtered
}

func TenantPermissionCatalogMenus() []AdminMenuItem {
	seeds := seeders.TenantMenuCatalogSeeds()
	menus := make([]AdminMenuItem, 0, len(seeds))
	for _, seed := range seeds {
		menus = append(menus, AdminMenuItem{
			ID:        seed.ID,
			ParentID:  seed.ParentID,
			Name:      seed.Name,
			Path:      seed.Path,
			Component: seed.Component,
			Redirect:  seed.Redirect,
			Status:    1,
			Sort:      int16(seed.Sort),
			Meta:      models.JSONMap(seed.Meta),
		})
	}
	return buildAdminMenuTree(menus, 0)
}

func BuildLegacyTenantPermissionSnapshot(tenant Tenant) (TenantPermissionPayload, bool) {
	if _, ok := tenantPermissionPayloadFromFeatures(tenant.Features); ok {
		return TenantPermissionPayload{}, false
	}
	return TenantPermissionPayload{
		Allowed: TenantFullPermissionNames(),
	}, true
}

func ValidateTenantRolePermissions(tenant Tenant, permissions []string) error {
	if TenantPermissionSnapshotFromFeatures(tenant.Features).LegacyFullAccess {
		return nil
	}
	for _, permission := range permissions {
		if !TenantAllowsPermission(tenant, permission) {
			return BusinessError{Message: "角色权限超出租户授权范围"}
		}
	}
	return nil
}

func flattenAdminMenus(menus []AdminMenuItem) []AdminMenuItem {
	flattened := make([]AdminMenuItem, 0, len(menus))
	for _, menu := range menus {
		flattened = append(flattened, menu)
		flattened = append(flattened, flattenAdminMenus(menu.Children)...)
	}
	return flattened
}

func adminMenuIDs(menus []AdminMenuItem) []uint64 {
	ids := make([]uint64, 0, len(menus))
	for _, menu := range menus {
		ids = append(ids, menu.ID)
	}
	return ids
}

func tenantAllowedMenuIDs(tenant Tenant, menus []AdminMenuItem) map[uint64]struct{} {
	byID := make(map[uint64]AdminMenuItem, len(menus))
	allowed := make(map[uint64]struct{})
	for _, menu := range menus {
		byID[menu.ID] = menu
		if TenantAllowsPermission(tenant, menu.Name) {
			allowed[menu.ID] = struct{}{}
		}
	}
	for id := range allowed {
		parentID := byID[id].ParentID
		for parentID != 0 {
			parent, ok := byID[parentID]
			if !ok {
				break
			}
			allowed[parent.ID] = struct{}{}
			parentID = parent.ParentID
		}
	}
	return allowed
}

func tenantPermissionAncestorsAllowed(allowed map[string]struct{}, permission string) bool {
	parentByName := tenantPermissionParentByName()
	parent, ok := parentByName[permission]
	for ok && parent != "" {
		if _, exists := allowed[parent]; !exists {
			return false
		}
		parent, ok = parentByName[parent]
	}
	return true
}

func tenantPermissionParentByName() map[string]string {
	menus := flattenAdminMenus(TenantPermissionCatalogMenus())
	byID := make(map[uint64]AdminMenuItem, len(menus))
	for _, menu := range menus {
		byID[menu.ID] = menu
	}

	parentByName := make(map[string]string, len(menus))
	for _, menu := range menus {
		if menu.Name == "" || menu.ParentID == 0 {
			continue
		}
		parent, ok := byID[menu.ParentID]
		if !ok || parent.Name == "" {
			continue
		}
		parentByName[menu.Name] = parent.Name
	}
	return parentByName
}

func tenantPermissionPayloadFromFeatures(features models.JSONMap) (TenantPermissionPayload, bool) {
	if features == nil {
		return TenantPermissionPayload{}, false
	}
	raw, ok := features[tenantPermissionsFeatureKey]
	if !ok || raw == nil {
		return TenantPermissionPayload{}, false
	}
	payload := TenantPermissionPayload{
		Allowed: stringListFromMap(raw, "allowed"),
	}
	return payload, true
}

func permissionPayloadMap(payload TenantPermissionPayload) models.JSONMap {
	payload = normalizePermissionPayload(payload)
	return models.JSONMap{
		"allowed": payload.Allowed,
	}
}

func stringListFromMap(raw any, key string) []string {
	values, ok := raw.(map[string]any)
	if !ok {
		if jsonValues, ok := raw.(models.JSONMap); ok {
			values = map[string]any(jsonValues)
		} else {
			return nil
		}
	}
	return normalizeStringSlice(values[key])
}

func normalizeStringSlice(raw any) []string {
	switch values := raw.(type) {
	case []string:
		return normalizeStrings(values)
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return normalizeStrings(out)
	default:
		return nil
	}
}

func normalizeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizePermissionPayload(payload TenantPermissionPayload) TenantPermissionPayload {
	return TenantPermissionPayload{
		Allowed: normalizeStrings(payload.Allowed),
	}
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range normalizeStrings(values) {
		set[value] = struct{}{}
	}
	return set
}

func sortedSetDiff(left, right []string) []string {
	rightSet := stringSet(right)
	out := make([]string, 0)
	for _, value := range normalizeStrings(left) {
		if _, ok := rightSet[value]; !ok {
			out = append(out, value)
		}
	}
	return out
}

func sortedSetIntersect(left, right []string) []string {
	rightSet := stringSet(right)
	out := make([]string, 0)
	for _, value := range normalizeStrings(left) {
		if _, ok := rightSet[value]; ok {
			out = append(out, value)
		}
	}
	return out
}
