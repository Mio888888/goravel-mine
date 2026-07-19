package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goravel/framework/contracts/database/driver"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"
	frameworkerrors "github.com/goravel/framework/errors"
	postgresfacades "github.com/goravel/postgres/facades"

	"goravel/app/facades"
	"goravel/app/http/request"
	"goravel/app/models"
	"goravel/app/scopes"
	"goravel/database/migrations"
	"goravel/database/seeders"
)

const (
	TenantStatusActive    int8 = 1
	TenantStatusSuspended int8 = 2
	TenantStatusArchived  int8 = 3
)

var tenantModuleMigrationsProvider func() []schema.Migration

func SetTenantModuleMigrationsProvider(provider func() []schema.Migration) {
	tenantModuleMigrationsProvider = provider
}

var (
	ErrTenantRequired  = errors.New("tenant is required")
	ErrTenantNotFound  = errors.New("tenant not found")
	ErrTenantSuspended = errors.New("tenant is not active")
)

type Tenant = models.Tenant

type TenantService struct {
	ctx context.Context
}

type tenantContextKey struct{}

var (
	tenantInitializeMu         sync.Mutex
	tenantConnectionRegisterMu sync.Mutex
)

type TenantPayload struct {
	Code         string         `json:"code"`
	Name         string         `json:"name"`
	Status       int8           `json:"status"`
	Plan         string         `json:"plan"`
	DBHost       string         `json:"db_host"`
	DBPort       int            `json:"db_port"`
	DBDatabase   string         `json:"db_database"`
	DBUsername   string         `json:"db_username"`
	DBPassword   string         `json:"db_password"`
	DBSchema     string         `json:"db_schema"`
	CustomDomain string         `json:"custom_domain"`
	Branding     models.JSONMap `json:"branding"`
	Billing      models.JSONMap `json:"billing"`
	Quotas       models.JSONMap `json:"quotas"`
	Features     models.JSONMap `json:"features"`
	Remark       string         `json:"remark"`
	Initialize   bool           `json:"initialize"`
}

type TenantDestroyPayload struct {
	IDs          []uint64 `json:"ids"`
	ConfirmCode  string   `json:"confirm_code"`
	DropDatabase bool     `json:"drop_database"`
	ReAuthToken  string   `json:"reauth_token"`
	ApprovalID   string   `json:"approval_id"`
	OperatorID   uint64   `json:"-"`
}

type TenantPlanUpdatePayload struct {
	Plan     string         `json:"plan"`
	Features models.JSONMap `json:"features"`
}

type TenantPermissionOperator struct {
	ID   uint64
	Name string
}

type PostgresProvisionPlan struct {
	PlatformStatements []string
	TenantStatements   []string
}

func NewTenantService() *TenantService {
	return &TenantService{}
}

func (s *TenantService) WithContext(ctx context.Context) *TenantService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *TenantService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, PlatformConnection())
}

func TenantContextKey() any {
	return tenantContextKey{}
}

func (p TenantPayload) Tenant() Tenant {
	status := p.Status
	if status == 0 {
		status = TenantStatusActive
	}
	plan := strings.TrimSpace(p.Plan)
	if plan == "" {
		plan = "standard"
	}
	var customDomain *string
	if value := strings.TrimSpace(p.CustomDomain); value != "" {
		customDomain = &value
	}
	dbPort := p.DBPort
	if dbPort == 0 {
		dbPort = 5432
	}
	dbDatabase := strings.TrimSpace(p.DBDatabase)
	if dbDatabase == "" {
		dbDatabase = defaultTenantDatabaseName(p.Code)
	}
	dbSchema := strings.TrimSpace(p.DBSchema)
	if dbSchema == "" {
		dbSchema = "public"
	}

	return Tenant{
		Code:         strings.TrimSpace(p.Code),
		Name:         strings.TrimSpace(p.Name),
		Status:       status,
		Plan:         plan,
		DBHost:       strings.TrimSpace(p.DBHost),
		DBPort:       dbPort,
		DBDatabase:   dbDatabase,
		DBUsername:   strings.TrimSpace(p.DBUsername),
		DBPassword:   p.DBPassword,
		DBSchema:     dbSchema,
		CustomDomain: customDomain,
		Branding:     p.Branding,
		Billing:      p.Billing,
		Quotas:       p.Quotas,
		Features:     platformManagedFeatures(p.Features),
		Remark:       p.Remark,
	}
}

