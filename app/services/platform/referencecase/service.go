package referencecase

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"github.com/goravel/framework/support/collect"

	"goravel/app/http/request"
	"goravel/app/models"
	"goravel/app/scopes"
	"goravel/app/support/apperror"
	"goravel/app/support/contextutil"
)

const ReferenceCaseStatusEnabled int8 = 1

const (
	referenceCaseBaselineVersion = "1.0.0"
	referenceCaseUpgradeVersion  = "1.1.0"
)

type ReferenceCase = models.ReferenceCase

type ReferenceCasePayload struct {
	Code    string         `json:"code"`
	Title   string         `json:"title"`
	Status  int8           `json:"status"`
	Version string         `json:"version"`
	Payload models.JSONMap `json:"payload"`
	Remark  string         `json:"remark"`
}

type ReferenceCaseService struct {
	ctx                context.Context
	platformConnection string
	ormFactory         func(context.Context, string) contractsorm.Orm
}

func NewService(platformConnection string, factory func(context.Context, string) contractsorm.Orm) *ReferenceCaseService {
	return &ReferenceCaseService{platformConnection: platformConnection, ormFactory: factory}
}

func (s *ReferenceCaseService) WithContext(ctx context.Context) *ReferenceCaseService {
	clone := *s
	clone.ctx = contextutil.OrBackground(ctx)
	return &clone
}

func (s *ReferenceCaseService) orm() contractsorm.Orm {
	return s.ormFactory(contextutil.OrBackground(s.ctx), s.platformConnection)
}

func (p ReferenceCasePayload) ReferenceCase() ReferenceCase {
	status := p.Status
	if status == 0 {
		status = ReferenceCaseStatusEnabled
	}
	version := strings.TrimSpace(p.Version)
	if version == "" {
		version = "1.0.0"
	}
	return ReferenceCase{
		Code:    strings.TrimSpace(p.Code),
		Title:   strings.TrimSpace(p.Title),
		Status:  status,
		Version: version,
		Payload: p.Payload,
		Remark:  strings.TrimSpace(p.Remark),
	}
}

func (s *ReferenceCaseService) List(filters map[string]string, page, pageSize int) (request.PageResult[ReferenceCase], error) {
	query := s.orm().Query().Table("reference_case")
	query = query.Scopes(scopes.Contains("code", filters["code"]))
	query = query.Scopes(scopes.Contains("title", filters["title"]))
	query = query.Scopes(scopes.Contains("version", filters["version"]))
	query = query.Scopes(scopes.EqualIfPresent("status", filters["status"]))

	return request.Paginate[ReferenceCase](query.OrderByDesc("id"), page, pageSize)
}

func (s *ReferenceCaseService) Create(input ReferenceCasePayload) (ReferenceCase, error) {
	item := input.ReferenceCase()
	if err := validateReferenceCase(item); err != nil {
		return ReferenceCase{}, err
	}
	err := s.orm().Transaction(func(tx contractsorm.Query) error {
		row := ReferenceCase{
			Code: item.Code, Title: item.Title, Status: item.Status,
			Version: item.Version, Remark: item.Remark,
		}
		if err := tx.Table("reference_case").Create(&row); err != nil {
			return err
		}
		item.ID = row.ID
		return updateReferenceCasePayloadWithQuery(tx, item.ID, item.Payload)
	})
	if err != nil {
		return ReferenceCase{}, err
	}
	return item, nil
}

func (s *ReferenceCaseService) Update(id uint64, input ReferenceCasePayload) (ReferenceCase, error) {
	item := input.ReferenceCase()
	if err := validateReferenceCase(item); err != nil {
		return ReferenceCase{}, err
	}
	err := s.orm().Transaction(func(tx contractsorm.Query) error {
		result, err := tx.Table("reference_case").Where("id", id).Update(map[string]any{
			"code": item.Code, "title": item.Title, "status": item.Status,
			"version": item.Version, "remark": item.Remark, "updated_at": time.Now(),
		})
		if err != nil {
			return err
		}
		if result.RowsAffected != 1 {
			return apperror.BusinessError{Message: "参考案例不存在"}
		}
		return updateReferenceCasePayloadWithQuery(tx, id, item.Payload)
	})
	if err != nil {
		return ReferenceCase{}, err
	}
	item.ID = id
	return item, nil
}

func (s *ReferenceCaseService) Delete(ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.orm().Query().Table("reference_case").WhereIn("id", uint64Any(ids)).Delete()
	return err
}

func (s *ReferenceCaseService) updatePayload(id uint64, payload models.JSONMap) error {
	return updateReferenceCasePayloadWithQuery(s.orm().Query(), id, payload)
}

func validateReferenceCase(item ReferenceCase) error {
	if item.Code == "" || item.Title == "" {
		return apperror.BusinessError{Message: "参考案例编码和标题不能为空"}
	}
	if item.Status != ReferenceCaseStatusEnabled && item.Status != 2 {
		return apperror.BusinessError{Message: "参考案例状态无效"}
	}
	return nil
}

func updateReferenceCasePayloadWithQuery(query contractsorm.Query, id uint64, payload models.JSONMap) error {
	data, err := json.Marshal(nullIfEmpty(payload))
	if err != nil {
		return err
	}
	_, err = query.Exec("UPDATE reference_case SET payload = ?::jsonb WHERE id = ?", string(data), id)
	return err
}

func ApplyUpgrade(ctx context.Context, platformConnection string, factory func(context.Context, string) contractsorm.Orm) error {
	return factory(contextutil.OrBackground(ctx), platformConnection).Transaction(func(tx contractsorm.Query) error {
		if _, err := tx.Exec(`
			ALTER TABLE reference_case
			ADD COLUMN IF NOT EXISTS upgrade_note VARCHAR(255) NOT NULL DEFAULT ''
		`); err != nil {
			return err
		}
		_, err := tx.Exec(`
			INSERT INTO reference_case (
				code, title, status, version, payload, upgrade_note,
				created_at, updated_at, remark
			)
			VALUES (
				'golden-case', 'Golden Reference Case', 1, ?,
				'{"scenario":"upgrade"}'::jsonb, 'reference lifecycle upgrade applied',
				CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'golden reference module baseline'
			)
			ON CONFLICT (code) DO UPDATE SET
				version = EXCLUDED.version,
				upgrade_note = EXCLUDED.upgrade_note,
				updated_at = CURRENT_TIMESTAMP
		`, referenceCaseUpgradeVersion)
		return err
	})
}

func RollbackUpgrade(ctx context.Context, platformConnection string, factory func(context.Context, string) contractsorm.Orm) error {
	return factory(contextutil.OrBackground(ctx), platformConnection).Transaction(func(tx contractsorm.Query) error {
		if _, err := tx.Table("reference_case").Where("code", "golden-case").Update(map[string]any{
			"version":    referenceCaseBaselineVersion,
			"updated_at": time.Now(),
		}); err != nil {
			return err
		}
		_, err := tx.Exec("ALTER TABLE reference_case DROP COLUMN IF EXISTS upgrade_note")
		return err
	})
}

func nullIfEmpty(value models.JSONMap) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func uint64Any(values []uint64) []any {
	return collect.Map(values, func(value uint64, _ int) any {
		return value
	})
}
