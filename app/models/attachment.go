package models

type Attachment struct {
	ID              uint64 `gorm:"column:id;primaryKey" json:"id"`
	StorageMode     string `gorm:"column:storage_mode" json:"storage_mode"`
	StorageConfigID uint64 `gorm:"column:storage_config_id" json:"storage_config_id"`
	OriginName      string `gorm:"column:origin_name" json:"origin_name"`
	ObjectName      string `gorm:"column:object_name" json:"object_name"`
	Hash            string `gorm:"column:hash" json:"hash"`
	MimeType        string `gorm:"column:mime_type" json:"mime_type"`
	StoragePath     string `gorm:"column:storage_path" json:"storage_path"`
	Suffix          string `gorm:"column:suffix" json:"suffix"`
	SizeByte        int64  `gorm:"column:size_byte" json:"size_byte"`
	SizeInfo        string `gorm:"column:size_info" json:"size_info"`
	URL             string `gorm:"column:url" json:"url"`
	AuditColumns
	Timestamps
	Remark string `gorm:"column:remark" json:"remark"`
}

func (Attachment) TableName() string {
	return "attachment"
}