func (s *TenantService) List(filters map[string]string, page, pageSize int) (request.PageResult[Tenant], error) {
	query := s.orm().
		Query().
		Table("tenant")
	query = query.Scopes(scopes.Contains("code", filters["code"]))
	query = query.Scopes(scopes.Contains("name", filters["name"]))
	query = query.Scopes(scopes.Contains("plan", filters["plan"]))
	query = query.Scopes(scopes.EqualIfPresent("status", filters["status"]))
	return request.Paginate[Tenant](query.OrderByDesc("id"), page, pageSize)
}

func (s *TenantService) Create(input TenantPayload) (Tenant, error) {
	tenant := input.Tenant()
	if err := s.applyPlanFeatureSnapshot(&tenant, featuresWithoutTenantPermissions(input.Features)); err != nil {
		return Tenant{}, err
	}
	if input.Initialize {
		var err error
		tenant, err = ApplyPostgresProvisionDefaults(tenant)
		if err != nil {
			return Tenant{}, err
		}
	}
	if err := s.validateTenant(tenant); err != nil {
		return Tenant{}, err
	}
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		row := Tenant{
			Code: tenant.Code, Name: tenant.Name, Status: tenant.Status,
			Plan: tenant.Plan, DBHost: tenant.DBHost, DBPort: tenant.DBPort,
			DBDatabase: tenant.DBDatabase, DBUsername: tenant.DBUsername,
			DBPassword: tenant.DBPassword, DBSchema: tenant.DBSchema,
			CustomDomain: tenant.CustomDomain, Remark: tenant.Remark,
		}
		if err := tx.Table("tenant").Create(&row); err != nil {
			return err
		}
		tenant.ID = row.ID
		return updateTenantJSONColumnsWithQuery(tx, tenant.ID, tenant)
	}); err != nil {
		return Tenant{}, err
	}
	if input.Initialize {
		if err := s.ProvisionPostgresTenant(tenant); err != nil {
			return Tenant{}, err
		}
		if err := s.InitializeDatabase(tenant); err != nil {
			return Tenant{}, err
		}
	}
	return tenant, nil
}

func (s *TenantService) Update(id uint64, input TenantPayload) (Tenant, error) {
	existing, err := s.FindByID(id)
	if err != nil {
		return Tenant{}, err
	}
	tenant := input.Tenant()
	if tenant.Plan != existing.Plan || tenant.Status != existing.Status {
		return Tenant{}, ErrSensitiveOperationPolicy
	}
	features := featuresWithoutTenantPermissions(input.Features)
	if tenant.Plan == existing.Plan {
		features = preserveTenantPermissionFeature(features, existing.Features)
	}
	if err := s.applyPlanFeatureSnapshot(&tenant, features); err != nil {
		return Tenant{}, err
	}
	if err := s.validateTenant(tenant); err != nil {
		return Tenant{}, err
	}

	values := map[string]any{
		"code": tenant.Code, "name": tenant.Name, "status": tenant.Status,
		"plan": tenant.Plan, "db_host": tenant.DBHost, "db_port": tenant.DBPort,
		"db_database": tenant.DBDatabase, "db_username": tenant.DBUsername,
		"db_schema": tenant.DBSchema, "custom_domain": tenant.CustomDomain,
		"remark":     tenant.Remark,
		"updated_at": time.Now(),
	}
	if tenant.DBPassword != "" {
		values["db_password"] = tenant.DBPassword
	}

	if _, err := s.orm().
		Query().
		Table("tenant").
		Where("id", id).
		Update(values); err != nil {
		return Tenant{}, err
	}
	if err := s.updateTenantJSONColumns(id, tenant); err != nil {
		return Tenant{}, err
	}
	tenant.ID = id
	return tenant, nil
}

