package models

type UserOperationLog struct {
	ID          uint64 `gorm:"column:id;primaryKey" json:"id"`
	Username    string `gorm:"column:username" json:"username"`
	Method      string `gorm:"column:method" json:"method"`
	Router      string `gorm:"column:router" json:"router"`
	ServiceName string `gorm:"column:service_name" json:"service_name"`
	IP          string `gorm:"column:ip" json:"ip"`
	Timestamps
	Remark string `gorm:"column:remark" json:"remark"`
}

func (UserOperationLog) TableName() string {
	return "user_operation_log"
}
