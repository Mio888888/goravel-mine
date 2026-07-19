package protection

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	frameworkerrors "github.com/goravel/framework/errors"

	"goravel/app/http/request"
	"goravel/app/models"
	"goravel/app/scopes"
	queueservice "goravel/app/services/runtime/queue"
	"goravel/app/support/contextutil"
)

type RuleSetPayload struct {
	Name            string         `json:"name"`
	Scope           string         `json:"scope"`
	ResourcePattern string         `json:"resource_pattern"`
	Rules           models.JSONMap `json:"rules"`
	Enabled         *bool          `json:"enabled"`
	Version         int            `json:"version"`
}

type RuleSetService struct {
	ctx context.Context
}

var runtimeState = struct {
	sync.Mutex
	engine   *Engine
	loadedAt time.Time
}{
	engine: NewEngine(),
}

func NewRuleSetService() *RuleSetService {
	return &RuleSetService{ctx: context.Background()}
}

func (s *RuleSetService) WithContext(ctx context.Context) *RuleSetService {
	return &RuleSetService{ctx: contextutil.OrBackground(ctx)}
}

func (s *RuleSetService) List(filters map[string]string, page, pageSize int) (request.PageResult[models.ProtectionRuleSet], error) {
	query := s.query().Table("protection_rule_set")
	query = query.Scopes(scopes.Contains("name", filters["name"]))
	query = query.Scopes(scopes.Equal("scope", strings.ToUpper(filters["scope"])))
	query = query.Scopes(scopes.Equal("status", strings.ToUpper(filters["status"])))
	return request.Paginate[models.ProtectionRuleSet](query.OrderByDesc("id"), page, pageSize)
}

func (s *RuleSetService) Find(id uint64) (models.ProtectionRuleSet, error) {
	var row models.ProtectionRuleSet
	err := s.query().Table("protection_rule_set").Where("id", id).First(&row)
	if err == frameworkerrors.OrmRecordNotFound {
		return models.ProtectionRuleSet{}, BusinessError{Message: "保护规则集不存在"}
	}
	return row, err
}

func (s *RuleSetService) Create(payload RuleSetPayload, operatorID uint64) (models.ProtectionRuleSet, error) {
	row := payload.ruleSet(true)
	row.Status = StatusDraft
	row.Version = 1
	row.CreatedBy = operatorID
	row.UpdatedBy = operatorID
	now := time.Now()
	row.CreatedAt = now
	row.UpdatedAt = now
	if err := validateRuleSetDefinition(row); err != nil {
		return models.ProtectionRuleSet{}, BusinessError{Message: err.Error()}
	}
	if err := s.createRuleSet(&row); err != nil {
		return models.ProtectionRuleSet{}, err
	}
	return row, nil
}

func (s *RuleSetService) Update(id uint64, payload RuleSetPayload, operatorID uint64) (models.ProtectionRuleSet, error) {
	existing, err := s.Find(id)
	if err != nil {
		return models.ProtectionRuleSet{}, err
	}
	if payload.Version < 1 || payload.Version != existing.Version {
		return models.ProtectionRuleSet{}, BusinessError{Message: "保护规则集版本冲突，请刷新后重试"}
	}
	next := payload.ruleSet(existing.Enabled)
	if err := validateRuleSetDefinition(next); err != nil {
		return models.ProtectionRuleSet{}, BusinessError{Message: err.Error()}
	}
	rulesJSON, err := json.Marshal(next.Rules)
	if err != nil {
		return models.ProtectionRuleSet{}, err
	}
	result, err := s.query().Exec(`
		UPDATE protection_rule_set
		SET name = ?, scope = ?, resource_pattern = ?, rules = ?::jsonb, status = ?,
			enabled = ?, version = ?, updated_by = ?, updated_at = ?
		WHERE id = ? AND version = ?
	`, next.Name, next.Scope, next.ResourcePattern, string(rulesJSON), StatusDraft,
		next.Enabled, existing.Version+1, operatorID, time.Now(), id, existing.Version)
	if err != nil {
		return models.ProtectionRuleSet{}, err
	}
	if result.RowsAffected != 1 {
		return models.ProtectionRuleSet{}, BusinessError{Message: "保护规则集版本冲突，请刷新后重试"}
	}
	return s.Find(id)
}

