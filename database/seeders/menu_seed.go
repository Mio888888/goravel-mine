package seeders

import "encoding/json"

type MenuSeeder struct{}

type menuSeed struct {
	ID        uint64
	ParentID  uint64
	Name      string
	Path      string
	Component string
	Redirect  string
	Meta      map[string]any
	Sort      int
}

type MenuCatalogSeed struct {
	ID        uint64
	ParentID  uint64
	Name      string
	Path      string
	Component string
	Redirect  string
	Meta      map[string]any
	Sort      int
}

func (s *MenuSeeder) Signature() string {
	return "menu_seed"
}

func (s *MenuSeeder) Run() error {
	if err := cleanupPlatformMenusFromTenant(); err != nil {
		return err
	}

	for _, item := range menuSeeds() {
		meta, err := json.Marshal(item.Meta)
		if err != nil {
			return err
		}

		if err := exec(`
			INSERT INTO menu (
				id, parent_id, name, meta, path, component, redirect, status, sort,
				created_by, updated_by, created_at, updated_at, remark
			)
			VALUES (?, ?, ?, ?::jsonb, ?, ?, ?, 1, ?, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, '')
			ON CONFLICT (id) DO UPDATE SET
				parent_id = EXCLUDED.parent_id,
				name = EXCLUDED.name,
				meta = EXCLUDED.meta,
				path = EXCLUDED.path,
				component = EXCLUDED.component,
				redirect = EXCLUDED.redirect,
				status = EXCLUDED.status,
				sort = EXCLUDED.sort,
				updated_at = CURRENT_TIMESTAMP
		`, item.ID, item.ParentID, item.Name, string(meta), item.Path, item.Component, item.Redirect, item.Sort); err != nil {
			return err
		}
	}

	return syncSequence("menu", "id")
}

func cleanupPlatformMenusFromTenant() error {
	if err := exec(`
		DELETE FROM role_belongs_menu
		WHERE menu_id IN (
			SELECT id FROM menu WHERE name = 'platform' OR name LIKE 'platform:%'
		)
	`); err != nil {
		return err
	}

	return exec(`
		DELETE FROM menu
		WHERE name = 'platform' OR name LIKE 'platform:%'
	`)
}