func (s *TenantService) applyPlanFeatureSnapshot(tenant *Tenant, inputFeatures models.JSONMap) error {
	plan, err := NewTenantPlanService().WithContext(s.ctx).ActiveByCode(tenant.Plan)
	if err != nil {
		return err
	}
	if plan.ID == 0 {
		return BusinessError{Message: "租户套餐不存在或已禁用"}
	}
	tenant.Features = SnapshotFeaturesForPlan(plan.Features, inputFeatures)
	return nil
}

func (s *TenantService) updateTenantJSONColumns(id uint64, tenant Tenant) error {
	return updateTenantJSONColumnsWithQuery(s.orm().Query(), id, tenant)
}

func updateTenantJSONColumnsWithQuery(query contractsorm.Query, id uint64, tenant Tenant) error {
	branding, err := json.Marshal(nullIfEmpty(tenant.Branding))
	if err != nil {
		return err
	}
	billing, err := json.Marshal(nullIfEmpty(tenant.Billing))
	if err != nil {
		return err
	}
	quotas, err := json.Marshal(nullIfEmpty(tenant.Quotas))
	if err != nil {
		return err
	}
	features, err := json.Marshal(nullIfEmpty(tenant.Features))
	if err != nil {
		return err
	}
	_, err = query.Exec(
		"UPDATE tenant SET branding = ?::jsonb, billing = ?::jsonb, quotas = ?::jsonb, features = ?::jsonb WHERE id = ?",
		string(branding), string(billing), string(quotas), string(features), id,
	)
	return err
}

func nullIfEmpty(value models.JSONMap) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func platformManagedFeatures(input models.JSONMap) models.JSONMap {
	features := models.JSONMap{}
	for key, value := range input {
		if key == "sso" {
			continue
		}
		features[key] = value
	}
	return features
}

func (s *TenantService) Suspend(id uint64) error {
	return s.UpdateStatus(id, TenantStatusSuspended)
}

func (s *TenantService) Resume(id uint64) error {
	return s.UpdateStatus(id, TenantStatusActive)
}

func (s *TenantService) Archive(id uint64) error {
	return s.UpdateStatus(id, TenantStatusArchived)
}

func (s *TenantService) UpdateStatus(id uint64, status int8) error {
	if !validTenantStatus(status) {
		return BusinessError{Message: "租户状态无效"}
	}
	if err := s.ensureTenantExists(id); err != nil {
		return err
	}
	_, err := s.orm().
		Query().
		Table("tenant").
		Where("id", id).
		Update(map[string]any{"status": status, "updated_at": time.Now()})
	return err
}

func (s *TenantService) Usage(id uint64) (TenantUsageReport, error) {
	tenant, err := s.FindByID(id)
	if err != nil {
		return TenantUsageReport{}, ErrTenantNotFound
	}
	return NewTenantRuntimeService().WithContext(s.ctx).Usage(tenant)
}

func (s *TenantService) Permissions(id uint64) (TenantPermissionPayload, error) {
	tenant, err := s.FindByID(id)
	if err != nil {
		return TenantPermissionPayload{}, err
	}
	return TenantEffectivePermissionPayload(tenant), nil
}

func (s *TenantService) UpdatePermissions(id uint64, payload TenantPermissionPayload, operator TenantPermissionOperator) (TenantPermissionPayload, error) {
	tenant, err := s.FindByID(id)
	if err != nil {
		return TenantPermissionPayload{}, err
	}
	before := TenantEffectivePermissionPayload(tenant)
	tenant.Features = platformManagedFeatures(tenant.Features)
	tenant.Features[tenantPermissionsFeatureKey] = permissionPayloadMap(payload)
	after := TenantPermissionPayloadFromTenant(tenant)

	err = s.orm().Transaction(func(tx contractsorm.Query) error {
		if err := updateTenantJSONColumnsWithQuery(tx, id, tenant); err != nil {
			return err
		}
		return NewTenantPermissionAuditService().LogWithQuery(tx, TenantPermissionAuditInput{
			TenantID:     tenant.ID,
			TenantCode:   tenant.Code,
			Operation:    TenantPermissionAuditOperationUpdate,
			Source:       TenantPermissionAuditSourcePlatform,
			Before:       before,
			After:        after,
			OperatorID:   operator.ID,
			OperatorName: operator.Name,
		})
	})
	if err != nil {
		return TenantPermissionPayload{}, err
	}
	return TenantPermissionPayloadFromTenant(tenant), nil
}

