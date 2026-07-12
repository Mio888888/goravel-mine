package services

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
)

func resolveRBACSensitivePlan(ctx context.Context, policyKey string, tenantID uint64, selector string) (string, []string, []string, error) {
	switch policyKey {
	case "user.password.reset":
		resource, userID, err := parseRBACPasswordResetSelector(selector)
		if err != nil {
			return "", nil, nil, err
		}
		if err := requireRBACUser(ctx, tenantID, userID); err != nil {
			return "", nil, nil, err
		}
		before, after, err := rbacPasswordSnapshots(userID, true)
		return resource, before, after, err
	case "user.roles.sync":
		resource, userID, roles, err := parseRBACUserRolesSelector(selector)
		if err != nil {
			return "", nil, nil, err
		}
		before, err := rbacUserRolesSnapshot(ctx, tenantID, userID)
		if err != nil {
			return "", nil, nil, err
		}
		after, err := rbacSnapshot("user_roles", userID, roles)
		return resource, before, after, err
	case "role.permissions.sync":
		resource, roleID, permissions, err := parseRBACRolePermissionsSelector(selector)
		if err != nil {
			return "", nil, nil, err
		}
		before, err := rbacRolePermissionsSnapshot(ctx, tenantID, roleID)
		if err != nil {
			return "", nil, nil, err
		}
		after, err := rbacSnapshot("role_permissions", roleID, permissions)
		return resource, before, after, err
	default:
		return "", nil, nil, ErrSensitiveOperationPolicy
	}
}

func rbacUserRolesSelector(userID uint64, roleCodes []string) (string, error) {
	return rbacSelector("user", userID, "roles", roleCodes)
}

func rbacRolePermissionsSelector(roleID uint64, permissions []string) (string, error) {
	return rbacSelector("role", roleID, "permissions", permissions)
}

func rbacPasswordResetSelector(userID uint64) string {
	return "rbac:user:" + strconv.FormatUint(userID, 10) + ":password:reset"
}

func parseRBACUserRolesSelector(selector string) (string, uint64, []string, error) {
	resource, id, desired, err := parseRBACSelector(selector, "user", "roles")
	return resource, id, desired, err
}

func parseRBACRolePermissionsSelector(selector string) (string, uint64, []string, error) {
	resource, id, desired, err := parseRBACSelector(selector, "role", "permissions")
	return resource, id, desired, err
}

func parseRBACPasswordResetSelector(selector string) (string, uint64, error) {
	parts := strings.Split(strings.TrimSpace(selector), ":")
	if len(parts) != 5 || parts[0] != "rbac" || parts[1] != "user" || parts[3] != "password" || parts[4] != "reset" {
		return "", 0, ErrSensitiveOperationPolicy
	}
	userID, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil || userID == 0 {
		return "", 0, ErrSensitiveOperationPolicy
	}
	return rbacPasswordResetSelector(userID), userID, nil
}

func rbacSelector(subject string, id uint64, operation string, desired []string) (string, error) {
	if id == 0 {
		return "", ErrSensitiveOperationPolicy
	}
	canonical := canonicalRBACValues(desired)
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"rbac", subject, strconv.FormatUint(id, 10), operation,
		base64.RawURLEncoding.EncodeToString(payload),
	}, ":"), nil
}

