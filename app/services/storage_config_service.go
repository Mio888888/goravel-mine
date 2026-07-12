package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	frameworkerrors "github.com/goravel/framework/errors"

	"goravel/app/facades"
	"goravel/app/http/request"
	"goravel/app/models"
)

const (
	StorageConfigStatusEnabled int8 = 1
	storageDriverLocal              = "local"
	storageDriverS3Compatible       = "s3_compatible"
)

var supportedStorageProviders = map[string]struct{}{
	"local":       {},
	"minio":       {},
	"aws_s3":      {},
	"aliyun_oss":  {},
	"tencent_cos": {},
	"qiniu":       {},
	"huawei_obs":  {},
}

type StorageConfig = models.StorageConfig

type StorageConfigPayload struct {
	Name       string         `json:"name"`
	Provider   string         `json:"provider"`
	Driver     string         `json:"driver"`
	Bucket     string         `json:"bucket"`
	Endpoint   string         `json:"endpoint"`
	Region     string         `json:"region"`
	AccessKey  string         `json:"access_key"`
	SecretKey  string         `json:"secret_key"`
	BaseURL    string         `json:"base_url"`
	PathPrefix string         `json:"path_prefix"`
	IsDefault  bool           `json:"is_default"`
	Status     int8           `json:"status"`
	Options    models.JSONMap `json:"options"`
	Remark     string         `json:"remark"`
}

type StorageConfigService struct {
	ctx context.Context
}

func NewStorageConfigService() *StorageConfigService {
	return &StorageConfigService{}
}

func (s *StorageConfigService) WithContext(ctx context.Context) *StorageConfigService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *StorageConfigService) List(filters map[string]string, page, pageSize int) (request.PageResult[StorageConfig], error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 15
	}
	query := s.query().Table("storage_config")
	query = applyStringFilter(query, "name", filters["name"])
	query = equalFilter(query, "provider", filters["provider"])
	query = equalFilter(query, "driver", filters["driver"])
	if filters["status"] != "" {
		query = query.Where("status", filters["status"])
	}
	total, err := query.Count()
	if err != nil {
		return request.PageResult[StorageConfig]{}, err
	}
	rows := make([]StorageConfig, 0)
	err = query.OrderByDesc("is_default").OrderByDesc("id").Offset((page - 1) * pageSize).Limit(pageSize).Get(&rows)
	s.hideSecrets(rows)
	return request.PageResult[StorageConfig]{List: rows, Total: total}, err
}

func (s *StorageConfigService) ActiveDefault() (StorageConfig, error) {
	var config StorageConfig
	err := s.query().
		Table("storage_config").
		Where("is_default", true).
		Where("status", StorageConfigStatusEnabled).
		First(&config)
	if err != nil {
		if errors.Is(err, frameworkerrors.OrmRecordNotFound) {
			return defaultLocalStorageConfig(), nil
		}
		return StorageConfig{}, err
	}
	if config.ID == 0 {
		return defaultLocalStorageConfig(), nil
	}
	return config, nil
}

func (s *StorageConfigService) Create(input StorageConfigPayload, operatorID uint64) (StorageConfig, error) {
	config := input.StorageConfig()
	config.AuditColumns = models.AuditColumns{CreatedBy: operatorID, UpdatedBy: operatorID}
	if config.SecretKey != "" {
		config.SecretKeyRotatedAt = time.Now()
	}
	if err := validateStorageConfig(config); err != nil {
		return StorageConfig{}, err
	}
	err := s.orm().Transaction(func(tx contractsorm.Query) error {
		if config.IsDefault {
			if err := clearDefaultStorageConfig(tx, 0); err != nil {
				return err
			}
		}
		row := storageConfigScalar(config)
		if err := tx.Table("storage_config").Create(&row); err != nil {
			return err
		}
		config.ID = row.ID
		return updateStorageConfigOptions(tx, config.ID, config.Options)
	})
	if err != nil {
		return StorageConfig{}, err
	}
	config.SecretKey = ""
	return config, nil
}

