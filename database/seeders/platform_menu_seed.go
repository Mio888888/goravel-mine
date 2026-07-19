package seeders

import "encoding/json"

type PlatformMenuSeeder struct{}

func (s *PlatformMenuSeeder) Signature() string {
	return "platform_menu_seed"
}

func (s *PlatformMenuSeeder) Run() error {
	for _, item := range platformMenuSeeds() {
		meta, err := json.Marshal(item.Meta)
		if err != nil {
			return err
		}

		if err := exec(`
			INSERT INTO platform_menu (
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

	return syncSequence("platform_menu", "id")
}

func platformMenuSeeds() []menuSeed {
	return []menuSeed{
		menu(1, 0, "platform", "/platform", "", "", "平台管理", "baseMenu.platform.index", "material-symbols:admin-panel-settings-outline", "M", 50),
		menu(42, 0, "platform:tenantManage", "/tenant-manage", "", "", "租户管理", "baseMenu.platform.tenantManage", "material-symbols:domain", "M", 40),
		menu(43, 0, "platform:system", "/platform-system", "", "", "系统管理", "baseMenu.platform.system", "material-symbols:settings-outline", "M", 60),
		menu(60, 0, "dashboard", "/dashboard", "", "/dashboard/workbench", "仪表盘", "menu.dashboard", "mingcute:dashboard-line", "M", 10),
		menu(2, 42, "platform:tenant", "/tenant-manage/tenant", "base/views/platform/tenant/index", "", "租户管理", "baseMenu.platform.tenant", "material-symbols:domain", "M", 10),
		button(3, 2, "platform:tenant:list", "租户列表", "baseMenu.platform.tenantList", 10),
		button(4, 2, "platform:tenant:save", "租户保存", "baseMenu.platform.tenantSave", 20),
		button(5, 2, "platform:tenant:update", "租户更新", "baseMenu.platform.tenantUpdate", 30),
		button(6, 2, "platform:tenant:suspend", "租户挂起", "baseMenu.platform.tenantSuspend", 40),
		button(7, 2, "platform:tenant:resume", "租户恢复", "baseMenu.platform.tenantResume", 50),
		button(8, 2, "platform:tenant:archive", "租户归档", "baseMenu.platform.tenantArchive", 60),
		button(9, 2, "platform:tenant:destroy", "租户销毁", "baseMenu.platform.tenantDestroy", 70),
		button(30, 2, "platform:tenant:usage", "租户用量", "baseMenu.platform.tenantUsage", 80),
		button(69, 2, "platform:tenant:governance", "租户治理", "baseMenu.platform.tenantGovernance", 90),
		button(90, 2, "platform:tenant:export", "租户数据导出", "baseMenu.platform.tenantExport", 100),
		button(44, 2, "platform:tenant:permissions", "租户权限分配", "baseMenu.platform.tenantPermissions", 110),
		button(45, 2, "platform:tenant:updatePlan", "租户套餐变更", "baseMenu.platform.tenantUpdatePlan", 120),
		menu(31, 42, "platform:tenantPlan", "/tenant-manage/tenant-plan", "base/views/platform/tenantPlan/index", "", "套餐管理", "baseMenu.platform.tenantPlan", "material-symbols:inventory-2-outline", "M", 20),
		button(32, 31, "platform:tenantPlan:list", "套餐列表", "baseMenu.platform.tenantPlanList", 10),
		button(33, 31, "platform:tenantPlan:save", "套餐保存", "baseMenu.platform.tenantPlanSave", 20),
		button(34, 31, "platform:tenantPlan:update", "套餐更新", "baseMenu.platform.tenantPlanUpdate", 30),
		button(35, 31, "platform:tenantPlan:delete", "套餐删除", "baseMenu.platform.tenantPlanDelete", 40),
		menu(10, 1, "platform:user", "/platform/user", "base/views/platform/user/index", "", "平台用户", "baseMenu.platform.user", "material-symbols:manage-accounts-outline", "M", 30),
		button(11, 10, "platform:user:list", "平台用户列表", "baseMenu.platform.userList", 10),
		button(12, 10, "platform:user:save", "平台用户保存", "baseMenu.platform.userSave", 20),
		button(13, 10, "platform:user:update", "平台用户更新", "baseMenu.platform.userUpdate", 30),
		button(14, 10, "platform:user:delete", "平台用户删除", "baseMenu.platform.userDelete", 40),
		button(15, 10, "platform:user:password", "平台用户初始化密码", "baseMenu.platform.userPassword", 50),
		button(16, 10, "platform:user:getRole", "获取平台用户角色", "baseMenu.platform.getUserRole", 60),
		button(17, 10, "platform:user:setRole", "平台用户角色赋予", "baseMenu.platform.setUserRole", 70),
		button(65, 10, "platform:security:mfa", "平台 MFA 管理", "baseMenu.platform.securityMfa", 80),
		button(81, 10, "platform:security:control", "敏感操作控制", "baseMenu.platform.securityControl", 90),
		menu(18, 1, "platform:role", "/platform/role", "base/views/platform/role/index", "", "平台角色", "baseMenu.platform.role", "material-symbols:supervisor-account-outline-rounded", "M", 40),
		button(19, 18, "platform:role:list", "平台角色列表", "baseMenu.platform.roleList", 10),
		button(20, 18, "platform:role:save", "平台角色保存", "baseMenu.platform.roleSave", 20),
		button(21, 18, "platform:role:update", "平台角色更新", "baseMenu.platform.roleUpdate", 30),
		button(22, 18, "platform:role:delete", "平台角色删除", "baseMenu.platform.roleDelete", 40),
		button(23, 18, "platform:role:getMenu", "获取平台角色菜单", "baseMenu.platform.getRolePermission", 50),
		button(24, 18, "platform:role:setMenu", "平台角色菜单赋予", "baseMenu.platform.setRolePermission", 60),
		menu(25, 1, "platform:menu", "/platform/menu", "base/views/platform/menu/index", "", "平台菜单", "baseMenu.platform.menu", "ph:list-bold", "M", 50),
		button(26, 25, "platform:menu:list", "平台菜单列表", "baseMenu.platform.menuList", 10),
		button(27, 25, "platform:menu:create", "平台菜单保存", "baseMenu.platform.menuSave", 20),
		button(28, 25, "platform:menu:save", "平台菜单更新", "baseMenu.platform.menuUpdate", 30),
		button(29, 25, "platform:menu:delete", "平台菜单删除", "baseMenu.platform.menuDelete", 40),
		menu(36, 43, "platform:dictionary", "/platform-system/dictionary", "base/views/platform/dictionary/index", "", "字典管理", "baseMenu.platform.dictionary", "material-symbols:book-2-outline", "M", 10),
		button(37, 36, "platform:dictionary:list", "字典列表", "baseMenu.platform.dictionaryList", 10),
		button(38, 36, "platform:dictionary:save", "字典保存", "baseMenu.platform.dictionarySave", 20),
		button(39, 36, "platform:dictionary:update", "字典更新", "baseMenu.platform.dictionaryUpdate", 30),
		button(40, 36, "platform:dictionary:delete", "字典删除", "baseMenu.platform.dictionaryDelete", 40),
		button(41, 36, "platform:dictionary:dispatch", "字典分发", "baseMenu.platform.dictionaryDispatch", 50),
		menu(46, 43, "platform:storageConfig", "/platform-system/storage-config", "base/views/platform/storageConfig/index", "", "储存配置", "baseMenu.platform.storageConfig", "material-symbols:cloud-outline", "M", 20),
		button(47, 46, "platform:storageConfig:list", "储存配置列表", "baseMenu.platform.storageConfigList", 10),
		button(48, 46, "platform:storageConfig:save", "储存配置保存", "baseMenu.platform.storageConfigSave", 20),
		button(49, 46, "platform:storageConfig:update", "储存配置更新", "baseMenu.platform.storageConfigUpdate", 30),
		button(50, 46, "platform:storageConfig:delete", "储存配置删除", "baseMenu.platform.storageConfigDelete", 40),
		button(64, 46, "platform:attachment:upload", "平台附件上传", "baseMenu.platform.attachmentUpload", 50),
		menu(51, 43, "platform:scheduledTask", "/platform-system/scheduled-task", "base/views/platform/scheduledTask/index", "", "计划任务", "baseMenu.platform.scheduledTask", "material-symbols:timer-outline", "M", 30),
		button(52, 51, "platform:scheduledTask:list", "计划任务列表", "baseMenu.platform.scheduledTaskList", 10),
		button(53, 51, "platform:scheduledTask:save", "计划任务保存", "baseMenu.platform.scheduledTaskSave", 20),
		button(54, 51, "platform:scheduledTask:update", "计划任务更新", "baseMenu.platform.scheduledTaskUpdate", 30),
		button(55, 51, "platform:scheduledTask:delete", "计划任务删除", "baseMenu.platform.scheduledTaskDelete", 40),
		button(56, 51, "platform:scheduledTask:run", "计划任务执行", "baseMenu.platform.scheduledTaskRun", 50),
		button(57, 51, "platform:scheduledTask:log", "计划任务日志", "baseMenu.platform.scheduledTaskLog", 60),
		button(61, 51, "platform:queueFailedJob:list", "失败队列列表", "baseMenu.platform.queueFailedJobList", 70),
		button(62, 51, "platform:queueFailedJob:retry", "失败队列重试", "baseMenu.platform.queueFailedJobRetry", 80),
		button(63, 51, "platform:queueFailedJob:delete", "失败队列丢弃", "baseMenu.platform.queueFailedJobDelete", 90),
		menu(91, 43, "platform:middleware", "/platform-system/middleware", "base/views/platform/middleware/index", "", "中间件平台", "baseMenu.platform.middleware", "material-symbols:hub-outline", "M", 35),
		button(92, 91, "platform:middleware:list", "中间件平台查看", "baseMenu.platform.middlewareList", 10),
		button(93, 91, "platform:middleware:configure", "中间件平台配置", "baseMenu.platform.middlewareConfigure", 20),
		button(94, 91, "platform:middleware:execute", "中间件平台执行", "baseMenu.platform.middlewareExecute", 30),
		button(95, 91, "platform:middleware:publish", "中间件发布", "baseMenu.platform.middlewarePublish", 40),
		button(96, 91, "platform:middleware:replay", "消息死信重放", "baseMenu.platform.middlewareReplay", 50),
		button(97, 91, "platform:middleware:payload", "消息载荷查看", "baseMenu.platform.middlewarePayload", 60),
		menu(58, 60, "platform:observability", "/dashboard/observability", "base/views/platform/observability/index", "", "系统监控", "baseMenu.platform.observability", "ant-design:dashboard-outlined", "M", 40),
		button(59, 58, "platform:observability:list", "监控面板", "baseMenu.platform.observabilityList", 10),
		menu(66, 43, "platform:moduleLifecycle", "/platform-system/module-lifecycle", "base/views/platform/moduleLifecycle/index", "", "模块治理", "baseMenu.platform.moduleLifecycle", "material-symbols:deployed-code-outline", "M", 40),
		button(67, 66, "platform:moduleLifecycle:list", "模块治理面板", "baseMenu.platform.moduleLifecycleList", 10),
		button(68, 66, "platform:moduleLifecycle:execute", "模块生命周期执行", "baseMenu.platform.moduleLifecycleExecute", 20),
		button(75, 66, "platform:moduleLifecycle:log", "模块生命周期日志", "baseMenu.platform.moduleLifecycleLog", 30),
		menu(76, 43, "platform:referenceCase", "/platform-system/reference-case", "base/views/platform/referenceCase/index", "", "参考模块", "baseMenu.platform.referenceCase", "material-symbols:fact-check-outline", "M", 50),
		button(77, 76, "platform:referenceCase:list", "参考模块列表", "baseMenu.platform.referenceCaseList", 10),
		button(78, 76, "platform:referenceCase:save", "参考模块保存", "baseMenu.platform.referenceCaseSave", 20),
		button(79, 76, "platform:referenceCase:update", "参考模块更新", "baseMenu.platform.referenceCaseUpdate", 30),
		button(80, 76, "platform:referenceCase:delete", "参考模块删除", "baseMenu.platform.referenceCaseDelete", 40),
	}
}

func PlatformMenuCatalogSeeds() []MenuCatalogSeed {
	seeds := platformMenuSeeds()
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