func parseRBACSelector(selector, subject, operation string) (string, uint64, []string, error) {
	parts := strings.SplitN(strings.TrimSpace(selector), ":", 5)
	if len(parts) != 5 || parts[0] != "rbac" || parts[1] != subject || parts[3] != operation {
		return "", 0, nil, ErrSensitiveOperationPolicy
	}
	id, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil || id == 0 {
		return "", 0, nil, ErrSensitiveOperationPolicy
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil {
		return "", 0, nil, ErrSensitiveOperationPolicy
	}
	desired := make([]string, 0)
	if json.Unmarshal(payload, &desired) != nil || !sameRBACValues(desired, canonicalRBACValues(desired)) {
		return "", 0, nil, ErrSensitiveOperationPolicy
	}
	digest := sha256.Sum256(payload)
	resource := strings.Join([]string{
		"rbac", subject, strconv.FormatUint(id, 10), operation, hex.EncodeToString(digest[:]),
	}, ":")
	return resource, id, desired, nil
}

func canonicalRBACValues(values []string) []string {
	return canonicalSnapshot(values)
}

func sameRBACValues(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

func rbacPasswordSnapshots(userID uint64, configured bool) ([]string, []string, error) {
	state := "absent"
	if configured {
		state = "configured"
	}
	before, err := rbacSnapshot("user_password", userID, []string{state})
	if err != nil {
		return nil, nil, err
	}
	after, err := rbacSnapshot("user_password", userID, []string{"reset"})
	if err != nil {
		return nil, nil, err
	}
	return before, after, nil
}

func rbacUserRolesSnapshot(ctx context.Context, tenantID, userID uint64) ([]string, error) {
	roles := make([]string, 0)
	roleTable, relationTable := "role", "user_belongs_role"
	if tenantID == 0 {
		roleTable, relationTable = "platform_role", "platform_user_belongs_role"
	}
	err := OrmForConnectionWithContext(ctx, rbacConnection(ctx, tenantID)).Query().
		Table(roleTable).
		Select(roleTable+".code").
		Join("JOIN "+relationTable+" ubr ON ubr.role_id = "+roleTable+".id").
		Where("ubr.user_id", userID).
		OrderBy(roleTable+".code").
		Pluck(roleTable+".code", &roles)
	if err != nil {
		return nil, err
	}
	return rbacSnapshot("user_roles", userID, roles)
}

func rbacRolePermissionsSnapshot(ctx context.Context, tenantID, roleID uint64) ([]string, error) {
	permissions := make([]string, 0)
	menuTable, relationTable := "menu", "role_belongs_menu"
	if tenantID == 0 {
		menuTable, relationTable = "platform_menu", "platform_role_belongs_menu"
	}
	err := OrmForConnectionWithContext(ctx, rbacConnection(ctx, tenantID)).Query().
		Table(menuTable).
		Select(menuTable+".name").
		Join("JOIN "+relationTable+" rbm ON rbm.menu_id = "+menuTable+".id").
		Where("rbm.role_id", roleID).
		OrderBy(menuTable+".name").
		Pluck(menuTable+".name", &permissions)
	if err != nil {
		return nil, err
	}
	return rbacSnapshot("role_permissions", roleID, permissions)
}

func requireRBACUser(ctx context.Context, tenantID, userID uint64) error {
	count, err := OrmForConnectionWithContext(ctx, rbacConnection(ctx, tenantID)).Query().
		Table(rbacUserTable(tenantID)).Where("id", userID).Count()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrSensitiveOperationPolicy
	}
	return nil
}

func rbacConnection(ctx context.Context, tenantID uint64) string {
	if tenantID == 0 {
		return PlatformConnection()
	}
	return TenantConnectionFromContext(ctx)
}

func rbacUserTable(tenantID uint64) string {
	if tenantID == 0 {
		return "platform_user"
	}
	return "user"
}

func (s *PermissionAdminService) ResetPasswordSensitive(actorID, userID uint64, evidence SensitiveOperationEvidence) error {
	return executeRBACSensitive(s.ctx, "user.password.reset", actorID, s.tenant.ID, rbacPasswordResetSelector(userID), evidence, func() error {
		return s.ResetPassword(userID)
	})
}

func (s *PermissionAdminService) SyncUserRolesSensitive(actorID, userID uint64, roleCodes []string, evidence SensitiveOperationEvidence) error {
	selector, err := rbacUserRolesSelector(userID, roleCodes)
	if err != nil {
		return err
	}
	return executeRBACSensitive(s.ctx, "user.roles.sync", actorID, s.tenant.ID, selector, evidence, func() error {
		return s.SyncUserRoles(userID, roleCodes)
	})
}

func (s *PermissionAdminService) SyncRolePermissionsSensitive(actorID, roleID uint64, permissions []string, evidence SensitiveOperationEvidence) error {
	selector, err := rbacRolePermissionsSelector(roleID, permissions)
	if err != nil {
		return err
	}
	return executeRBACSensitive(s.ctx, "role.permissions.sync", actorID, s.tenant.ID, selector, evidence, func() error {
		return s.SyncRolePermissions(roleID, permissions)
	})
}

func (s *PlatformPermissionAdminService) ResetPasswordSensitive(actorID, userID uint64, evidence SensitiveOperationEvidence) error {
	return executeRBACSensitive(s.ctx, "user.password.reset", actorID, 0, rbacPasswordResetSelector(userID), evidence, func() error {
		return s.ResetPassword(userID)
	})
}

func (s *PlatformPermissionAdminService) SyncUserRolesSensitive(actorID, userID uint64, roleCodes []string, evidence SensitiveOperationEvidence) error {
	selector, err := rbacUserRolesSelector(userID, roleCodes)
	if err != nil {
		return err
	}
	return executeRBACSensitive(s.ctx, "user.roles.sync", actorID, 0, selector, evidence, func() error {
		return s.SyncUserRoles(userID, roleCodes)
	})
}

func (s *PlatformPermissionAdminService) SyncRolePermissionsSensitive(actorID, roleID uint64, permissions []string, evidence SensitiveOperationEvidence) error {
	selector, err := rbacRolePermissionsSelector(roleID, permissions)
	if err != nil {
		return err
	}
	return executeRBACSensitive(s.ctx, "role.permissions.sync", actorID, 0, selector, evidence, func() error {
		return s.SyncRolePermissions(roleID, permissions)
	})
}

func executeRBACSensitive(ctx context.Context, policyKey string, actorID, tenantID uint64, selector string, evidence SensitiveOperationEvidence, mutate func() error) error {
	guard := NewSensitiveOperationGuard(nil)
	plan, err := guard.PrepareCanonical(ctx, policyKey, actorID, tenantID, SensitiveOperationPlanSelector{Resource: selector})
	if err != nil {
		return err
	}
	return guard.Execute(ctx, plan, evidence, mutate)
}

func rbacSnapshot(kind string, id uint64, values []string) ([]string, error) {
	raw, err := json.Marshal(struct {
		Kind   string   `json:"kind"`
		ID     uint64   `json:"id"`
		Values []string `json:"values"`
	}{Kind: kind, ID: id, Values: canonicalRBACValues(values)})
	if err != nil {
		return nil, err
	}
	return []string{string(raw)}, nil
}
