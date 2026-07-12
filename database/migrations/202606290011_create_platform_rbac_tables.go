package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606290011CreatePlatformRBACTables struct{}

func (r *M202606290011CreatePlatformRBACTables) Signature() string {
	return "202606290011_create_platform_rbac_tables"
}

func (r *M202606290011CreatePlatformRBACTables) Up() error {
	if err := createPlatformUserTable(); err != nil {
		return err
	}
	if err := createPlatformRoleTable(); err != nil {
		return err
	}
	if err := createPlatformMenuTable(); err != nil {
		return err
	}
	if err := createPlatformUserRoleTable(); err != nil {
		return err
	}
	if err := createPlatformRoleMenuTable(); err != nil {
		return err
	}
	return createPlatformCasbinRuleTable()
}

func (r *M202606290011CreatePlatformRBACTables) Down() error {
	return dropTables(
		"platform_casbin_rule",
		"platform_role_belongs_menu",
		"platform_user_belongs_role",
		"platform_menu",
		"platform_role",
		"platform_user",
	)
}

func createPlatformUserTable() error {
	if facades.Schema().HasTable("platform_user") {
		return nil
	}

	return facades.Schema().Create("platform_user", func(table schema.Blueprint) {
		table.ID()
		table.String("username", 20).Default("")
		table.String("password", 100)
		table.String("user_type", 3).Default("900")
		table.String("nickname", 30).Default("")
		table.String("phone", 11).Default("")
		table.String("email", 50).Default("")
		table.String("avatar", 255).Default("")
		table.String("signed", 255).Default("")
		table.String("dashboard", 100).Default("")
		table.TinyInteger("status").Default(1)
		table.String("login_ip", 45).Default("127.0.0.1")
		table.Timestamp("login_time").UseCurrent()
		table.Jsonb("backend_setting").Nullable()
		addAuditColumns(table)
		addTimestamps(table)
		table.String("remark", 255).Default("")
		table.Unique("username")
	})
}

func createPlatformRoleTable() error {
	if facades.Schema().HasTable("platform_role") {
		return nil
	}

	return facades.Schema().Create("platform_role", func(table schema.Blueprint) {
		table.ID()
		table.String("name", 30)
		table.String("code", 100)
		table.TinyInteger("status").Default(1)
		table.SmallInteger("sort").Default(0)
		addAuditColumns(table)
		addTimestamps(table)
		table.String("remark", 255).Default("")
		table.Unique("code")
	})
}

func createPlatformMenuTable() error {
	if facades.Schema().HasTable("platform_menu") {
		return nil
	}

	return facades.Schema().Create("platform_menu", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("parent_id").Default(0)
		table.String("name", 80).Default("")
		table.Jsonb("meta").Nullable()
		table.String("path", 80).Default("")
		table.String("component", 180).Default("")
		table.String("redirect", 100).Default("")
		table.TinyInteger("status").Default(1)
		table.SmallInteger("sort").Default(0)
		addAuditColumns(table)
		addTimestamps(table)
		table.String("remark", 60).Default("")
		table.Unique("name")
		table.Index("parent_id")
	})
}

func createPlatformUserRoleTable() error {
	if facades.Schema().HasTable("platform_user_belongs_role") {
		return nil
	}

	return facades.Schema().Create("platform_user_belongs_role", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("user_id")
		table.UnsignedBigInteger("role_id")
		addTimestamps(table)
		table.Unique("user_id", "role_id")
		table.Index("role_id")
	})
}

func createPlatformRoleMenuTable() error {
	if facades.Schema().HasTable("platform_role_belongs_menu") {
		return nil
	}

	return facades.Schema().Create("platform_role_belongs_menu", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("role_id")
		table.UnsignedBigInteger("menu_id")
		addTimestamps(table)
		table.Unique("role_id", "menu_id")
		table.Index("menu_id")
	})
}

func createPlatformCasbinRuleTable() error {
	if facades.Schema().HasTable("platform_casbin_rule") {
		return nil
	}

	return facades.Schema().Create("platform_casbin_rule", func(table schema.Blueprint) {
		table.ID()
		table.String("ptype").Nullable()
		table.String("v0").Nullable()
		table.String("v1").Nullable()
		table.String("v2").Nullable()
		table.String("v3").Nullable()
		table.String("v4").Nullable()
		table.String("v5").Nullable()
		addTimestamps(table)
		table.Index("ptype", "v0")
	})
}