func (s *TenantService) PermissionPlanDiff(id uint64, targetPlan string) (TenantPermissionPlanDiff, error) {
	tenant, err := s.FindByID(id)
	if err != nil {
		return TenantPermissionPlanDiff{}, err
	}
	if strings.TrimSpace(targetPlan) == "" {
		targetPlan = tenant.Plan
	}
	plan, err := NewTenantPlanService().WithContext(s.ctx).ActiveByCode(targetPlan)
	if err != nil {
		return TenantPermissionPlanDiff{}, err
	}
	if plan.ID == 0 {
		return TenantPermissionPlanDiff{}, BusinessError{Message: "租户套餐不存在或已禁用"}
	}
	current := TenantEffectivePermissionPayload(tenant)
	next, _ := tenantPermissionPayloadFromFeatures(SnapshotFeaturesForPlan(plan.Features, models.JSONMap{}))
	next = normalizePermissionPayload(next)
	return TenantPermissionPlanDiff{
		Plan:       plan.Code,
		Allowed:    next.Allowed,
		Added:      sortedSetDiff(next.Allowed, current.Allowed),
		Removed:    sortedSetDiff(current.Allowed, next.Allowed),
		Unchanged:  sortedSetIntersect(current.Allowed, next.Allowed),
		Permission: next.Allowed,
	}, nil
}

func (s *TenantService) UpdatePlan(id uint64, input TenantPlanUpdatePayload, operator TenantPermissionOperator) (Tenant, error) {
	tenant, err := s.FindByID(id)
	if err != nil {
		return Tenant{}, err
	}
	plan := strings.TrimSpace(input.Plan)
	if plan == "" {
		return Tenant{}, BusinessError{Message: "租户套餐不能为空"}
	}
	planModel, err := NewTenantPlanService().WithContext(s.ctx).ActiveByCode(plan)
	if err != nil {
		return Tenant{}, err
	}
	if planModel.ID == 0 {
		return Tenant{}, BusinessError{Message: "租户套餐不存在或已禁用"}
	}
	before := TenantEffectivePermissionPayload(tenant)
	features := input.Features
	if features == nil {
		features = models.JSONMap{}
	}
	tenant.Plan = plan
	tenant.Features = SnapshotFeaturesForPlan(planModel.Features, features)
	after := TenantPermissionPayloadFromTenant(tenant)

	err = s.orm().Transaction(func(tx contractsorm.Query) error {
		if _, err := tx.
			Table("tenant").
			Where("id", id).
			Update(map[string]any{"plan": tenant.Plan, "updated_at": time.Now()}); err != nil {
			return err
		}
		if err := updateTenantJSONColumnsWithQuery(tx, id, tenant); err != nil {
			return err
		}
		return NewTenantPermissionAuditService().LogWithQuery(tx, TenantPermissionAuditInput{
			TenantID:     tenant.ID,
			TenantCode:   tenant.Code,
			Operation:    TenantPermissionAuditOperationPlanChange,
			Source:       TenantPermissionAuditSourcePlanChange,
			Before:       before,
			After:        after,
			OperatorID:   operator.ID,
			OperatorName: operator.Name,
			Remark:       "update tenant plan to " + tenant.Plan,
		})
	})
	if err != nil {
		return Tenant{}, err
	}
	return tenant, nil
}

func (s *TenantService) SnapshotLegacyPermissions(dryRun bool) (int, error) {
	tenants := make([]Tenant, 0)
	if err := s.orm().
		Query().
		Table("tenant").
		OrderBy("id").
		Get(&tenants); err != nil {
		return 0, err
	}

	count := 0
	for _, tenant := range tenants {
		snapshot, ok := BuildLegacyTenantPermissionSnapshot(tenant)
		if !ok {
			continue
		}
		count++
		if dryRun {
			continue
		}
		before := TenantEffectivePermissionPayload(tenant)
		tenant.Features = platformManagedFeatures(tenant.Features)
		tenant.Features[tenantPermissionsFeatureKey] = permissionPayloadMap(snapshot)
		if err := s.orm().Transaction(func(tx contractsorm.Query) error {
			if err := updateTenantJSONColumnsWithQuery(tx, tenant.ID, tenant); err != nil {
				return err
			}
			return NewTenantPermissionAuditService().LogWithQuery(tx, TenantPermissionAuditInput{
				TenantID:     tenant.ID,
				TenantCode:   tenant.Code,
				Operation:    TenantPermissionAuditOperationLegacySnapshot,
				Source:       TenantPermissionAuditSourceLegacyMigration,
				Before:       before,
				After:        snapshot,
				OperatorName: TenantPermissionAuditSystemOperatorName,
				Remark:       "snapshot legacy full permissions",
			})
		}); err != nil {
			return count - 1, err
		}
	}
	return count, nil
}

