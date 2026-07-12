package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type JSONMap map[string]any

func (m *JSONMap) UnmarshalJSON(data []byte) error {
	if string(data) == "[]" {
		*m = nil
		return nil
	}

	type jsonMap JSONMap
	var decoded jsonMap
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	*m = JSONMap(decoded)
	return nil
}

func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}

	return json.Marshal(m)
}

func (m *JSONMap) Scan(value any) error {
	return scanJSON(value, m)
}

type JSONSlice []any

func (s JSONSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}

	return json.Marshal(s)
}

func (s *JSONSlice) Scan(value any) error {
	return scanJSON(value, s)
}

type Timestamps struct {
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

type AuditColumns struct {
	CreatedBy uint64 `gorm:"column:created_by" json:"created_by"`
	UpdatedBy uint64 `gorm:"column:updated_by" json:"updated_by"`
}

type SoftDelete struct {
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}

func scanJSON(value any, target any) error {
	if value == nil {
		return nil
	}

	switch data := value.(type) {
	case []byte:
		return json.Unmarshal(data, target)
	case string:
		return json.Unmarshal([]byte(data), target)
	default:
		return fmt.Errorf("unsupported JSON scan type %T", value)
	}
}