func (s *StorageConfigService) Update(id uint64, input StorageConfigPayload, operatorID uint64) (StorageConfig, error) {
	existing, err := s.find(id)
	if err != nil {
		return StorageConfig{}, err
	}
	config := input.StorageConfig()
	config.ID = id
	config.AuditColumns = models.AuditColumns{UpdatedBy: operatorID}
	if config.SecretKey == "" {
		config.SecretKey = existing.SecretKey
		config.SecretKeyRotatedAt = existing.SecretKeyRotatedAt
	} else if config.SecretKey != existing.SecretKey {
		config.SecretKeyRotatedAt = time.Now()
	} else {
		config.SecretKeyRotatedAt = existing.SecretKeyRotatedAt
	}
	if err := validateStorageConfig(config); err != nil {
		return StorageConfig{}, err
	}
	if err := s.ensureReferencedBackendUnchanged(existing, config); err != nil {
		return StorageConfig{}, err
	}
	err = s.orm().Transaction(func(tx contractsorm.Query) error {
		if config.IsDefault {
			if err := clearDefaultStorageConfig(tx, id); err != nil {
				return err
			}
		}
		_, err := tx.Table("storage_config").Where("id", id).Update(map[string]any{
			"name": config.Name, "provider": config.Provider, "driver": config.Driver,
			"bucket": config.Bucket, "endpoint": config.Endpoint, "region": config.Region,
			"access_key": config.AccessKey, "secret_key": config.SecretKey, "secret_key_rotated_at": config.SecretKeyRotatedAt, "base_url": config.BaseURL,
			"path_prefix": config.PathPrefix, "is_default": config.IsDefault, "status": config.Status,
			"updated_by": operatorID, "updated_at": time.Now(), "remark": config.Remark,
		})
		if err != nil {
			return err
		}
		return updateStorageConfigOptions(tx, id, config.Options)
	})
	if err != nil {
		return StorageConfig{}, err
	}
	config.SecretKey = ""
	return config, nil
}

func (s *StorageConfigService) Delete(ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	if err := s.ensureConfigsUnused(ids); err != nil {
		return err
	}
	_, err := s.query().Table("storage_config").WhereIn("id", uint64Any(ids)).Delete()
	return err
}

func (s *StorageConfigService) query() contractsorm.Query {
	return s.orm().Query()
}

func (s *StorageConfigService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, PlatformConnection())
}

func (s *StorageConfigService) find(id uint64) (StorageConfig, error) {
	var config StorageConfig
	err := s.query().Table("storage_config").Where("id", id).First(&config)
	if err != nil {
		return StorageConfig{}, err
	}
	return config, nil
}

func (s *StorageConfigService) ensureReferencedBackendUnchanged(existing, next StorageConfig) error {
	if sameStorageBackend(existing, next) {
		return nil
	}
	used, err := s.configHasAttachments(existing.ID)
	if err != nil {
		return err
	}
	if used {
		return BusinessError{Message: "储存配置已被附件引用，不能修改后端连接信息"}
	}
	return nil
}

func (s *StorageConfigService) ensureConfigsUnused(ids []uint64) error {
	for _, id := range ids {
		used, err := s.configHasAttachments(id)
		if err != nil {
			return err
		}
		if used {
			return BusinessError{Message: "储存配置已被附件引用，不能删除"}
		}
	}
	return nil
}

func (s *StorageConfigService) configHasAttachments(id uint64) (bool, error) {
	if id == 0 {
		return false, nil
	}
	if has, err := s.storageConfigUsedInConnection(PlatformConnection(), id); err != nil || has {
		return has, err
	}

	tenants, err := s.tenantsForStorageScan()
	if err != nil {
		return false, err
	}
	for _, tenant := range tenants {
		RegisterTenantConnection(tenant)
		has, err := s.storageConfigUsedInConnection(TenantConnectionName(tenant), id)
		if err != nil || has {
			return has, err
		}
	}
	return false, nil
}

func (s *StorageConfigService) hideSecrets(rows []StorageConfig) {
	for i := range rows {
		rows[i].SecretKey = ""
	}
}

func (p StorageConfigPayload) StorageConfig() StorageConfig {
	status := p.Status
	if status == 0 {
		status = StorageConfigStatusEnabled
	}
	provider := normalizeStorageProvider(p.Provider)
	driver := strings.TrimSpace(p.Driver)
	if driver == "" {
		driver = defaultDriverForProvider(provider)
	}
	return StorageConfig{
		Name: strings.TrimSpace(p.Name), Provider: provider, Driver: driver,
		Bucket: strings.TrimSpace(p.Bucket), Endpoint: strings.TrimSpace(p.Endpoint),
		Region: strings.TrimSpace(p.Region), AccessKey: strings.TrimSpace(p.AccessKey),
		SecretKey: strings.TrimSpace(p.SecretKey), BaseURL: strings.TrimRight(strings.TrimSpace(p.BaseURL), "/"),
		PathPrefix: normalizeStoragePathPrefix(p.PathPrefix), IsDefault: p.IsDefault,
		Status: status, Options: mapOrEmpty(p.Options), Remark: strings.TrimSpace(p.Remark),
	}
}