func (s *RuleSetService) Delete(id uint64, expectedVersion int) error {
	row, err := s.Find(id)
	if err != nil {
		return err
	}
	if row.Version != expectedVersion {
		return BusinessError{Message: "保护规则集版本冲突，请刷新后重试"}
	}
	versions, err := s.query().Table("protection_rule_version").Where("rule_set_id", id).Count()
	if err != nil {
		return err
	}
	if versions > 0 {
		return BusinessError{Message: "已发布的保护规则不可删除，请停用后保留历史"}
	}
	result, err := s.query().Table("protection_rule_set").Where("id", id).Where("version", expectedVersion).Delete()
	if err != nil {
		return err
	}
	if result.RowsAffected != 1 {
		return BusinessError{Message: "保护规则集版本冲突，请刷新后重试"}
	}
	return nil
}

func (s *RuleSetService) Validate(id uint64) (ValidationResult, error) {
	row, err := s.Find(id)
	if err != nil {
		return ValidationResult{}, err
	}
	result := ValidateRuleSet(row.Scope, row.ResourcePattern, row.Rules)
	if result.Valid && row.Enabled {
		candidate, candidateErr := publishedDefinition(row, row.PublishedVersion+1)
		if candidateErr != nil {
			result.Valid = false
			result.Errors = append(result.Errors, candidateErr.Error())
		} else if conflictErr := s.validatePublishConflicts(candidate); conflictErr != nil {
			result.Valid = false
			result.Errors = append(result.Errors, conflictErr.Error())
		}
	}
	return result, nil
}

func (s *RuleSetService) Publish(id uint64, expectedVersion int, operatorID uint64) (models.ProtectionRuleSet, error) {
	row, err := s.Find(id)
	if err != nil {
		return models.ProtectionRuleSet{}, err
	}
	if row.Version != expectedVersion {
		return models.ProtectionRuleSet{}, BusinessError{Message: "保护规则集版本冲突，请刷新后重试"}
	}
	if err := validateRuleSetDefinition(row); err != nil {
		return models.ProtectionRuleSet{}, BusinessError{Message: err.Error()}
	}
	nextPublishedVersion := row.PublishedVersion + 1
	candidate, err := publishedDefinition(row, nextPublishedVersion)
	if err != nil {
		return models.ProtectionRuleSet{}, err
	}
	if row.Enabled {
		if err := s.validatePublishConflicts(candidate); err != nil {
			return models.ProtectionRuleSet{}, BusinessError{Message: err.Error()}
		}
	}
	now := time.Now()
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		result, updateErr := tx.Table("protection_rule_set").
			Where("id", id).Where("version", expectedVersion).
			Update(map[string]any{
				"status": StatusPublished, "published_version": nextPublishedVersion,
				"published_at": now, "version": expectedVersion + 1,
				"updated_by": operatorID, "updated_at": now,
			})
		if updateErr != nil {
			return updateErr
		}
		if result.RowsAffected != 1 {
			return BusinessError{Message: "保护规则集版本冲突，请刷新后重试"}
		}
		return insertRuleVersion(tx, row, nextPublishedVersion, row.Enabled, operatorID, now)
	}); err != nil {
		return models.ProtectionRuleSet{}, err
	}
	if err := s.reloadRuntime(true); err != nil {
		return models.ProtectionRuleSet{}, err
	}
	return s.Find(id)
}

func (s *RuleSetService) PublishIdempotent(
	id uint64,
	expectedVersion int,
	operatorID uint64,
	idempotencyKey string,
) (models.ProtectionRuleSet, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return models.ProtectionRuleSet{}, BusinessError{Message: "保护规则发布幂等键不能为空"}
	}
	store := queueservice.NewDBQueueIdempotencyStore(platformConnection())
	result, err := store.Once(
		s.ctx,
		fmt.Sprintf("protection-rule-publish:%d:%x", id, sha256.Sum256([]byte(idempotencyKey))),
		func(context.Context) (queueservice.QueueIdempotencyResult, error) {
			ruleSet, publishErr := s.Publish(id, expectedVersion, operatorID)
			if publishErr != nil {
				return queueservice.QueueIdempotencyResult{}, publishErr
			}
			encoded, marshalErr := json.Marshal(ruleSet)
			if marshalErr != nil {
				return queueservice.QueueIdempotencyResult{}, marshalErr
			}
			return queueservice.QueueIdempotencyResult{
				Status: queueservice.QueueIdempotencyStatusSuccess,
				Result: string(encoded),
			}, nil
		},
	)
	if err != nil {
		return models.ProtectionRuleSet{}, err
	}
	if result.Status == queueservice.QueueIdempotencyStatusRunning || strings.TrimSpace(result.Result) == "" {
		return models.ProtectionRuleSet{}, BusinessError{Message: "保护规则发布操作正在处理中，请稍后重试"}
	}
	var ruleSet models.ProtectionRuleSet
	if err := json.Unmarshal([]byte(result.Result), &ruleSet); err != nil {
		return models.ProtectionRuleSet{}, err
	}
	return ruleSet, nil
}

