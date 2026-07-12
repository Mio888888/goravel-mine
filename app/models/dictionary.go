package models

type DictType struct {
	ID         uint64 `gorm:"column:id;primaryKey" json:"id"`
	SourceID   uint64 `gorm:"column:source_id" json:"source_id"`
	SourceCode string `gorm:"column:source_code" json:"source_code"`
	Code       string `gorm:"column:code" json:"code"`
	Name       string `gorm:"column:name" json:"name"`
	Status     int8   `gorm:"column:status" json:"status"`
	Sort       int    `gorm:"column:sort" json:"sort"`
	Version    int    `gorm:"column:version" json:"version"`
	IsSystem   bool   `gorm:"column:is_system" json:"is_system"`
	AuditColumns
	Timestamps
	Remark string `gorm:"column:remark" json:"remark"`
}

func (DictType) TableName() string {
	return "dict_type"
}

type DictItem struct {
	ID         uint64 `gorm:"column:id;primaryKey" json:"id"`
	TypeID     uint64 `gorm:"column:type_id" json:"type_id"`
	SourceID   uint64 `gorm:"column:source_id" json:"source_id"`
	SourceCode string `gorm:"column:source_code" json:"source_code"`
	TypeCode   string `gorm:"column:type_code" json:"type_code"`
	Label      string `gorm:"column:label" json:"label"`
	Value      string `gorm:"column:value" json:"value"`
	I18n       string `gorm:"column:i18n" json:"i18n"`
	Color      string `gorm:"column:color" json:"color"`
	Status     int8   `gorm:"column:status" json:"status"`
	Sort       int    `gorm:"column:sort" json:"sort"`
	Version    int    `gorm:"column:version" json:"version"`
	IsSystem   bool   `gorm:"column:is_system" json:"is_system"`
	AuditColumns
	Timestamps
	Remark string `gorm:"column:remark" json:"remark"`
}

func (DictItem) TableName() string {
	return "dict_item"
}

type PlatformDictType struct {
	ID       uint64 `gorm:"column:id;primaryKey" json:"id"`
	Code     string `gorm:"column:code" json:"code"`
	Name     string `gorm:"column:name" json:"name"`
	Status   int8   `gorm:"column:status" json:"status"`
	Sort     int    `gorm:"column:sort" json:"sort"`
	Version  int    `gorm:"column:version" json:"version"`
	IsSystem bool   `gorm:"column:is_system" json:"is_system"`
	AuditColumns
	Timestamps
	Remark string             `gorm:"column:remark" json:"remark"`
	Items  []PlatformDictItem `gorm:"-" json:"items"`
}

func (PlatformDictType) TableName() string {
	return "platform_dict_type"
}

type PlatformDictItem struct {
	ID       uint64 `gorm:"column:id;primaryKey" json:"id"`
	TypeID   uint64 `gorm:"column:type_id" json:"type_id"`
	TypeCode string `gorm:"column:type_code" json:"type_code"`
	Label    string `gorm:"column:label" json:"label"`
	Value    string `gorm:"column:value" json:"value"`
	I18n     string `gorm:"column:i18n" json:"i18n"`
	Color    string `gorm:"column:color" json:"color"`
	Status   int8   `gorm:"column:status" json:"status"`
	Sort     int    `gorm:"column:sort" json:"sort"`
	Version  int    `gorm:"column:version" json:"version"`
	IsSystem bool   `gorm:"column:is_system" json:"is_system"`
	AuditColumns
	Timestamps
	Remark string `gorm:"column:remark" json:"remark"`
}

func (PlatformDictItem) TableName() string {
	return "platform_dict_item"
}
