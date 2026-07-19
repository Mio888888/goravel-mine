package permission

import (
	"github.com/goravel/framework/support/collect"

	"goravel/app/support/apperror"
)

type PolicyType string

const (
	PolicyAll        PolicyType = "ALL"
	PolicyDeptSelf   PolicyType = "DEPT_SELF"
	PolicyDeptTree   PolicyType = "DEPT_TREE"
	PolicySelf       PolicyType = "SELF"
	PolicyCustomDept PolicyType = "CUSTOM_DEPT"
	PolicyCustomFunc PolicyType = "CUSTOM_FUNC"
)

type DataPolicy struct {
	Type    PolicyType
	DeptIDs []uint64
}

type DataScopeContext struct {
	UserID      uint64
	OwnerColumn string
	DeptColumn  string
}

type DataScope struct {
	Condition string
	Args      []any
}

func ResolveDataPolicy(userPolicy DataPolicy, positionPolicies []DataPolicy) DataPolicy {
	if userPolicy.Type != "" {
		return userPolicy
	}
	if len(positionPolicies) > 0 {
		return positionPolicies[0]
	}
	return DataPolicy{Type: PolicyAll}
}

func BuildDataScope(policy DataPolicy, ctx DataScopeContext) (DataScope, error) {
	switch policy.Type {
	case "", PolicyAll:
		return DataScope{}, nil
	case PolicySelf:
		column := ctx.OwnerColumn
		if column == "" {
			column = "created_by"
		}
		return DataScope{Condition: column + " = ?", Args: []any{ctx.UserID}}, nil
	case PolicyDeptSelf, PolicyDeptTree, PolicyCustomDept:
		column := ctx.DeptColumn
		if column == "" {
			column = "dept_id"
		}
		if len(policy.DeptIDs) == 0 {
			return DataScope{Condition: "1 = 0"}, nil
		}
		return DataScope{Condition: column + " IN ?", Args: []any{uint64Any(policy.DeptIDs)}}, nil
	case PolicyCustomFunc:
		return DataScope{}, apperror.BusinessError{Message: "自定义数据权限函数未注册"}
	default:
		return DataScope{}, apperror.BusinessError{Message: "未知数据权限策略"}
	}
}

func uint64Any(values []uint64) []any {
	return collect.Map(values, func(value uint64, _ int) any {
		return value
	})
}
