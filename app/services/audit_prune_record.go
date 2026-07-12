package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
)

type auditPruneRecordRow struct {
	ID         uint64 `gorm:"column:id"`
	RecordJSON string `gorm:"column:record_json"`
}

func auditPruneRecordSelect(table string) string {
	return fmt.Sprintf("to_jsonb(%s.*)::text AS record_json", table)
}

func canonicalAuditPruneRecord(payload []byte) ([]byte, error) {
	var record any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&record); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("audit prune record contains trailing JSON")
	}
	return json.Marshal(record)
}

func auditPruneRecordDigest(payload []byte) (string, error) {
	canonical, err := canonicalAuditPruneRecord(payload)
	if err != nil {
		return "", err
	}
	return digestBytes(canonical), nil
}

func loadAuditPruneRecord(query contractsorm.Query, table string, targetID uint64, lock bool) ([]byte, bool, error) {
	query = query.Table(table).Select("id", auditPruneRecordSelect(table)).Where("id", targetID)
	if lock {
		query = query.LockForUpdate()
	}
	rows := make([]auditPruneRecordRow, 0, 1)
	if err := query.Limit(1).Get(&rows); err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	canonical, err := canonicalAuditPruneRecord([]byte(strings.TrimSpace(rows[0].RecordJSON)))
	return canonical, true, err
}