func (s *RuleSetService) SetEnabled(id uint64, enabled bool, expectedVersion int, operatorID uint64) (models.ProtectionRuleSet, error) {
	row, err := s.Find(id)
	if err != nil {
		return models.ProtectionRuleSet{}, err
	}
	if row.Version != expectedVersion {
		return models.ProtectionRuleSet{}, BusinessError{Message: "保护规则集版本冲突，请刷新后重试"}
	}
	now := time.Now()
	if row.PublishedVersion < 1 {
		result, updateErr := s.query().Table("protection_rule_set").
			Where("id", id).Where("version", expectedVersion).
			Update(map[string]any{
				"enabled": enabled, "version": expectedVersion + 1,
				"updated_by": operatorID, "updated_at": now,
			})
		if updateErr != nil {
			return models.ProtectionRuleSet{}, updateErr
		}
		if result.RowsAffected != 1 {
			return models.ProtectionRuleSet{}, BusinessError{Message: "保护规则集版本冲突，请刷新后重试"}
		}
		return s.Find(id)
	}

	latest, err := s.latestPublishedVersion(id)
	if err != nil {
		return models.ProtectionRuleSet{}, err
	}
	nextPublishedVersion := latest.Version + 1
	versionContent := models.ProtectionRuleSet{
		ID: id, Name: latest.Name, Scope: latest.Scope,
		ResourcePattern: latest.ResourcePattern, Rules: latest.Rules,
	}
	if enabled {
		candidate, candidateErr := publishedDefinition(versionContent, nextPublishedVersion)
		if candidateErr != nil {
			return models.ProtectionRuleSet{}, candidateErr
		}
		if conflictErr := s.validatePublishConflicts(candidate); conflictErr != nil {
			return models.ProtectionRuleSet{}, BusinessError{Message: conflictErr.Error()}
		}
	}
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		result, updateErr := tx.Table("protection_rule_set").
			Where("id", id).Where("version", expectedVersion).
			Update(map[string]any{
				"enabled": enabled, "published_version": nextPublishedVersion,
				"version": expectedVersion + 1, "published_at": now,
				"updated_by": operatorID, "updated_at": now,
			})
		if updateErr != nil {
			return updateErr
		}
		if result.RowsAffected != 1 {
			return BusinessError{Message: "保护规则集版本冲突，请刷新后重试"}
		}
		return insertRuleVersion(tx, versionContent, nextPublishedVersion, enabled, operatorID, now)
	}); err != nil {
		return models.ProtectionRuleSet{}, err
	}
	if err := s.reloadRuntime(true); err != nil {
		return models.ProtectionRuleSet{}, err
	}
	return s.Find(id)
}

func (s *RuleSetService) Versions(id uint64) ([]models.ProtectionRuleVersion, error) {
	if _, err := s.Find(id); err != nil {
		return nil, err
	}
	rows := make([]models.ProtectionRuleVersion, 0)
	err := s.query().Table("protection_rule_version").
		Where("rule_set_id", id).OrderByDesc("version").Get(&rows)
	return rows, err
}

