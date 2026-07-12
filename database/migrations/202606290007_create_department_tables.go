package migrations

import (
	"github.com/goravel/framework/contracts/database/schema"

	"goravel/app/facades"
)

type M202606290007CreateDepartmentTables struct{}

func (r *M202606290007CreateDepartmentTables) Signature() string {
	return "202606290007_create_department_tables"
}

func (r *M202606290007CreateDepartmentTables) Up() error {
	if err := createDepartmentTable(); err != nil {
		return err
	}
	if err := createPositionTable(); err != nil {
		return err
	}
	if err := createUserDeptTable(); err != nil {
		return err
	}
	if err := createUserPositionTable(); err != nil {
		return err
	}
	if err := createDeptLeaderTable(); err != nil {
		return err
	}

	return createDataPermissionPolicyTable()
}

func (r *M202606290007CreateDepartmentTables) Down() error {
	return dropTables(
		"data_permission_policy",
		"dept_leader",
		"user_position",
		"user_dept",
		"position",
		"department",
	)
}

func createDepartmentTable() error {
	if facades.Schema().HasTable("department") {
		return nil
	}

	return facades.Schema().Create("department", func(table schema.Blueprint) {
		table.ID()
		table.String("name", 50)
		table.UnsignedBigInteger("parent_id").Default(0)
		addTimestamps(table)
		addSoftDeletes(table)
		table.Index("parent_id")
	})
}

func createPositionTable() error {
	if facades.Schema().HasTable("position") {
		return nil
	}

	return facades.Schema().Create("position", func(table schema.Blueprint) {
		table.ID()
		table.String("name", 50)
		table.UnsignedBigInteger("dept_id")
		addTimestamps(table)
		addSoftDeletes(table)
		table.Index("dept_id")
	})
}

func createUserDeptTable() error {
	if facades.Schema().HasTable("user_dept") {
		return nil
	}

	return facades.Schema().Create("user_dept", func(table schema.Blueprint) {
		table.UnsignedBigInteger("user_id")
		table.UnsignedBigInteger("dept_id")
		addTimestamps(table)
		addSoftDeletes(table)
		table.Unique("user_id", "dept_id")
		table.Index("dept_id")
	})
}

func createUserPositionTable() error {
	if facades.Schema().HasTable("user_position") {
		return nil
	}

	return facades.Schema().Create("user_position", func(table schema.Blueprint) {
		table.UnsignedBigInteger("user_id")
		table.UnsignedBigInteger("position_id")
		addTimestamps(table)
		addSoftDeletes(table)
		table.Unique("user_id", "position_id")
		table.Index("position_id")
	})
}

func createDeptLeaderTable() error {
	if facades.Schema().HasTable("dept_leader") {
		return nil
	}

	return facades.Schema().Create("dept_leader", func(table schema.Blueprint) {
		table.UnsignedBigInteger("dept_id")
		table.UnsignedBigInteger("user_id")
		addTimestamps(table)
		addSoftDeletes(table)
		table.Unique("dept_id", "user_id")
		table.Index("user_id")
	})
}

func createDataPermissionPolicyTable() error {
	if facades.Schema().HasTable("data_permission_policy") {
		return nil
	}

	return facades.Schema().Create("data_permission_policy", func(table schema.Blueprint) {
		table.ID()
		table.UnsignedBigInteger("user_id").Default(0)
		table.UnsignedBigInteger("position_id").Default(0)
		table.String("policy_type", 20)
		table.Boolean("is_default").Default(true)
		table.Jsonb("value").Nullable()
		addTimestamps(table)
		addSoftDeletes(table)
		table.Index("user_id")
		table.Index("position_id")
	})
}
