package seeders

type DictionarySeeder struct{}

type dictTypeSeed struct {
	ID     uint64
	Code   string
	Name   string
	Sort   int
	Remark string
	Items  []dictItemSeed
}

type dictItemSeed struct {
	ID     uint64
	Label  string
	Value  string
	I18n   string
	Color  string
	Sort   int
	Remark string
}

func (s *DictionarySeeder) Signature() string {
	return "dictionary_seed"
}

func (s *DictionarySeeder) Run() error {
	for _, item := range dictionarySeeds() {
		if err := seedDictType(item); err != nil {
			return err
		}
		for _, dictItem := range item.Items {
			if err := seedDictItem(item, dictItem); err != nil {
				return err
			}
		}
	}
	if err := syncSequence("dict_type", "id"); err != nil {
		return err
	}
	return syncSequence("dict_item", "id")
}

func seedDictType(item dictTypeSeed) error {
	return exec(`
		INSERT INTO dict_type (
			id, source_id, source_code, code, name, status, sort, version, is_system,
			created_by, updated_by, created_at, updated_at, remark
		)
		VALUES (?, ?, ?, ?, ?, 1, ?, 1, true, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?)
		ON CONFLICT (code) DO UPDATE SET
			source_id = EXCLUDED.source_id,
			source_code = EXCLUDED.source_code,
			version = EXCLUDED.version,
			is_system = EXCLUDED.is_system,
			updated_at = CURRENT_TIMESTAMP
	`, item.ID, item.ID, item.Code, item.Code, item.Name, item.Sort, item.Remark)
}

func seedDictItem(dictType dictTypeSeed, item dictItemSeed) error {
	return exec(`
		INSERT INTO dict_item (
			id, type_id, source_id, source_code, type_code, label, value, i18n, color,
			status, sort, version, is_system, created_by, updated_by, created_at, updated_at, remark
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, 1, true, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?)
		ON CONFLICT (type_code, value) DO UPDATE SET
			type_id = EXCLUDED.type_id,
			source_id = EXCLUDED.source_id,
			source_code = EXCLUDED.source_code,
			version = EXCLUDED.version,
			is_system = EXCLUDED.is_system,
			updated_at = CURRENT_TIMESTAMP
	`, item.ID, dictType.ID, item.ID, dictType.Code+":"+item.Value, dictType.Code, item.Label,
		item.Value, item.I18n, item.Color, item.Sort, item.Remark)
}

type PlatformDictionarySeeder struct{}

func (s *PlatformDictionarySeeder) Signature() string {
	return "platform_dictionary_seed"
}

func (s *PlatformDictionarySeeder) Run() error {
	for _, item := range dictionarySeeds() {
		if err := seedPlatformDictType(item); err != nil {
			return err
		}
		for _, dictItem := range item.Items {
			if err := seedPlatformDictItem(item, dictItem); err != nil {
				return err
			}
		}
	}
	if err := syncSequence("platform_dict_type", "id"); err != nil {
		return err
	}
	return syncSequence("platform_dict_item", "id")
}

func seedPlatformDictType(item dictTypeSeed) error {
	return exec(`
		INSERT INTO platform_dict_type (
			id, code, name, status, sort, version, is_system,
			created_by, updated_by, created_at, updated_at, remark
		)
		VALUES (?, ?, ?, 1, ?, 1, true, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?)
		ON CONFLICT (code) DO UPDATE SET
			name = EXCLUDED.name,
			status = EXCLUDED.status,
			sort = EXCLUDED.sort,
			version = platform_dict_type.version + 1,
			is_system = EXCLUDED.is_system,
			updated_at = CURRENT_TIMESTAMP,
			remark = EXCLUDED.remark
	`, item.ID, item.Code, item.Name, item.Sort, item.Remark)
}

func seedPlatformDictItem(dictType dictTypeSeed, item dictItemSeed) error {
	return exec(`
		INSERT INTO platform_dict_item (
			id, type_id, type_code, label, value, i18n, color,
			status, sort, version, is_system, created_by, updated_by, created_at, updated_at, remark
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, 1, true, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?)
		ON CONFLICT (type_code, value) DO UPDATE SET
			type_id = EXCLUDED.type_id,
			label = EXCLUDED.label,
			i18n = EXCLUDED.i18n,
			color = EXCLUDED.color,
			status = EXCLUDED.status,
			sort = EXCLUDED.sort,
			version = platform_dict_item.version + 1,
			is_system = EXCLUDED.is_system,
			updated_at = CURRENT_TIMESTAMP,
			remark = EXCLUDED.remark
	`, item.ID, dictType.ID, dictType.Code, item.Label, item.Value, item.I18n, item.Color, item.Sort, item.Remark)
}