func (s *RuleSetService) Rollback(id uint64, targetVersion, expectedVersion int, operatorID uint64) (models.ProtectionRuleSet, error) {
	current, err := s.Find(id)
	if err != nil {
		return models.ProtectionRuleSet{}, err
	}
	if current.Version != expectedVersion {
		return models.ProtectionRuleSet{}, BusinessError{Message: "保护规则集版本冲突，请刷新后重试"}
	}
	var target models.ProtectionRuleVersion
	err = s.query().Table("protection_rule_version").
		Where("rule_set_id", id).Where("version", targetVersion).First(&target)
	if err == frameworkerrors.OrmRecordNotFound {
		return models.ProtectionRuleSet{}, BusinessError{Message: "目标保护规则版本不存在"}
	}
	if err != nil {
		return models.ProtectionRuleSet{}, err
	}
	nextPublishedVersion := current.PublishedVersion + 1
	rollback := models.ProtectionRuleSet{
		ID: current.ID, Name: target.Name, Scope: target.Scope,
		ResourcePattern: target.ResourcePattern, Rules: target.Rules,
		Enabled: target.Enabled,
	}
	candidate, err := publishedDefinition(rollback, nextPublishedVersion)
	if err != nil {
		return models.ProtectionRuleSet{}, err
	}
	if err := s.validatePublishConflicts(candidate); err != nil {
		return models.ProtectionRuleSet{}, BusinessError{Message: err.Error()}
	}
	rulesJSON, err := json.Marshal(target.Rules)
	if err != nil {
		return models.ProtectionRuleSet{}, err
	}
	now := time.Now()
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		result, updateErr := tx.Exec(`
			UPDATE protection_rule_set
			SET name = ?, scope = ?, resource_pattern = ?, rules = ?::jsonb,
				status = ?, enabled = ?, published_version = ?, published_at = ?,
				version = ?, updated_by = ?, updated_at = ?
			WHERE id = ? AND version = ?
		`, target.Name, target.Scope, target.ResourcePattern, string(rulesJSON),
			StatusPublished, target.Enabled, nextPublishedVersion, now,
			expectedVersion+1, operatorID, now, id, expectedVersion)
		if updateErr != nil {
			return updateErr
		}
		if result.RowsAffected != 1 {
			return BusinessError{Message: "保护规则集版本冲突，请刷新后重试"}
		}
		return insertRuleVersion(tx, rollback, nextPublishedVersion, target.Enabled, operatorID, now)
	}); err != nil {
		return models.ProtectionRuleSet{}, err
	}
	if err := s.reloadRuntime(true); err != nil {
		return models.ProtectionRuleSet{}, err
	}
	return s.Find(id)
}

func (s *RuleSetService) Evaluate(resourceName string, request RequestContext) (Decision, error) {
	if err := s.reloadRuntime(false); err != nil {
		return Decision{}, err
	}
	return runtimeState.engine.Evaluate(resourceName, request), nil
}

func (s *RuleSetService) State(id uint64) (RuleSetState, error) {
	if _, err := s.Find(id); err != nil {
		return RuleSetState{}, err
	}
	if err := s.reloadRuntime(false); err != nil {
		return RuleSetState{}, err
	}
	return runtimeState.engine.State(id), nil
}

func RuntimeMetrics() []Metric {
	return runtimeState.engine.Metrics()
}

func RecordSuccess(resourceName string, duration time.Duration) {
	runtimeState.engine.RecordSuccess(resourceName, duration)
}

func RecordFailure(resourceName string, duration time.Duration) {
	runtimeState.engine.RecordFailure(resourceName, duration)
}

func RecordDecisionSuccess(decision Decision, duration time.Duration) {
	runtimeState.engine.RecordDecisionSuccess(decision, duration)
}

func RecordDecisionFailure(decision Decision, duration time.Duration) {
	runtimeState.engine.RecordDecisionFailure(decision, duration)
}

func ResetRuntimeForTest() {
	runtimeState.Lock()
	defer runtimeState.Unlock()
	runtimeState.engine = NewEngine()
	runtimeState.loadedAt = time.Time{}
}

func (s *RuleSetService) createRuleSet(row *models.ProtectionRuleSet) error {
	rules := row.Rules
	return s.orm().Transaction(func(tx contractsorm.Query) error {
		row.Rules = nil
		if err := tx.Table("protection_rule_set").Create(row); err != nil {
			return err
		}
		row.Rules = rules
		rulesJSON, err := json.Marshal(rules)
		if err != nil {
			return err
		}
		_, err = tx.Exec(
			`UPDATE protection_rule_set SET rules = ?::jsonb WHERE id = ?`,
			string(rulesJSON),
			row.ID,
		)
		return err
	})
}

func (s *RuleSetService) validatePublishConflicts(candidate PublishedRuleSet) error {
	published, err := s.publishedDefinitions(candidate.RuleSetID)
	if err != nil {
		return err
	}
	published = append(published, candidate)
	return ValidatePublishedConflicts(published)
}

