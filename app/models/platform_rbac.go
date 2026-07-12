package models

type PlatformUser struct {
	User
}

func (PlatformUser) TableName() string {
	return "platform_user"
}

type PlatformRole struct {
	Role
}

func (PlatformRole) TableName() string {
	return "platform_role"
}

type PlatformMenu struct {
	Menu
}

func (PlatformMenu) TableName() string {
	return "platform_menu"
}

type PlatformUserBelongsRole struct {
	UserBelongsRole
}

func (PlatformUserBelongsRole) TableName() string {
	return "platform_user_belongs_role"
}

type PlatformRoleBelongsMenu struct {
	RoleBelongsMenu
}

func (PlatformRoleBelongsMenu) TableName() string {
	return "platform_role_belongs_menu"
}

type PlatformCasbinRule struct {
	CasbinRule
}

func (PlatformCasbinRule) TableName() string {
	return "platform_casbin_rule"
}