func (s *TenantService) FindByID(id uint64) (Tenant, error) {
	var tenant Tenant
	err := s.orm().
		Query().
		Where("id", id).
		First(&tenant)
	if err != nil || tenant.ID == 0 {
		return Tenant{}, ErrTenantNotFound
	}
	RegisterTenantConnection(tenant)
	return tenant, nil
}

func (s *TenantService) Destroy(input TenantDestroyPayload) error {
	if len(input.IDs) == 0 {
		return nil
	}
	if input.DropDatabase && len(input.IDs) != 1 {
		return BusinessError{Message: "物理库删除仅支持单租户"}
	}

	tenants := make([]Tenant, 0, len(input.IDs))
	if err := s.orm().
		Query().
		Table("tenant").
		WhereIn("id", uint64Any(input.IDs)).
		Get(&tenants); err != nil {
		return err
	}
	if len(tenants) != len(input.IDs) {
		return ErrTenantNotFound
	}
	if len(tenants) == 1 && strings.TrimSpace(input.ConfirmCode) != tenants[0].Code {
		return BusinessError{Message: "租户销毁确认码不匹配"}
	}
	deleteMode := "metadata"
	if input.DropDatabase {
		deleteMode = "database"
	}
	resource := TenantDataActionResource("delete", input.IDs, deleteMode)
	approvalRequired := false
	for _, tenant := range tenants {
		policy, err := NewTenantGovernanceService().WithContext(s.ctx).Policy(tenant)
		if err != nil {
			return err
		}
		if !policy.DataDeletion.Enabled {
			return BusinessError{Message: "租户治理策略禁止数据删除"}
		}
		approvalRequired = approvalRequired || policy.DataDeletion.RequiresApproval
	}
	security := NewEnterpriseSecurityControlService()
	request := SensitiveOperationRequest{
		UserID: input.OperatorID, Operation: "tenant.data.delete", Resource: resource, ReAuthToken: input.ReAuthToken,
	}
	destroy := func() error {
		if input.DropDatabase {
			for _, tenant := range tenants {
				if err := s.DropTenantDatabase(tenant, input.ConfirmCode); err != nil {
					return err
				}
			}
		}
		_, err := s.orm().Query().Table("tenant").WhereIn("id", uint64Any(input.IDs)).Delete()
		return err
	}
	if !approvalRequired {
		if err := security.ExecuteSensitiveOperation(request, destroy); err != nil {
			if !errors.Is(err, ErrReAuthRequired) {
				return err
			}
			return BusinessError{Message: "tenant deletion requires valid re-auth token"}
		}
	} else {
		err := security.ExecuteSensitiveOperationWithApproval(
			s.ctx, request, input.ApprovalID, input.OperatorID, "tenant.data.delete", resource, destroy,
		)
		if errors.Is(err, ErrReAuthRequired) {
			return BusinessError{Message: "tenant deletion requires valid re-auth token"}
		}
		if errors.Is(err, ErrApprovalRequired) || errors.Is(err, ErrApprovalSelfApproved) {
			return BusinessError{Message: "tenant deletion requires approved approval record"}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *TenantService) DropTenantDatabase(tenant Tenant, confirmCode string) error {
	if strings.TrimSpace(confirmCode) != tenant.Code {
		return BusinessError{Message: "租户物理库删除确认码不匹配"}
	}
	platformDB := facades.Config().GetString("database.connections." + PlatformConnection() + ".database")
	if tenant.DBDatabase == "" || tenant.DBDatabase == platformDB {
		return BusinessError{Message: "拒绝删除平台数据库"}
	}
	_, err := s.orm().
		Query().
		Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", quoteIdentifier(tenant.DBDatabase)))
	return err
}

func (s *TenantService) validateTenant(tenant Tenant) error {
	if tenant.Code == "" || tenant.Name == "" || tenant.DBDatabase == "" {
		return BusinessError{Message: "租户编码、名称和数据库名不能为空"}
	}
	if !validTenantStatus(tenant.Status) {
		return BusinessError{Message: "租户状态无效"}
	}
	exists, err := NewTenantPlanService().WithContext(s.ctx).ExistsActive(tenant.Plan)
	if err != nil {
		return err
	}
	if !exists {
		return BusinessError{Message: "租户套餐不存在或已禁用"}
	}
	return nil
}

func validTenantStatus(status int8) bool {
	return status == TenantStatusActive || status == TenantStatusSuspended || status == TenantStatusArchived
}

func (s *TenantService) ensureTenantExists(id uint64) error {
	count, err := s.orm().
		Query().
		Table("tenant").
		Where("id", id).
		Count()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrTenantNotFound
	}
	return nil
}

func WithTenant(ctx context.Context, tenant Tenant) context.Context {
	return context.WithValue(ctx, TenantContextKey(), tenant)
}

func CurrentTenant(ctx context.Context) (Tenant, bool) {
	tenant, ok := ctx.Value(TenantContextKey()).(Tenant)
	return tenant, ok
}

func TenantOrm(ctx context.Context) string {
	if tenant, ok := CurrentTenant(ctx); ok {
		return TenantConnectionName(tenant)
	}
	return facades.Config().GetString("database.default")
}

func (s *TenantService) Resolve(code string) (Tenant, error) {
	return s.ResolveByCodeOrHost(code, "")
}

func (s *TenantService) FindByCustomDomain(host string) (Tenant, error) {
	host = TenantHostCode(host)
	if host == "" {
		return Tenant{}, ErrTenantNotFound
	}

	var tenant Tenant
	err := s.orm().
		Query().
		Where("custom_domain", host).
		First(&tenant)
	if err != nil {
		if errors.Is(err, frameworkerrors.OrmRecordNotFound) {
			return Tenant{}, ErrTenantNotFound
		}
		return Tenant{}, err
	}
	if tenant.ID == 0 {
		return Tenant{}, ErrTenantNotFound
	}
	RegisterTenantConnection(tenant)
	return tenant, nil
}

func (s *TenantService) ResolveByCodeOrHost(code, host string) (Tenant, error) {
	code = strings.TrimSpace(code)
	host = TenantHostCode(host)
	if code == "" && host == "" {
		code = facades.Config().GetString("tenant.default")
	}
	if code == "" && host == "" {
		return Tenant{}, ErrTenantRequired
	}

	var tenant Tenant
	query := s.orm().
		Query()
	if code != "" {
		query = query.Where("code", code)
	} else {
		query = query.Where("custom_domain", host)
	}
	err := query.First(&tenant)
	if err != nil || tenant.ID == 0 {
		if code == "" && host != "" {
			if fallback := facades.Config().GetString("tenant.default"); fallback != "" {
				return s.Resolve(fallback)
			}
		}
		return Tenant{}, ErrTenantNotFound
	}
	if tenant.Status != TenantStatusActive {
		return Tenant{}, ErrTenantSuspended
	}

	RegisterTenantConnection(tenant)
	return tenant, nil
}

func PlatformConnection() string {
	connection := facades.Config().GetString("tenant.platform_connection")
	if connection == "" {
		return facades.Config().GetString("database.default")
	}
	return connection
}

func RegisterTenantConnection(tenant Tenant) string {
	tenantConnectionRegisterMu.Lock()
	defer tenantConnectionRegisterMu.Unlock()
	name := TenantConnectionName(tenant)
	configs := facades.Config().Get("database.connections", map[string]any{})
	merged := make(map[string]any)
	if existing, ok := configs.(map[string]any); ok {
		for key, value := range existing {
			merged[key] = value
		}
	}
	connectionConfig := TenantDatabaseConfig(tenant)
	if err := RegisterTenantConnectionCapacity(name); err != nil {
		connectionConfig["via"] = func() (driver.Driver, error) { return nil, err }
	}
	merged[name] = connectionConfig
	facades.Config().Add("database.connections", merged)
	return name
}

func TenantConnectionName(tenant Tenant) string {
	code := strings.ToLower(strings.TrimSpace(tenant.Code))
	code = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(code, "_")
	code = strings.Trim(code, "_")
	if code == "" {
		code = "default"
	}
	return "tenant_" + strconv.FormatUint(tenant.ID, 10) + "_" + code
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func quoteLiteral(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `''`) + `'`
}

func ApplyPostgresProvisionDefaults(tenant Tenant) (Tenant, error) {
	if strings.TrimSpace(tenant.DBUsername) == "" {
		tenant.DBUsername = defaultTenantDBUsername(tenant.Code)
	}
	if strings.TrimSpace(tenant.DBPassword) == "" {
		password, err := randomTenantPassword()
		if err != nil {
			return Tenant{}, err
		}
		tenant.DBPassword = password
	}
	return tenant, nil
}

func NewPostgresProvisionPlan(tenant Tenant) (PostgresProvisionPlan, error) {
	if strings.TrimSpace(tenant.DBDatabase) == "" || strings.TrimSpace(tenant.DBUsername) == "" || tenant.DBPassword == "" {
		return PostgresProvisionPlan{}, BusinessError{Message: "租户数据库名、用户和密码不能为空"}
	}
	schema := strings.TrimSpace(tenant.DBSchema)
	if schema == "" {
		schema = "public"
	}

	database := quoteIdentifier(tenant.DBDatabase)
	username := quoteIdentifier(tenant.DBUsername)
	schemaName := quoteIdentifier(schema)
	password := quoteLiteral(tenant.DBPassword)
	roleName := quoteLiteral(tenant.DBUsername)

	return PostgresProvisionPlan{
		PlatformStatements: []string{
			fmt.Sprintf("DO $$\nBEGIN\n\tIF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = %s) THEN\n\t\tCREATE ROLE %s LOGIN PASSWORD %s;\n\tELSE\n\t\tALTER ROLE %s WITH LOGIN PASSWORD %s;\n\tEND IF;\nEND $$", roleName, username, password, username, password),
			fmt.Sprintf("CREATE DATABASE %s OWNER %s", database, username),
			fmt.Sprintf("REVOKE CONNECT ON DATABASE %s FROM PUBLIC", database),
			fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO %s", database, username),
		},
		TenantStatements: []string{
			fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s AUTHORIZATION %s", schemaName, username),
			fmt.Sprintf("GRANT USAGE, CREATE ON SCHEMA %s TO %s", schemaName, username),
			fmt.Sprintf("GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA %s TO %s", schemaName, username),
			fmt.Sprintf("GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA %s TO %s", schemaName, username),
			fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT ALL PRIVILEGES ON TABLES TO %s", schemaName, username),
			fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT ALL PRIVILEGES ON SEQUENCES TO %s", schemaName, username),
		},
	}, nil
}

func (s *TenantService) ProvisionPostgresTenant(tenant Tenant) error {
	plan, err := NewPostgresProvisionPlan(tenant)
	if err != nil {
		return err
	}
	if err := s.runPostgresPlatformProvision(tenant, plan.PlatformStatements); err != nil {
		return err
	}

	connection := RegisterTenantConnection(tenant)
	for _, statement := range plan.TenantStatements {
		if _, err := OrmForConnectionWithContext(s.ctx, connection).Query().Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *TenantService) runPostgresPlatformProvision(tenant Tenant, statements []string) error {
	for index, statement := range statements {
		if index == 1 {
			exists, err := s.databaseExists(tenant.DBDatabase)
			if err != nil {
				return err
			}
			if exists {
				continue
			}
		}
		if _, err := s.orm().Query().Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *TenantService) databaseExists(name string) (bool, error) {
	count, err := s.orm().
		Query().
		Table("pg_database").
		Where("datname", name).
		Count()
	return count > 0, err
}

func defaultTenantDBUsername(code string) string {
	return "tenant_" + normalizedTenantCode(code)
}

func defaultTenantDatabaseName(code string) string {
	return "tenant_" + normalizedTenantCode(code)
}

func normalizedTenantCode(code string) string {
	name := strings.ToLower(strings.TrimSpace(code))
	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		name = "default"
	}
	return name
}

func randomTenantPassword() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func TenantDatabaseConfig(tenant Tenant) map[string]any {
	port := tenant.DBPort
	if port == 0 {
		port = 5432
	}
	schema := tenant.DBSchema
	if strings.TrimSpace(schema) == "" {
		schema = "public"
	}

	connection := TenantConnectionName(tenant)
	return map[string]any{
		"host":     tenant.DBHost,
		"port":     port,
		"database": tenant.DBDatabase,
		"username": tenant.DBUsername,
		"password": tenant.DBPassword,
		"sslmode":  "disable",
		"singular": false,
		"prefix":   "",
		"schema":   schema,
		"via": func() (driver.Driver, error) {
			return postgresfacades.Postgres(connection)
		},
	}
}

func TenantBusinessMigrations() []schema.Migration {
	items := []schema.Migration{
		&migrations.M202606290001CreateCasbinRuleTable{},
		&migrations.M202606290002CreateUserTable{},
		&migrations.M202606290003CreateRoleTable{},
		&migrations.M202606290004CreateMenuTable{},
		&migrations.M202606290005CreateRoleBelongsMenuTable{},
		&migrations.M202606290006CreateUserBelongsRoleTable{},
		&migrations.M202606290007CreateDepartmentTables{},
		&migrations.M202606290008CreateAttachmentTable{},
		&migrations.M202607030002AddStorageConfigIDToAttachmentTable{},
		&migrations.M202606290009CreateUserLoginLogTable{},
		&migrations.M202606290010CreateUserOperationLogTable{},
		&migrations.M202606290012CreateSSOProviderTable{},
		&migrations.M202606300003CreateSSOUserBindingTable{},
		&migrations.M202606300004CreateSSOLoginLogTable{},
		&migrations.M202606300005CreateDictionaryTables{},
		&migrations.M202607050001CreateUserMFATable{},
		&migrations.M202607050003CreateUserPasswordHistoryTable{},
		&migrations.M202607050005AddSecretRotationMetadata{},
	}
	if tenantModuleMigrationsProvider != nil {
		items = append(items, tenantModuleMigrationsProvider()...)
	}
	return items
}

func TenantInitialSeeders() []seeder.Seeder {
	return []seeder.Seeder{
		&seeders.AdminSeeder{},
		&seeders.MenuSeeder{},
		&seeders.DictionarySeeder{},
		&seeders.DepartmentSeeder{},
		&seeders.CasbinSeeder{},
	}
}

func (s *TenantService) InitializeDatabase(tenant Tenant) error {
	tenantInitializeMu.Lock()
	defer tenantInitializeMu.Unlock()

	if err := runTenantMigrations(tenant); err != nil {
		return err
	}

	connection := TenantConnectionName(tenant)
	restoreSeederConnection := seeders.SetConnection(connection)
	defer restoreSeederConnection()
	for _, item := range TenantInitialSeeders() {
		if err := item.Run(); err != nil {
			return err
		}
	}
	return NewTenantDictionaryServiceForTenant(tenant).SyncFromPlatform()
}

func (s *TenantService) MigrateAllTenants(dryRun bool) (int, error) {
	tenants := make([]Tenant, 0)
	if err := s.orm().Query().Table("tenant").Get(&tenants); err != nil {
		return 0, err
	}
	if dryRun {
		return len(tenants), nil
	}
	for _, tenant := range tenants {
		if err := runTenantMigrations(tenant); err != nil {
			return 0, err
		}
	}
	return len(tenants), nil
}

func runTenantMigrations(tenant Tenant) error {
	connection := RegisterTenantConnection(tenant)
	previous := facades.Schema().GetConnection()
	facades.Schema().SetConnection(connection)
	defer facades.Schema().SetConnection(previous)

	for _, migration := range TenantBusinessMigrations() {
		if err := migration.Up(); err != nil {
			return err
		}
	}
	return nil
}
