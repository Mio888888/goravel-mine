package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/models"
)

type ssoMappingResult struct {
	RoleCodes  []string
	DataPolicy *DataPolicy
}

func applySSOMappings(ctx context.Context, connection string, userID uint64, provider SSOProvider, claims ssoClaims) error {
	result := resolveSSOMappings(provider, claims)
	return OrmForConnectionWithContext(ctx, connection).Transaction(func(tx contractsorm.Query) error {
		if result.RoleCodes != nil {
			if err := syncMappedUserRoles(tx, userID, result.RoleCodes); err != nil {
				return err
			}
		}
		if result.DataPolicy != nil {
			if err := syncMappedUserDataPolicy(tx, userID, *result.DataPolicy); err != nil {
				return err
			}
		}
		return nil
	})
}

func resolveSSOMappings(provider SSOProvider, claims ssoClaims) ssoMappingResult {
	return ssoMappingResult{
		RoleCodes:  resolveRoleMapping(provider.RoleMapping, claims.Raw),
		DataPolicy: resolveDataPermissionMapping(provider.DataPermissionMapping, claims.Raw),
	}
}

func resolveRoleMapping(mapping models.JSONMap, claims map[string]any) []string {
	if len(mapping) == 0 {
		return nil
	}
	roleCodes := make([]string, 0)
	claimValues := claimStrings(claims, jsonString(mapping, "claim"))
	rules, _ := asJSONMap(mapping["mapping"])
	for key, raw := range rules {
		if !claimValueMatches(claimValues, key) {
			continue
		}
		roleCodes = append(roleCodes, rolesFromMappingValue(raw, claims)...)
	}
	if len(roleCodes) == 0 {
		roleCodes = roleCodesFromAny(mapping["default"])
	}
	return uniqueStrings(roleCodes)
}

func rolesFromMappingValue(value any, claims map[string]any) []string {
	if rule, ok := asJSONMap(value); ok {
		if condition := strings.TrimSpace(jsonString(rule, "condition")); condition != "" && !evalSSOCondition(condition, claims) {
			return nil
		}
		return roleCodesFromAny(rule["roles"])
	}
	return roleCodesFromAny(value)
}

func resolveDataPermissionMapping(mapping models.JSONMap, claims map[string]any) *DataPolicy {
	if len(mapping) == 0 {
		return nil
	}
	claimValues := claimStrings(claims, jsonString(mapping, "claim"))
	rules, _ := asJSONMap(mapping["mapping"])
	for key, raw := range rules {
		if !claimValueMatches(claimValues, key) {
			continue
		}
		if policy, ok := dataPolicyFromMappingValue(raw, claims); ok {
			return &policy
		}
	}
	if policyType := PolicyType(strings.TrimSpace(jsonString(mapping, "default"))); policyType != "" {
		return &DataPolicy{Type: policyType}
	}
	return nil
}

func dataPolicyFromMappingValue(value any, claims map[string]any) (DataPolicy, bool) {
	if policyType, ok := value.(string); ok {
		policyType = strings.TrimSpace(policyType)
		return DataPolicy{Type: PolicyType(policyType)}, policyType != ""
	}
	rule, ok := asJSONMap(value)
	if !ok {
		return DataPolicy{}, false
	}
	if condition := strings.TrimSpace(jsonString(rule, "condition")); condition != "" && !evalSSOCondition(condition, claims) {
		return DataPolicy{}, false
	}
	policyType := PolicyType(strings.TrimSpace(jsonString(rule, "policy_type")))
	if policyType == "" {
		return DataPolicy{}, false
	}
	return DataPolicy{Type: policyType, DeptIDs: uint64SliceFromAny(rule["value"])}, true
}

func syncMappedUserRoles(tx contractsorm.Query, userID uint64, roleCodes []string) error {
	_, err := tx.Table("user_belongs_role").Where("user_id", userID).Delete()
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("user:%d", userID)
	_, err = tx.Table("casbin_rule").Where("ptype", "g").Where("v0", subject).Delete()
	if err != nil {
		return err
	}
	for _, code := range roleCodes {
		if err := attachMappedUserRole(tx, userID, subject, code); err != nil {
			return err
		}
	}
	return nil
}

func attachMappedUserRole(tx contractsorm.Query, userID uint64, subject, code string) error {
	var role models.Role
	if err := tx.Table("role").Where("code", code).First(&role); err != nil {
		return err
	}
	now := time.Now()
	if err := tx.Table("user_belongs_role").Create(&models.UserBelongsRole{
		UserID: userID, RoleID: role.ID,
		Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
	}); err != nil {
		return err
	}
	return addCasbinRule(tx, "g", subject, "role:"+role.Code, "")
}

func syncMappedUserDataPolicy(tx contractsorm.Query, userID uint64, policy DataPolicy) error {
	if policy.Type == PolicyCustomFunc {
		return BusinessError{Message: "自定义数据权限函数未注册"}
	}
	deptIDs := policy.DeptIDs
	if deptIDs == nil {
		deptIDs = []uint64{}
	}
	encoded, err := json.Marshal(deptIDs)
	if err != nil {
		return err
	}
	_, err = tx.Table("data_permission_policy").Where("user_id", userID).Delete()
	if err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO data_permission_policy (user_id, policy_type, is_default, value, created_at, updated_at)
		VALUES (?, ?, true, ?::jsonb, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, userID, string(policy.Type), string(encoded))
	return err
}

func claimStrings(claims map[string]any, key string) []string {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	return stringsFromAny(claims[key])
}

func claimValueMatches(values []string, expected string) bool {
	expected = strings.TrimSpace(expected)
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}

func roleCodesFromAny(value any) []string {
	return uniqueStrings(stringsFromAny(value))
}

func stringsFromAny(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if typed = strings.TrimSpace(typed); typed != "" {
			return []string{typed}
		}
	case []string:
		return uniqueStrings(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, stringsFromAny(item)...)
		}
		return uniqueStrings(out)
	default:
		text := strings.TrimSpace(fmt.Sprint(typed))
		if text != "" {
			return []string{text}
		}
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func uint64SliceFromAny(value any) []uint64 {
	switch typed := value.(type) {
	case []any:
		out := make([]uint64, 0, len(typed))
		for _, item := range typed {
			if id := uint64FromAny(item); id > 0 {
				out = append(out, id)
			}
		}
		return compactPositiveIDs(out)
	case []uint64:
		return compactPositiveIDs(typed)
	case []int:
		out := make([]uint64, 0, len(typed))
		for _, item := range typed {
			if item > 0 {
				out = append(out, uint64(item))
			}
		}
		return out
	default:
		if id := uint64FromAny(value); id > 0 {
			return []uint64{id}
		}
	}
	return nil
}

func uint64FromAny(value any) uint64 {
	switch typed := value.(type) {
	case float64:
		return uint64(typed)
	case int:
		return uint64(typed)
	case int64:
		return uint64(typed)
	case uint64:
		return typed
	case string:
		parsed, _ := strconv.ParseUint(strings.TrimSpace(typed), 10, 64)
		return parsed
	}
	return 0
}
