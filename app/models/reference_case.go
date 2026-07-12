package models

type ReferenceCase struct {
	ID      uint64  `gorm:"column:id;primaryKey" json:"id"`
	Code    string  `gorm:"column:code" json:"code"`
	Title   string  `gorm:"column:title" json:"title"`
	Status  int8    `gorm:"column:status" json:"status"`
	Version string  `gorm:"column:version" json:"version"`
	Payload JSONMap `gorm:"column:payload;type:jsonb" json:"payload"`
	Timestamps
	Remark string `gorm:"column:remark" json:"remark"`
}

func (ReferenceCase) TableName() string {
	return "reference_case"
}