func menuSeeds() []menuSeed {
	return []menuSeed{
		menu(1, 0, "permission", "/permission", "", "", "权限管理", "baseMenu.permission.index", "ri:git-repository-private-line", "M", 100),
		menu(2, 1, "permission:user", "/permission/user", "base/views/permission/user/index", "", "用户管理", "baseMenu.permission.user", "material-symbols:manage-accounts-outline", "M", 10),
		button(3, 2, "permission:user:index", "用户列表", "baseMenu.permission.userList", 10),
		button(4, 2, "permission:user:save", "用户保存", "baseMenu.permission.userSave", 20),
		button(5, 2, "permission:user:update", "用户更新", "baseMenu.permission.userUpdate", 30),
		button(6, 2, "permission:user:delete", "用户删除", "baseMenu.permission.userDelete", 40),
		button(7, 2, "permission:user:password", "用户初始化密码", "baseMenu.permission.userPassword", 50),
		button(8, 2, "permission:user:getRole", "获取用户角色", "baseMenu.permission.getUserRole", 60),
		button(9, 2, "permission:user:setRole", "用户角色赋予", "baseMenu.permission.setUserRole", 70),
		menu(10, 1, "permission:menu", "/permission/menu", "base/views/permission/menu/index", "", "菜单管理", "baseMenu.permission.menu", "ph:list-bold", "M", 20),
		button(11, 10, "permission:menu:index", "菜单列表", "baseMenu.permission.menuList", 10),
		button(12, 10, "permission:menu:create", "菜单保存", "baseMenu.permission.menuSave", 20),
		button(13, 10, "permission:menu:save", "菜单更新", "baseMenu.permission.menuUpdate", 30),
		button(14, 10, "permission:menu:delete", "菜单删除", "baseMenu.permission.menuDelete", 40),
		menu(15, 1, "permission:role", "/permission/role", "base/views/permission/role/index", "", "角色管理", "baseMenu.permission.role", "material-symbols:supervisor-account-outline-rounded", "M", 30),
		button(16, 15, "permission:role:index", "角色列表", "baseMenu.permission.roleList", 10),
		button(17, 15, "permission:role:save", "角色保存", "baseMenu.permission.roleSave", 20),
		button(18, 15, "permission:role:update", "角色更新", "baseMenu.permission.roleUpdate", 30),
		button(19, 15, "permission:role:delete", "角色删除", "baseMenu.permission.roleDelete", 40),
		button(20, 15, "permission:role:getMenu", "获取角色菜单", "baseMenu.permission.getRolePermission", 50),
		button(21, 15, "permission:role:setMenu", "角色菜单赋予", "baseMenu.permission.setRolePermission", 60),
		menu(22, 1, "permission:department", "/permission/department", "base/views/permission/department/index", "", "部门管理", "baseMenu.permission.department", "mingcute:department-line", "M", 40),
		button(23, 22, "permission:department:index", "部门列表", "baseMenu.permission.departmentList", 10),
		button(24, 22, "permission:department:save", "部门保存", "baseMenu.permission.departmentSave", 20),
		button(25, 22, "permission:department:update", "部门更新", "baseMenu.permission.departmentSave", 30),
		button(26, 22, "permission:department:delete", "部门删除", "baseMenu.permission.departmentDelete", 40),
		uncached(menu(27, 1, "permission:position", "/permission/position", "base/views/permission/department/index", "", "岗位管理", "baseMenu.permission.positionList", "icon-park-outline:permissions", "M", 50)),
		button(28, 27, "permission:position:index", "岗位列表", "baseMenu.permission.positionList", 10),
		button(29, 27, "permission:position:save", "岗位保存", "baseMenu.permission.positionCreate", 20),
		button(30, 27, "permission:position:update", "岗位更新", "baseMenu.permission.positionSave", 30),
		button(31, 27, "permission:position:delete", "岗位删除", "baseMenu.permission.positionDelete", 40),
		button(32, 27, "permission:position:data_permission", "数据权限", "baseMenu.permission.positionDataScope", 50),
		uncached(menu(33, 1, "permission:leader", "/permission/leader", "base/views/permission/department/index", "", "领导管理", "baseMenu.permission.leaderList", "carbon:user-role", "M", 60)),
		button(34, 33, "permission:leader:index", "领导列表", "baseMenu.permission.leaderList", 10),
		button(35, 33, "permission:leader:save", "领导保存", "baseMenu.permission.leaderCreate", 20),
		button(36, 33, "permission:leader:delete", "领导删除", "baseMenu.permission.leaderDelete", 30),
		menu(78, 37, "dataCenter:dictionary", "/data-center/dictionary", "base/views/dataCenter/dictionary/index", "", "数据字典", "baseMenu.dataCenter.dictionary", "material-symbols:book-2-outline", "M", 20),
		button(79, 78, "dataCenter:dictionary:list", "数据字典列表", "baseMenu.dataCenter.dictionaryList", 10),
		button(80, 78, "dataCenter:dictionary:update", "数据字典更新", "baseMenu.dataCenter.dictionaryUpdate", 20),
		menu(63, 0, "security", "/security", "", "", "安全管理", "baseMenu.security.index", "material-symbols:security", "M", 150),
		button(81, 63, "security:mfa", "租户 MFA 管理", "baseMenu.security.mfa", 5),
		menu(77, 63, "security:sso", "/security/sso", "", "", "单点登录", "baseMenu.security.sso", "material-symbols:passkey", "M", 10),
		menu(64, 77, "security:ssoProvider", "/security/sso/provider", "base/views/security/ssoProvider/index", "", "身份源配置", "baseMenu.security.ssoProvider", "material-symbols:manage-accounts-outline", "M", 10),
		button(65, 64, "security:ssoProvider:list", "单点登录配置列表", "baseMenu.security.ssoProviderList", 10),
		button(66, 64, "security:ssoProvider:save", "单点登录配置保存", "baseMenu.security.ssoProviderSave", 20),
		button(67, 64, "security:ssoProvider:update", "单点登录配置更新", "baseMenu.security.ssoProviderUpdate", 30),
		button(68, 64, "security:ssoProvider:delete", "单点登录配置删除", "baseMenu.security.ssoProviderDelete", 40),
		menu(69, 77, "security:ssoUserBinding", "/security/sso/binding", "base/views/security/ssoUserBinding/index", "", "用户绑定", "baseMenu.security.ssoUserBinding", "material-symbols:link", "M", 20),
		button(70, 69, "security:ssoUserBinding:list", "单点登录用户绑定列表", "baseMenu.security.ssoUserBindingList", 10),
		button(71, 69, "security:ssoUserBinding:detail", "单点登录用户绑定详情", "baseMenu.security.ssoUserBindingDetail", 20),
		button(72, 69, "security:ssoUserBinding:user", "用户单点登录绑定", "baseMenu.security.ssoUserBindingUser", 30),
		button(73, 69, "security:ssoUserBinding:unbind", "单点登录解绑", "baseMenu.security.ssoUserBindingUnbind", 40),
		menu(74, 77, "security:ssoLoginAudit", "/security/sso/login-audit", "base/views/log/ssoLogin", "", "登录审计", "baseMenu.security.ssoLoginAudit", "material-symbols:fact-check-outline", "M", 30),
		button(75, 74, "log:ssoLogin:list", "SSO 登录日志列表", "baseMenu.security.ssoLoginList", 10),
		button(76, 74, "log:ssoLogin:stats", "SSO 登录统计", "baseMenu.security.ssoLoginStats", 20),
		menu(37, 0, "dataCenter", "/data-center", "", "", "数据中心", "baseMenu.dataCenter.index", "carbon:data-center", "M", 200),
		menu(38, 37, "dataCenter:attachment", "/data-center/attachment", "base/views/dataCenter/attachment/index", "", "附件管理", "baseMenu.dataCenter.attachment", "material-symbols:attach-file", "M", 10),
		button(39, 38, "dataCenter:attachment:list", "附件列表", "baseMenu.dataCenter.attachmentList", 10),
		button(40, 38, "dataCenter:attachment:upload", "附件上传", "baseMenu.dataCenter.attachmentUpload", 20),
		button(41, 38, "dataCenter:attachment:delete", "附件删除", "baseMenu.dataCenter.attachmentDelete", 30),
		menu(42, 0, "log", "/log", "", "", "日志管理", "baseMenu.log.index", "ph:scroll", "M", 300),
		menu(43, 42, "log:userLogin", "/log/user-login", "base/views/log/userLogin", "", "登录日志", "baseMenu.log.userLoginLog", "material-symbols:login", "M", 10),
		button(44, 43, "log:userLogin:list", "登录日志列表", "baseMenu.log.userLoginList", 10),
		menu(46, 42, "log:userOperation", "/log/user-operation", "base/views/log/userOperation", "", "操作日志", "baseMenu.log.operationLog", "material-symbols:history", "M", 20),
		button(47, 46, "log:userOperation:list", "操作日志列表", "baseMenu.log.userOperationList", 10),
	}
}

