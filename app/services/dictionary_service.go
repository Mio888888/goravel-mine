package services

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"github.com/jackc/pgx/v5/pgconn"

	"goravel/app/http/request"
	"goravel/app/models"
)

const (
	DictStatusEnabled  int8 = 1
	DictStatusDisabled int8 = 2
)

type PlatformDictType = models.PlatformDictType
type PlatformDictItem = models.PlatformDictItem
type DictType = models.DictType
type DictItem = models.DictItem

type PlatformDictTypePayload struct {
	Code     string            `json:"code"`
	Name     string            `json:"name"`
	Status   int8              `json:"status"`
	Sort     int               `json:"sort"`
	Remark   string            `json:"remark"`
	IsSystem *bool             `json:"is_system"`
	Items    []DictItemPayload `json:"items"`
}

type DictItemPayload struct {
	ID     uint64 `json:"id"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	I18n   string `json:"i18n"`
	Color  string `json:"color"`
	Status int8   `json:"status"`
	Sort   int    `json:"sort"`
	Remark string `json:"remark"`
}

type TenantDictTypePayload struct {
	Name   string `json:"name"`
	Status int8   `json:"status"`
	Sort   int    `json:"sort"`
	Remark string `json:"remark"`
}

type TenantDictItemPayload struct {
	Label  string `json:"label"`
	I18n   string `json:"i18n"`
	Color  string `json:"color"`
	Status int8   `json:"status"`
	Sort   int    `json:"sort"`
	Remark string `json:"remark"`
}

type DictOption struct {
	Label    string `json:"label"`
	Value    any    `json:"value"`
	I18n     string `json:"i18n"`
	Color    string `json:"color"`
	Disabled bool   `json:"disabled"`
}

type TenantDictionaryService struct {
	ctx        context.Context
	connection string
	tenant     Tenant
}

type PlatformDictionaryService struct {
	ctx context.Context
}

func NewPlatformDictionaryService() *PlatformDictionaryService {
	return &PlatformDictionaryService{}
}

func NewTenantDictionaryServiceForTenant(tenant Tenant) *TenantDictionaryService {
	return &TenantDictionaryService{connection: TenantConnectionName(tenant), tenant: tenant}
}

func (s *TenantDictionaryService) WithContext(ctx context.Context) *TenantDictionaryService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *PlatformDictionaryService) WithContext(ctx context.Context) *PlatformDictionaryService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *TenantDictionaryService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, s.connection)
}

func (s *PlatformDictionaryService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, PlatformConnection())
}

func (p PlatformDictTypePayload) dictType(operatorID uint64) PlatformDictType {
	status := p.Status
	if status == 0 {
		status = DictStatusEnabled
	}
	isSystem := true
	if p.IsSystem != nil {
		isSystem = *p.IsSystem
	}
	return PlatformDictType{
		Code:         strings.TrimSpace(p.Code),
		Name:         strings.TrimSpace(p.Name),
		Status:       status,
		Sort:         p.Sort,
		Version:      1,
		IsSystem:     isSystem,
		AuditColumns: models.AuditColumns{CreatedBy: operatorID, UpdatedBy: operatorID},
		Remark:       strings.TrimSpace(p.Remark),
	}
}

func (p DictItemPayload) platformItem(typeID uint64, typeCode string, operatorID uint64) PlatformDictItem {
	status := p.Status
	if status == 0 {
		status = DictStatusEnabled
	}
	return PlatformDictItem{
		TypeID:       typeID,
		TypeCode:     typeCode,
		Label:        strings.TrimSpace(p.Label),
		Value:        strings.TrimSpace(p.Value),
		I18n:         strings.TrimSpace(p.I18n),
		Color:        strings.TrimSpace(p.Color),
		Status:       status,
		Sort:         p.Sort,
		Version:      1,
		IsSystem:     true,
		AuditColumns: models.AuditColumns{CreatedBy: operatorID, UpdatedBy: operatorID},
		Remark:       strings.TrimSpace(p.Remark),
	}
}

func (s *PlatformDictionaryService) List(filters map[string]string, page, pageSize int) (request.PageResult[PlatformDictType], error) {
	query := s.orm().Query().Table("platform_dict_type")
	query = applyStringFilter(query, "code", filters["code"])
	query = applyStringFilter(query, "name", filters["name"])
	if filters["status"] != "" {
		query = query.Where("status", filters["status"])
	}
	result, err := request.Paginate[PlatformDictType](query.OrderBy("sort").OrderByDesc("id"), page, pageSize)
	if err != nil {
		return request.PageResult[PlatformDictType]{}, err
	}
	if err := s.fillPlatformItems(result.List); err != nil {
		return request.PageResult[PlatformDictType]{}, err
	}
	return result, nil
}

func (s *PlatformDictionaryService) Detail(id uint64) (PlatformDictType, error) {
	var dictType PlatformDictType
	if err := s.orm().Query().Table("platform_dict_type").Where("id", id).First(&dictType); err != nil {
		return PlatformDictType{}, err
	}
	return s.withPlatformItems(dictType)
}

func (s *PlatformDictionaryService) Options(code string) ([]DictOption, error) {
	items, err := s.platformItemsByCode(code, true)
	if err != nil {
		return nil, err
	}
	options := make([]DictOption, 0, len(items))
	for _, item := range items {
		options = append(options, DictOption{
			Label:    item.Label,
			Value:    normalizeDictOptionValue(code, item.Value),
			I18n:     item.I18n,
			Color:    item.Color,
			Disabled: item.Status != DictStatusEnabled,
		})
	}
	return options, nil
}

func (s *PlatformDictionaryService) AllOptions() (map[string][]DictOption, error) {
	types := make([]PlatformDictType, 0)
	if err := s.orm().Query().Table("platform_dict_type").OrderBy("sort").OrderBy("id").Get(&types); err != nil {
		return nil, err
	}
	result := make(map[string][]DictOption, len(types))
	for _, dictType := range types {
		options, err := s.Options(dictType.Code)
		if err != nil {
			return nil, err
		}
		result[dictType.Code] = options
	}
	return result, nil
}

func (s *PlatformDictionaryService) Create(input PlatformDictTypePayload, operatorID uint64) (PlatformDictType, error) {
	dictType := input.dictType(operatorID)
	if err := validatePlatformDictType(dictType, input.Items); err != nil {
		return PlatformDictType{}, err
	}
	err := s.orm().Transaction(func(tx contractsorm.Query) error {
		if err := tx.Table("platform_dict_type").Create(&dictType); err != nil {
			return err
		}
		return s.replacePlatformItems(tx, dictType.ID, dictType.Code, input.Items, operatorID)
	})
	if err != nil {
		return PlatformDictType{}, err
	}
	return s.Detail(dictType.ID)
}

func (s *PlatformDictionaryService) Update(id uint64, input PlatformDictTypePayload, operatorID uint64) (PlatformDictType, error) {
	old, err := s.Detail(id)
	if err != nil {
		return PlatformDictType{}, err
	}
	dictType := input.dictType(operatorID)
	if err := validatePlatformDictType(dictType, input.Items); err != nil {
		return PlatformDictType{}, err
	}
	if err := validatePlatformIdentityUpdate(old, dictType, input.Items); err != nil {
		return PlatformDictType{}, err
	}
	err = s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("platform_dict_type").Where("id", id).Update(map[string]any{
			"code":       dictType.Code,
			"name":       dictType.Name,
			"status":     dictType.Status,
			"sort":       dictType.Sort,
			"version":    old.Version + 1,
			"is_system":  dictType.IsSystem,
			"updated_by": operatorID,
			"updated_at": time.Now(),
			"remark":     dictType.Remark,
		})
		if err != nil {
			return err
		}
		if old.Code != dictType.Code {
			if err := s.renamePlatformItemTypeCode(tx, old.Code, dictType.Code); err != nil {
				return err
			}
		}
		return s.upsertPlatformItems(tx, id, dictType.Code, input.Items, operatorID)
	})
	if err != nil {
		return PlatformDictType{}, err
	}
	return s.Detail(id)
}

func (s *PlatformDictionaryService) Delete(ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	count, err := s.distributedTypeCount(ids)
	if err != nil {
		return err
	}
	if count > 0 {
		return BusinessError{Message: "默认字典已分发到租户，不能删除，请改为禁用"}
	}
	return s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("platform_dict_item").WhereIn("type_id", uint64Any(ids)).Delete()
		if err != nil {
			return err
		}
		_, err = tx.Table("platform_dict_type").WhereIn("id", uint64Any(ids)).Delete()
		return err
	})
}

func (s *PlatformDictionaryService) DispatchAllTenants() error {
	tenants := make([]Tenant, 0)
	if err := s.orm().Query().Table("tenant").Where("status", TenantStatusActive).Get(&tenants); err != nil {
		return err
	}
	for _, tenant := range tenants {
		RegisterTenantConnection(tenant)
		if err := s.DispatchToTenant(tenant); err != nil {
			return err
		}
	}
	return nil
}

func (s *PlatformDictionaryService) DispatchToTenant(tenant Tenant) error {
	if tenant.ID == 0 {
		return ErrTenantNotFound
	}
	return NewTenantDictionaryServiceForTenant(tenant).SyncFromPlatform()
}

func (s *PlatformDictionaryService) fillPlatformItems(types []PlatformDictType) error {
	for index := range types {
		value, err := s.withPlatformItems(types[index])
		if err != nil {
			return err
		}
		types[index] = value
	}
	return nil
}

func (s *PlatformDictionaryService) withPlatformItems(dictType PlatformDictType) (PlatformDictType, error) {
	items := make([]PlatformDictItem, 0)
	err := s.orm().Query().
		Table("platform_dict_item").
		Where("type_id", dictType.ID).
		OrderBy("sort").
		OrderBy("id").
		Get(&items)
	dictType.Items = items
	return dictType, err
}

func (s *PlatformDictionaryService) platformItemsByCode(code string, enabledOnly bool) ([]PlatformDictItem, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return []PlatformDictItem{}, nil
	}
	query := s.orm().Query().Table("platform_dict_item").Where("type_code", code)
	if enabledOnly {
		query = query.Where("status", DictStatusEnabled)
	}
	items := make([]PlatformDictItem, 0)
	err := query.OrderBy("sort").OrderBy("id").Get(&items)
	return items, err
}

func (s *PlatformDictionaryService) replacePlatformItems(tx contractsorm.Query, typeID uint64, typeCode string, items []DictItemPayload, operatorID uint64) error {
	_, err := tx.Table("platform_dict_item").Where("type_id", typeID).Delete()
	if err != nil {
		return err
	}
	for _, input := range items {
		item := input.platformItem(typeID, typeCode, operatorID)
		if err := validateDictItem(item.Label, item.Value, item.Status); err != nil {
			return err
		}
		if err := tx.Table("platform_dict_item").Create(&item); err != nil {
			return err
		}
	}
	return nil
}

func (s *PlatformDictionaryService) upsertPlatformItems(tx contractsorm.Query, typeID uint64, typeCode string, items []DictItemPayload, operatorID uint64) error {
	keptIDs := make([]uint64, 0, len(items))
	for _, input := range items {
		item := input.platformItem(typeID, typeCode, operatorID)
		if err := validateDictItem(item.Label, item.Value, item.Status); err != nil {
			return err
		}
		if input.ID == 0 {
			if err := tx.Table("platform_dict_item").Create(&item); err != nil {
				return err
			}
			keptIDs = append(keptIDs, item.ID)
			continue
		}
		keptIDs = append(keptIDs, input.ID)
		_, err := tx.Table("platform_dict_item").
			Where("id", input.ID).
			Where("type_id", typeID).
			Update(map[string]any{
				"type_code":  typeCode,
				"label":      item.Label,
				"value":      item.Value,
				"i18n":       item.I18n,
				"color":      item.Color,
				"status":     item.Status,
				"sort":       item.Sort,
				"version":    item.Version + 1,
				"is_system":  item.IsSystem,
				"updated_by": operatorID,
				"updated_at": time.Now(),
				"remark":     item.Remark,
			})
		if err != nil {
			return err
		}
	}
	return s.disableRemovedPlatformItems(tx, typeID, keptIDs)
}

func (s *PlatformDictionaryService) disableRemovedPlatformItems(tx contractsorm.Query, typeID uint64, keptIDs []uint64) error {
	query := tx.Table("platform_dict_item").Where("type_id", typeID)
	if len(keptIDs) > 0 {
		query = query.WhereNotIn("id", uint64Any(keptIDs))
	}
	_, err := query.Update(map[string]any{
		"status":     DictStatusDisabled,
		"updated_at": time.Now(),
	})
	return err
}

func (s *PlatformDictionaryService) renamePlatformItemTypeCode(tx contractsorm.Query, oldCode, newCode string) error {
	_, err := tx.Table("platform_dict_item").Where("type_code", oldCode).Update(map[string]any{
		"type_code":  newCode,
		"updated_at": time.Now(),
	})
	return err
}

func (s *PlatformDictionaryService) distributedTypeCount(ids []uint64) (int64, error) {
	tenants := make([]Tenant, 0)
	if err := s.orm().Query().Table("tenant").Get(&tenants); err != nil {
		return 0, err
	}
	var total int64
	for _, tenant := range tenants {
		RegisterTenantConnection(tenant)
		count, err := OrmForConnectionWithContext(s.ctx, TenantConnectionName(tenant)).
			Query().
			Table("dict_type").
			WhereIn("source_id", uint64Any(ids)).
			Count()
		if err != nil {
			if isUndefinedTableError(err) {
				continue
			}
			return 0, err
		}
		total += count
	}
	return total, nil
}

func (s *TenantDictionaryService) List(filters map[string]string, page, pageSize int) (request.PageResult[DictType], error) {
	query := s.orm().Query().Table("dict_type")
	query = applyStringFilter(query, "code", filters["code"])
	query = applyStringFilter(query, "name", filters["name"])
	if filters["status"] != "" {
		query = query.Where("status", filters["status"])
	}
	return request.Paginate[DictType](query.OrderBy("sort").OrderByDesc("id"), page, pageSize)
}

func (s *TenantDictionaryService) Detail(id uint64) (DictType, error) {
	var dictType DictType
	if err := s.orm().Query().Table("dict_type").Where("id", id).First(&dictType); err != nil {
		return DictType{}, err
	}
	return dictType, nil
}

func (s *TenantDictionaryService) Items(typeID uint64) ([]DictItem, error) {
	var dictType DictType
	if err := s.orm().Query().Table("dict_type").Where("id", typeID).First(&dictType); err != nil {
		return nil, err
	}
	return s.itemsByCode(dictType.Code, false)
}

func (s *TenantDictionaryService) Options(code string) ([]DictOption, error) {
	items, err := s.itemsByCode(code, true)
	if err != nil {
		return nil, err
	}
	options := make([]DictOption, 0, len(items))
	for _, item := range items {
		options = append(options, DictOption{
			Label:    item.Label,
			Value:    normalizeDictOptionValue(code, item.Value),
			I18n:     item.I18n,
			Color:    item.Color,
			Disabled: item.Status != DictStatusEnabled,
		})
	}
	return options, nil
}

func (s *TenantDictionaryService) AllOptions() (map[string][]DictOption, error) {
	types := make([]DictType, 0)
	if err := s.orm().Query().Table("dict_type").OrderBy("sort").OrderBy("id").Get(&types); err != nil {
		return nil, err
	}
	result := make(map[string][]DictOption, len(types))
	for _, dictType := range types {
		options, err := s.Options(dictType.Code)
		if err != nil {
			return nil, err
		}
		result[dictType.Code] = options
	}
	return result, nil
}

func (s *TenantDictionaryService) UpdateType(id uint64, input TenantDictTypePayload, operatorID uint64) (DictType, error) {
	status := input.Status
	if status == 0 {
		status = DictStatusEnabled
	}
	if !validDictStatus(status) {
		return DictType{}, BusinessError{Message: "字典状态无效"}
	}
	_, err := s.orm().Query().Table("dict_type").Where("id", id).Update(map[string]any{
		"name":       strings.TrimSpace(input.Name),
		"status":     status,
		"sort":       input.Sort,
		"remark":     strings.TrimSpace(input.Remark),
		"updated_by": operatorID,
		"updated_at": time.Now(),
	})
	if err != nil {
		return DictType{}, err
	}
	return s.Detail(id)
}

func (s *TenantDictionaryService) UpdateItem(id uint64, input TenantDictItemPayload, operatorID uint64) (DictItem, error) {
	status := input.Status
	if status == 0 {
		status = DictStatusEnabled
	}
	if strings.TrimSpace(input.Label) == "" {
		return DictItem{}, BusinessError{Message: "字典项名称不能为空"}
	}
	if !validDictStatus(status) {
		return DictItem{}, BusinessError{Message: "字典项状态无效"}
	}
	_, err := s.orm().Query().Table("dict_item").Where("id", id).Update(map[string]any{
		"label":      strings.TrimSpace(input.Label),
		"i18n":       strings.TrimSpace(input.I18n),
		"color":      strings.TrimSpace(input.Color),
		"status":     status,
		"sort":       input.Sort,
		"remark":     strings.TrimSpace(input.Remark),
		"updated_by": operatorID,
		"updated_at": time.Now(),
	})
	if err != nil {
		return DictItem{}, err
	}
	var item DictItem
	err = s.orm().Query().Table("dict_item").Where("id", id).First(&item)
	return item, err
}

func (s *TenantDictionaryService) SyncFromPlatform() error {
	platform := NewPlatformDictionaryService()
	types := make([]PlatformDictType, 0)
	if err := platform.orm().Query().Table("platform_dict_type").OrderBy("sort").OrderBy("id").Get(&types); err != nil {
		return err
	}
	for _, platformType := range types {
		if err := s.syncTypeFromPlatform(platformType); err != nil {
			return err
		}
	}
	return nil
}

func (s *TenantDictionaryService) syncTypeFromPlatform(platformType PlatformDictType) error {
	items := make([]PlatformDictItem, 0)
	if err := NewPlatformDictionaryService().orm().Query().
		Table("platform_dict_item").
		Where("type_id", platformType.ID).
		OrderBy("sort").
		OrderBy("id").
		Get(&items); err != nil {
		return err
	}

	return s.orm().Transaction(func(tx contractsorm.Query) error {
		var tenantType DictType
		err := tx.Table("dict_type").Where("source_id", platformType.ID).First(&tenantType)
		if err != nil || tenantType.ID == 0 {
			tenantType = DictType{
				SourceID:     platformType.ID,
				SourceCode:   platformType.Code,
				Code:         platformType.Code,
				Name:         platformType.Name,
				Status:       platformType.Status,
				Sort:         platformType.Sort,
				Version:      platformType.Version,
				IsSystem:     true,
				AuditColumns: models.AuditColumns{},
				Remark:       platformType.Remark,
			}
			if err := tx.Table("dict_type").Create(&tenantType); err != nil {
				return err
			}
		} else {
			if err := s.syncTypeIdentity(tx, tenantType, platformType); err != nil {
				return err
			}
		}
		return s.syncItemsFromPlatform(tx, tenantType, items)
	})
}

func (s *TenantDictionaryService) syncTypeIdentity(tx contractsorm.Query, tenantType DictType, platformType PlatformDictType) error {
	values := map[string]any{
		"source_code": platformType.Code,
		"code":        platformType.Code,
		"version":     platformType.Version,
		"is_system":   true,
		"updated_at":  time.Now(),
	}
	if tenantType.Status == 0 {
		values["status"] = platformType.Status
	}
	if strings.TrimSpace(tenantType.Name) == "" {
		values["name"] = platformType.Name
	}
	_, err := tx.Table("dict_type").Where("id", tenantType.ID).Update(values)
	return err
}

func (s *TenantDictionaryService) syncItemsFromPlatform(tx contractsorm.Query, tenantType DictType, items []PlatformDictItem) error {
	for _, platformItem := range items {
		var tenantItem DictItem
		err := tx.Table("dict_item").Where("source_id", platformItem.ID).First(&tenantItem)
		if err != nil || tenantItem.ID == 0 {
			item := DictItem{
				TypeID:       tenantType.ID,
				SourceID:     platformItem.ID,
				SourceCode:   platformItem.TypeCode + ":" + platformItem.Value,
				TypeCode:     platformItem.TypeCode,
				Label:        platformItem.Label,
				Value:        platformItem.Value,
				I18n:         platformItem.I18n,
				Color:        platformItem.Color,
				Status:       platformItem.Status,
				Sort:         platformItem.Sort,
				Version:      platformItem.Version,
				IsSystem:     true,
				AuditColumns: models.AuditColumns{},
				Remark:       platformItem.Remark,
			}
			if err := tx.Table("dict_item").Create(&item); err != nil {
				return err
			}
			continue
		}
		if err := s.syncItemIdentity(tx, tenantItem, platformItem, tenantType.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *TenantDictionaryService) syncItemIdentity(tx contractsorm.Query, tenantItem DictItem, platformItem PlatformDictItem, typeID uint64) error {
	values := map[string]any{
		"type_id":     typeID,
		"source_code": platformItem.TypeCode + ":" + platformItem.Value,
		"type_code":   platformItem.TypeCode,
		"value":       platformItem.Value,
		"version":     platformItem.Version,
		"is_system":   true,
		"updated_at":  time.Now(),
	}
	if tenantItem.Status == 0 {
		values["status"] = platformItem.Status
	}
	_, err := tx.Table("dict_item").Where("id", tenantItem.ID).Update(values)
	return err
}

func (s *TenantDictionaryService) itemsByCode(code string, enabledOnly bool) ([]DictItem, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return []DictItem{}, nil
	}
	query := s.orm().Query().Table("dict_item").Where("type_code", code)
	if enabledOnly {
		query = query.Where("status", DictStatusEnabled)
	}
	items := make([]DictItem, 0)
	err := query.OrderBy("sort").OrderBy("id").Get(&items)
	return items, err
}

func validatePlatformDictType(dictType PlatformDictType, items []DictItemPayload) error {
	if dictType.Code == "" || dictType.Name == "" {
		return BusinessError{Message: "字典编码和名称不能为空"}
	}
	if !validDictStatus(dictType.Status) {
		return BusinessError{Message: "字典状态无效"}
	}
	seen := map[string]bool{}
	for _, item := range items {
		value := strings.TrimSpace(item.Value)
		if seen[value] {
			return BusinessError{Message: "字典项值不能重复"}
		}
		seen[value] = true
		if err := validateDictItem(item.Label, item.Value, item.Status); err != nil {
			return err
		}
	}
	return nil
}

func validatePlatformIdentityUpdate(old PlatformDictType, next PlatformDictType, items []DictItemPayload) error {
	if old.Code != next.Code {
		return BusinessError{Message: "字典编码为系统标识，不能修改"}
	}
	oldItems := map[uint64]PlatformDictItem{}
	for _, item := range old.Items {
		oldItems[item.ID] = item
	}
	for _, item := range items {
		if item.ID == 0 {
			continue
		}
		oldItem, ok := oldItems[item.ID]
		if !ok {
			return BusinessError{Message: "字典项不存在"}
		}
		if oldItem.Value != strings.TrimSpace(item.Value) {
			return BusinessError{Message: "字典项值为系统标识，不能修改"}
		}
	}
	return nil
}

func validateDictItem(label, value string, status int8) error {
	if strings.TrimSpace(label) == "" || strings.TrimSpace(value) == "" {
		return BusinessError{Message: "字典项名称和值不能为空"}
	}
	if status == 0 {
		status = DictStatusEnabled
	}
	if !validDictStatus(status) {
		return BusinessError{Message: "字典项状态无效"}
	}
	return nil
}

func validDictStatus(status int8) bool {
	return status == DictStatusEnabled || status == DictStatusDisabled
}

func normalizeDictOptionValue(code, value string) any {
	switch code {
	case "system-status", "system-state", "base-userType", "tenant-status":
		number, err := strconv.Atoi(value)
		if err == nil {
			return number
		}
	case "system-enabled":
		return value == "true"
	}
	return value
}

func isUndefinedTableError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42P01"
}
