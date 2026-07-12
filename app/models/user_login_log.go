package models

import "time"

type UserLoginLog struct {
	ID        uint64    `gorm:"column:id;primaryKey" json:"id"`
	Username  string    `gorm:"column:username" json:"username"`
	IP        string    `gorm:"column:ip" json:"ip"`
	OS        string    `gorm:"column:os" json:"os"`
	Browser   string    `gorm:"column:browser" json:"browser"`
	Status    int16     `gorm:"column:status" json:"status"`
	Message   string    `gorm:"column:message" json:"message"`
	LoginTime time.Time `gorm:"column:login_time" json:"login_time"`
	Remark    string    `gorm:"column:remark" json:"remark"`
}

func (UserLoginLog) TableName() string {
	return "user_login_log"
}