func TenantMenuCatalogSeeds() []MenuCatalogSeed {
	seeds := menuSeeds()
	items := make([]MenuCatalogSeed, 0, len(seeds))
	for _, seed := range seeds {
		meta := make(map[string]any, len(seed.Meta))
		for key, value := range seed.Meta {
			meta[key] = value
		}
		items = append(items, MenuCatalogSeed{
			ID:        seed.ID,
			ParentID:  seed.ParentID,
			Name:      seed.Name,
			Path:      seed.Path,
			Component: seed.Component,
			Redirect:  seed.Redirect,
			Meta:      meta,
			Sort:      seed.Sort,
		})
	}
	return items
}

func menu(id, parentID uint64, name, path, component, redirect, title, i18n, icon, menuType string, sort int) menuSeed {
	return menuSeed{
		ID:        id,
		ParentID:  parentID,
		Name:      name,
		Path:      path,
		Component: component,
		Redirect:  redirect,
		Sort:      sort,
		Meta: map[string]any{
			"title":            title,
			"i18n":             i18n,
			"icon":             icon,
			"type":             menuType,
			"hidden":           0,
			"componentPath":    "modules/",
			"componentSuffix":  ".vue",
			"breadcrumbEnable": 1,
			"copyright":        1,
			"cache":            1,
			"affix":            0,
		},
	}
}

func uncached(item menuSeed) menuSeed {
	item.Meta["cache"] = 0
	return item
}

func button(id, parentID uint64, name, title, i18n string, sort int) menuSeed {
	return menuSeed{
		ID:       id,
		ParentID: parentID,
		Name:     name,
		Sort:     sort,
		Meta: map[string]any{
			"title": title,
			"type":  "B",
			"i18n":  i18n,
		},
	}
}