func validateStorageConfig(config StorageConfig) error {
	if config.Name == "" {
		return BusinessError{Message: "配置名称不能为空"}
	}
	if _, ok := supportedStorageProviders[config.Provider]; !ok {
		return BusinessError{Message: "不支持的储存方式"}
	}
	if config.Driver != storageDriverLocal && config.Driver != storageDriverS3Compatible {
		return BusinessError{Message: "不支持的储存驱动"}
	}
	if config.Status != StorageConfigStatusEnabled && config.Status != 2 {
		return BusinessError{Message: "储存配置状态无效"}
	}
	if config.IsDefault && config.Status != StorageConfigStatusEnabled {
		return BusinessError{Message: "默认储存配置必须启用"}
	}
	if config.Driver == storageDriverS3Compatible {
		if config.Bucket == "" {
			return BusinessError{Message: "Bucket 不能为空"}
		}
		if config.Endpoint == "" {
			return BusinessError{Message: "Endpoint 不能为空"}
		}
		if config.AccessKey == "" || config.SecretKey == "" {
			return BusinessError{Message: "Access Key 和 Secret Key 不能为空"}
		}
	}
	return nil
}

func defaultDriverForProvider(provider string) string {
	if provider == "local" {
		return storageDriverLocal
	}
	return storageDriverS3Compatible
}

func defaultLocalStorageConfig() StorageConfig {
	return StorageConfig{
		Name: "本地储存", Provider: "local", Driver: storageDriverLocal,
		PathPrefix: "uploads", IsDefault: true, Status: StorageConfigStatusEnabled,
	}
}

func storageConfigScalar(config StorageConfig) StorageConfig {
	config.Options = nil
	return config
}

func clearDefaultStorageConfig(tx contractsorm.Query, exceptID uint64) error {
	query := tx.Table("storage_config").Where("is_default", true)
	if exceptID > 0 {
		query = query.Where("id <> ?", exceptID)
	}
	_, err := query.Update(map[string]any{"is_default": false, "updated_at": time.Now()})
	return err
}

func updateStorageConfigOptions(query contractsorm.Query, id uint64, options models.JSONMap) error {
	encoded, err := json.Marshal(nullIfEmpty(options))
	if err != nil {
		return err
	}
	_, err = query.Exec("UPDATE storage_config SET options = ?::jsonb WHERE id = ?", string(encoded), id)
	return err
}

func sameStorageBackend(a, b StorageConfig) bool {
	return a.Provider == b.Provider &&
		a.Driver == b.Driver &&
		a.Bucket == b.Bucket &&
		a.Endpoint == b.Endpoint &&
		a.Region == b.Region &&
		a.AccessKey == b.AccessKey &&
		a.SecretKey == b.SecretKey &&
		a.BaseURL == b.BaseURL &&
		normalizeStoragePathPrefix(a.PathPrefix) == normalizeStoragePathPrefix(b.PathPrefix)
}

func (s *StorageConfigService) storageConfigUsedInConnection(connection string, id uint64) (bool, error) {
	schema := facades.Schema()
	previous := schema.GetConnection()
	schema.SetConnection(connection)
	defer schema.SetConnection(previous)
	if !schema.HasTable("attachment") || !schema.HasColumn("attachment", "storage_config_id") {
		return false, nil
	}

	count, err := OrmForConnectionWithContext(s.ctx, connection).
		Query().
		Table("attachment").
		Where("storage_config_id", id).
		Count()
	return count > 0, err
}

func (s *StorageConfigService) tenantsForStorageScan() ([]Tenant, error) {
	tenants := make([]Tenant, 0)
	err := s.orm().
		Query().
		Table("tenant").
		Get(&tenants)
	return tenants, err
}

func normalizeStorageProvider(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func normalizeStoragePathPrefix(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return "uploads"
	}
	parts := strings.Split(value, "/")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		part = normalizedTenantCode(part)
		if part != "" {
			kept = append(kept, part)
		}
	}
	if len(kept) == 0 {
		return "uploads"
	}
	return strings.Join(kept, "/")
}
