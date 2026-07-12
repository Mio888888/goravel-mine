package models

import "time"

type StorageConfig struct {
	ID                 uint64    `gorm:"column:id;primaryKey" json:"id"`
	Name               string    `gorm:"column:name" json:"name"`
	Provider           string    `gorm:"column:provider" json:"provider"`
	Driver             string    `gorm:"column:driver" json:"driver"`
	Bucket             string    `gorm:"column:bucket" json:"bucket"`
	Endpoint           string    `gorm:"column:endpoint" json:"endpoint"`
	Region             string    `gorm:"column:region" json:"region"`
	AccessKey          string    `gorm:"column:access_key" json:"access_key,omitempty"`
	SecretKey          string    `gorm:"column:secret_key" json:"-"`
	SecretKeyRotatedAt time.Time `gorm:"column:secret_key_rotated_at" json:"secret_key_rotated_at"`
	BaseURL            string    `gorm:"column:base_url" json:"base_url"`
	PathPrefix         string    `gorm:"column:path_prefix" json:"path_prefix"`
	IsDefault          bool      `gorm:"column:is_default" json:"is_default"`
	Status             int8      `gorm:"column:status" json:"status"`
	Options            JSONMap   `gorm:"column:options;type:jsonb" json:"options"`
	AuditColumns
	Timestamps
	Remark string `gorm:"column:remark" json:"remark"`
}

func (StorageConfig) TableName() string {
	return "storage_config"
}
