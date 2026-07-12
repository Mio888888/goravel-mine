package models

type Tenant struct {
	ID           uint64  `gorm:"column:id;primaryKey" json:"id"`
	Code         string  `gorm:"column:code" json:"code"`
	Name         string  `gorm:"column:name" json:"name"`
	Status       int8    `gorm:"column:status" json:"status"`
	Plan         string  `gorm:"column:plan" json:"plan"`
	DBHost       string  `gorm:"column:db_host" json:"db_host"`
	DBPort       int     `gorm:"column:db_port" json:"db_port"`
	DBDatabase   string  `gorm:"column:db_database" json:"db_database"`
	DBUsername   string  `gorm:"column:db_username" json:"db_username"`
	DBPassword   string  `gorm:"column:db_password" json:"-"`
	DBSchema     string  `gorm:"column:db_schema" json:"db_schema"`
	CustomDomain *string `gorm:"column:custom_domain" json:"custom_domain"`
	Branding     JSONMap `gorm:"column:branding;type:jsonb" json:"branding"`
	Billing      JSONMap `gorm:"column:billing;type:jsonb" json:"billing"`
	Quotas       JSONMap `gorm:"column:quotas;type:jsonb" json:"quotas"`
	Features     JSONMap `gorm:"column:features;type:jsonb" json:"features"`
	Timestamps
	Remark string `gorm:"column:remark" json:"remark"`
}

func (Tenant) TableName() string {
	return "tenant"
}