func (s *RuleSetService) publishedDefinitions(excludeID uint64) ([]PublishedRuleSet, error) {
	rows := make([]models.ProtectionRuleVersion, 0)
	if err := s.query().Raw(`
		SELECT version_row.*
		FROM protection_rule_version AS version_row
		INNER JOIN (
			SELECT rule_set_id, MAX(version) AS version
			FROM protection_rule_version
			GROUP BY rule_set_id
		) AS latest
			ON latest.rule_set_id = version_row.rule_set_id
			AND latest.version = version_row.version
		WHERE version_row.enabled = true
			AND (? = 0 OR version_row.rule_set_id <> ?)
		ORDER BY version_row.rule_set_id
	`, excludeID, excludeID).Scan(&rows); err != nil {
		return nil, err
	}
	result := make([]PublishedRuleSet, 0, len(rows))
	for _, row := range rows {
		definition, err := publishedVersionDefinition(row)
		if err != nil {
			return nil, err
		}
		result = append(result, definition)
	}
	return result, nil
}

func (s *RuleSetService) reloadRuntime(force bool) error {
	runtimeState.Lock()
	defer runtimeState.Unlock()
	if !force && !runtimeState.loadedAt.IsZero() && time.Since(runtimeState.loadedAt) < 5*time.Second {
		return nil
	}
	definitions, err := s.publishedDefinitions(0)
	if err != nil {
		return err
	}
	if err := runtimeState.engine.ReplaceRules(definitions); err != nil {
		return err
	}
	runtimeState.loadedAt = time.Now()
	return nil
}

func (s *RuleSetService) query() contractsorm.Query {
	return s.orm().Query()
}

func (s *RuleSetService) orm() contractsorm.Orm {
	return ormForContext(s.ctx)
}

func (p RuleSetPayload) ruleSet(enabledFallback bool) models.ProtectionRuleSet {
	enabled := enabledFallback
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	return models.ProtectionRuleSet{
		Name: strings.TrimSpace(p.Name), Scope: normalizeScope(p.Scope),
		ResourcePattern: normalizePattern(p.ResourcePattern), Rules: p.Rules,
		Enabled: enabled, Version: p.Version,
	}
}

func validateRuleSetDefinition(row models.ProtectionRuleSet) error {
	if row.Name == "" || len(row.Name) > 120 {
		return fmt.Errorf("保护规则集名称不能为空且不能超过 120 个字符")
	}
	return validationError(ValidateRuleSet(row.Scope, row.ResourcePattern, row.Rules))
}

func publishedDefinition(row models.ProtectionRuleSet, version int) (PublishedRuleSet, error) {
	rules, err := rulesFromJSON(row.Rules)
	if err != nil {
		return PublishedRuleSet{}, fmt.Errorf("规则集 %d 无法加载: %w", row.ID, err)
	}
	return PublishedRuleSet{
		RuleSetID: row.ID, Version: version, Name: row.Name,
		Scope: row.Scope, ResourcePattern: row.ResourcePattern, Rules: rules,
	}, nil
}

func publishedVersionDefinition(row models.ProtectionRuleVersion) (PublishedRuleSet, error) {
	rules, err := rulesFromJSON(row.Rules)
	if err != nil {
		return PublishedRuleSet{}, fmt.Errorf("规则集 %d 版本 %d 无法加载: %w", row.RuleSetID, row.Version, err)
	}
	return PublishedRuleSet{
		RuleSetID: row.RuleSetID, Version: row.Version, Name: row.Name,
		Scope: row.Scope, ResourcePattern: row.ResourcePattern, Rules: rules,
	}, nil
}

func (s *RuleSetService) latestPublishedVersion(id uint64) (models.ProtectionRuleVersion, error) {
	var row models.ProtectionRuleVersion
	err := s.query().Table("protection_rule_version").
		Where("rule_set_id", id).OrderByDesc("version").First(&row)
	if err == frameworkerrors.OrmRecordNotFound {
		return models.ProtectionRuleVersion{}, BusinessError{Message: "保护规则集尚未发布"}
	}
	return row, err
}

func insertRuleVersion(
	query contractsorm.Query,
	row models.ProtectionRuleSet,
	version int,
	enabled bool,
	operatorID uint64,
	publishedAt time.Time,
) error {
	rulesJSON, err := json.Marshal(row.Rules)
	if err != nil {
		return err
	}
	_, err = query.Exec(`
		INSERT INTO protection_rule_version (
			rule_set_id, version, name, scope, resource_pattern, rules, enabled,
			published_by, published_at, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?::jsonb, ?, ?, ?, ?, ?)
	`, row.ID, version, row.Name, row.Scope, row.ResourcePattern, string(rulesJSON),
		enabled, operatorID, publishedAt, publishedAt, publishedAt)
	return err
}