func dictionarySeeds() []dictTypeSeed {
	return []dictTypeSeed{
		{
			ID:   1,
			Code: "system-status",
			Name: "系统状态",
			Sort: 10,
			Items: []dictItemSeed{
				{ID: 1, Label: "启用", Value: "1", I18n: "dictionary.system.statusEnabled", Color: "primary", Sort: 10},
				{ID: 2, Label: "禁用", Value: "2", I18n: "dictionary.system.statusDisabled", Color: "danger", Sort: 20},
			},
		},
		{
			ID:   2,
			Code: "system-state",
			Name: "系统结果",
			Sort: 20,
			Items: []dictItemSeed{
				{ID: 3, Label: "成功", Value: "1", I18n: "dictionary.system.successState", Color: "success", Sort: 10},
				{ID: 4, Label: "失败", Value: "2", I18n: "dictionary.system.failState", Color: "danger", Sort: 20},
			},
		},
		{
			ID:   3,
			Code: "base-userType",
			Name: "用户类型",
			Sort: 30,
			Items: []dictItemSeed{
				{ID: 5, Label: "系统用户", Value: "100", I18n: "dictionary.base.systemUser", Color: "primary", Sort: 10},
				{ID: 6, Label: "普通用户", Value: "200", I18n: "dictionary.base.normalUser", Color: "success", Sort: 20},
			},
		},
		{
			ID:   4,
			Code: "tenant-status",
			Name: "租户状态",
			Sort: 40,
			Items: []dictItemSeed{
				{ID: 7, Label: "正常", Value: "1", I18n: "dictionary.tenant.statusActive", Color: "success", Sort: 10},
				{ID: 8, Label: "挂起", Value: "2", I18n: "dictionary.tenant.statusSuspended", Color: "warning", Sort: 20},
				{ID: 9, Label: "归档", Value: "3", I18n: "dictionary.tenant.statusArchived", Color: "info", Sort: 30},
			},
		},
		{
			ID:   5,
			Code: "data-scope",
			Name: "数据权限范围",
			Sort: 50,
			Items: []dictItemSeed{
				{ID: 10, Label: "全部数据权限", Value: "ALL", I18n: "dictionary.dataScope.all", Color: "primary", Sort: 10},
				{ID: 11, Label: "本部门数据权限", Value: "DEPT_SELF", I18n: "dictionary.dataScope.deptSelf", Color: "primary", Sort: 20},
				{ID: 12, Label: "本部门及所有子部门数据权限", Value: "DEPT_TREE", I18n: "dictionary.dataScope.deptTree", Color: "primary", Sort: 30},
				{ID: 13, Label: "本人数据权限", Value: "SELF", I18n: "dictionary.dataScope.self", Color: "primary", Sort: 40},
				{ID: 14, Label: "自选部门数据权限", Value: "CUSTOM_DEPT", I18n: "dictionary.dataScope.customDept", Color: "primary", Sort: 50},
				{ID: 15, Label: "自定义函数数据权限", Value: "CUSTOM_FUNC", I18n: "dictionary.dataScope.customFunc", Color: "primary", Sort: 60},
			},
		},
		{
			ID:   1001,
			Code: "sso-provider-type",
			Name: "SSO Provider 类型",
			Sort: 60,
			Items: []dictItemSeed{
				{ID: 10001, Label: "OIDC", Value: "oidc", I18n: "dictionary.ssoProvider.typeOidc", Color: "primary", Sort: 10},
				{ID: 10002, Label: "OAuth2", Value: "oauth2", I18n: "dictionary.ssoProvider.typeOauth2", Color: "success", Sort: 20},
				{ID: 10003, Label: "SAML", Value: "saml", I18n: "dictionary.ssoProvider.typeSaml", Color: "warning", Sort: 30},
			},
		},
		{
			ID:   1002,
			Code: "sso-provider-scene",
			Name: "SSO Provider 场景",
			Sort: 70,
			Items: []dictItemSeed{
				{ID: 10004, Label: "管理后台", Value: "admin", I18n: "dictionary.ssoProvider.sceneAdmin", Color: "primary", Sort: 10},
				{ID: 10005, Label: "移动端", Value: "mobile", I18n: "dictionary.ssoProvider.sceneMobile", Color: "success", Sort: 20},
				{ID: 10006, Label: "门户端", Value: "portal", I18n: "dictionary.ssoProvider.scenePortal", Color: "warning", Sort: 30},
			},
		},
		{
			ID:   1003,
			Code: "system-enabled",
			Name: "启用状态",
			Sort: 80,
			Items: []dictItemSeed{
				{ID: 10007, Label: "启用", Value: "true", I18n: "dictionary.system.statusEnabled", Color: "success", Sort: 10},
				{ID: 10008, Label: "禁用", Value: "false", I18n: "dictionary.system.statusDisabled", Color: "danger", Sort: 20},
			},
		},
		{
			ID:   1004,
			Code: "tenant-subscription-status",
			Name: "租户订阅状态",
			Sort: 90,
			Items: []dictItemSeed{
				{ID: 10009, Label: "Active", Value: "active", I18n: "dictionary.tenantSubscription.active", Color: "success", Sort: 10},
				{ID: 10010, Label: "Trialing", Value: "trialing", I18n: "dictionary.tenantSubscription.trialing", Color: "primary", Sort: 20},
				{ID: 10011, Label: "Past Due", Value: "past_due", I18n: "dictionary.tenantSubscription.pastDue", Color: "warning", Sort: 30},
				{ID: 10012, Label: "Canceled", Value: "canceled", I18n: "dictionary.tenantSubscription.canceled", Color: "danger", Sort: 40},
				{ID: 10013, Label: "Expired", Value: "expired", I18n: "dictionary.tenantSubscription.expired", Color: "info", Sort: 50},
			},
		},
		{
			ID:   1005,
			Code: "menu-type",
			Name: "菜单类型",
			Sort: 100,
			Items: []dictItemSeed{
				{ID: 10014, Label: "菜单", Value: "M", I18n: "dictionary.menuType.menu", Color: "primary", Sort: 10},
				{ID: 10015, Label: "按钮", Value: "B", I18n: "dictionary.menuType.button", Color: "danger", Sort: 20},
				{ID: 10016, Label: "外链", Value: "L", I18n: "dictionary.menuType.link", Color: "success", Sort: 30},
				{ID: 10017, Label: "iFrame", Value: "I", I18n: "dictionary.menuType.iframe", Color: "warning", Sort: 40},
			},
		},
	}
}
