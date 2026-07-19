package services

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	contractsfilesystem "github.com/goravel/framework/contracts/filesystem"
	frameworkerrors "github.com/goravel/framework/errors"
	"github.com/goravel/framework/support/path"
	"goravel/app/http/request"
	"goravel/app/models"
)

type AttachmentService struct {
	ctx        context.Context
	connection string
	tenant     Tenant
	storage    StorageConfig
}

var allowedAttachmentSuffixes = map[string]struct{}{
	"7z": {}, "bmp": {}, "csv": {}, "doc": {}, "docx": {}, "gif": {},
	"gz": {}, "ico": {}, "jpeg": {}, "jpg": {}, "json": {}, "md": {},
	"mov": {}, "mp3": {}, "mp4": {}, "pdf": {}, "png": {}, "ppt": {},
	"pptx": {}, "rar": {}, "tar": {}, "txt": {}, "wav": {}, "webm": {},
	"webp": {}, "xls": {}, "xlsx": {}, "zip": {},
}

var blockedAttachmentMIMEs = map[string]struct{}{
	"application/ecmascript": {},
	"application/javascript": {},
	"application/xhtml+xml":  {},
	"application/xml":        {},
	"image/svg+xml":          {},
	"text/ecmascript":        {},
	"text/html":              {},
	"text/javascript":        {},
	"text/xml":               {},
}

func NewAttachmentService() *AttachmentService {
	return &AttachmentService{}
}

func NewAttachmentServiceForTenant(tenant Tenant) *AttachmentService {
	return &AttachmentService{connection: TenantConnectionName(tenant), tenant: tenant}
}

func (s *AttachmentService) WithContext(ctx context.Context) *AttachmentService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *AttachmentService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, s.connection)
}

func (s *AttachmentService) Upload(file contractsfilesystem.File, userID uint64, originName, suffix string) (models.Attachment, error) {
	if s.tenant.ID != 0 {
		size, err := file.Size()
		if err != nil {
			return models.Attachment{}, err
		}
		if err := s.ensureStorageQuota(size); err != nil {
			return models.Attachment{}, err
		}
	}
	suffix, mimeType, err := attachmentType(file, suffix)
	if err != nil {
		return models.Attachment{}, err
	}
	hash, err := fileMD5(file.File())
	if err != nil {
		return models.Attachment{}, err
	}
	if existing, ok, err := s.findByHash(hash); err != nil || ok {
		return existing, err
	}

	attachment, err := s.buildAttachment(file, hash, userID, originName, suffix, mimeType)
	if err != nil {
		return models.Attachment{}, err
	}
	if err := s.storeUploadedFile(file.File(), attachment.StoragePath, attachment.MimeType); err != nil {
		return models.Attachment{}, err
	}
	if err := s.orm().Query().Create(&attachment); err != nil {
		return models.Attachment{}, err
	}
	return attachment, nil
}

func (s *AttachmentService) ensureStorageQuota(addBytes int64) error {
	limit := jsonInt64(NewTenantRuntimeService().WithContext(s.ctx).EffectiveQuotas(s.tenant), "max_storage_mb")
	if limit <= 0 {
		return nil
	}
	var usedBytes int64
	if err := s.orm().Query().Table("attachment").Sum("size_byte", &usedBytes); err != nil {
		return err
	}
	if bytesToMB(usedBytes+addBytes) > limit {
		return ErrQuotaExceeded
	}
	return nil
}

func (s *AttachmentService) List(filters map[string]string, page, pageSize int) (request.PageResult[models.Attachment], error) {
	query := attachmentFilters(s.orm().Query().Table("attachment"), filters)
	return request.Paginate[models.Attachment](query.OrderByDesc("id"), page, pageSize)
}

func (s *AttachmentService) Delete(id uint64) error {
	var attachment models.Attachment
	if err := s.orm().Query().Table("attachment").Where("id", id).First(&attachment); err != nil {
		if errors.Is(err, frameworkerrors.OrmRecordNotFound) {
			return nil
		}
		return err
	}
	if err := s.deleteStoredFile(attachment); err != nil {
		return err
	}
	_, err := s.orm().Query().Table("attachment").Where("id", id).Delete()
	return err
}

func (s *AttachmentService) buildAttachment(file contractsfilesystem.File, hash string, userID uint64, originName, suffix, mimeType string) (models.Attachment, error) {
	size, err := file.Size()
	if err != nil {
		return models.Attachment{}, err
	}
	if originName == "" {
		originName = file.GetClientOriginalName()
	}
	objectName := hash
	if suffix != "" {
		objectName += "." + suffix
	}
	storage, err := s.activeStorageConfig()
	if err != nil {
		return models.Attachment{}, err
	}
	storageDir := storage.PathPrefix + "/" + s.storageScope() + "/" + time.Now().Format("2006/01/02")
	storagePath := storageDir + "/" + objectName
	now := time.Now()

	return models.Attachment{
		StorageMode: storage.Provider, StorageConfigID: storage.ID,
		OriginName: originName, ObjectName: objectName, Hash: hash, MimeType: mimeType,
		StoragePath: storagePath, Suffix: suffix, SizeByte: size, SizeInfo: sizeInfo(size),
		URL:          s.storageURL(storage, storagePath),
		AuditColumns: models.AuditColumns{CreatedBy: userID, UpdatedBy: userID},
		Timestamps:   models.Timestamps{CreatedAt: now, UpdatedAt: now},
	}, nil
}

func (s *AttachmentService) storageScope() string {
	if s.tenant.ID == 0 && strings.TrimSpace(s.tenant.Code) == "" {
		return "platform"
	}
	return "tenants/" + normalizedTenantCode(s.tenant.Code)
}

func (s *AttachmentService) activeStorageConfig() (StorageConfig, error) {
	if s.storage.Provider != "" {
		return s.storage, nil
	}
	storage, err := NewStorageConfigService().WithContext(s.ctx).ActiveDefault()
	if err != nil {
		return StorageConfig{}, err
	}
	storage.PathPrefix = normalizeStoragePathPrefix(storage.PathPrefix)
	s.storage = storage
	return storage, nil
}

func (s *AttachmentService) storageURL(storage StorageConfig, storagePath string) string {
	if storage.Driver == storageDriverS3Compatible {
		return newObjectStorageClient(storage).PublicURL(storagePath)
	}
	if storage.BaseURL != "" {
		return strings.TrimRight(storage.BaseURL, "/") + "/" + storagePath
	}
	return "/storage/" + storagePath
}

func (s *AttachmentService) storeUploadedFile(source, storagePath, mimeType string) error {
	storage, err := s.activeStorageConfig()
	if err != nil {
		return err
	}
	if storage.Driver == storageDriverS3Compatible {
		return newObjectStorageClient(storage).Put(source, storagePath, mimeType)
	}
	return copyUploadedFile(source, path.Storage("app/public"), storagePath)
}

func (s *AttachmentService) deleteStoredFile(attachment models.Attachment) error {
	storage, err := s.storageForAttachment(attachment)
	if err != nil {
		return err
	}
	if storage.Driver == storageDriverS3Compatible {
		return newObjectStorageClient(storage).Delete(attachment.StoragePath)
	}
	return deleteLocalAttachmentFile(attachment.StoragePath)
}

func (s *AttachmentService) storageForAttachment(attachment models.Attachment) (StorageConfig, error) {
	if attachment.StorageConfigID > 0 {
		storage, err := NewStorageConfigService().WithContext(s.ctx).find(attachment.StorageConfigID)
		if err != nil {
			return StorageConfig{}, err
		}
		storage.PathPrefix = normalizeStoragePathPrefix(storage.PathPrefix)
		return storage, nil
	}
	if attachment.StorageMode == "local" {
		return defaultLocalStorageConfig(), nil
	}
	return s.activeStorageConfig()
}

func attachmentType(file contractsfilesystem.File, suffix string) (string, string, error) {
	mimeType, err := file.MimeType()
	if err != nil {
		return "", "", err
	}
	if suffix == "" {
		suffix = file.GetClientOriginalExtension()
	}
	suffix = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(suffix), "."))
	mimeType = normalizeMimeType(mimeType, suffix)
	if err := validateAttachmentType(suffix, mimeType); err != nil {
		return "", "", err
	}
	return suffix, mimeType, nil
}

func (s *AttachmentService) findByHash(hash string) (models.Attachment, bool, error) {
	var attachment models.Attachment
	storage, err := s.activeStorageConfig()
	if err != nil {
		return models.Attachment{}, false, err
	}
	scopePattern := storage.PathPrefix + "/" + s.storageScope() + "/%"
	err = s.orm().Query().
		Table("attachment").
		Where("hash", hash).
		Where("storage_mode", storage.Provider).
		Where("storage_config_id", storage.ID).
		Where("storage_path LIKE ?", scopePattern).
		First(&attachment)
	if err != nil {
		return models.Attachment{}, false, nil
	}
	if attachment.ID == 0 {
		return models.Attachment{}, false, nil
	}
	return attachment, true, nil
}

func attachmentFilters(query contractsorm.Query, filters map[string]string) contractsorm.Query {
	query = equalFilter(query, "origin_name", filters["origin_name"])
	query = equalFilter(query, "hash", filters["hash"])
	query = equalFilter(query, "object_name", filters["object_name"])
	query = equalFilter(query, "storage_path", filters["storage_path"])
	query = equalFilter(query, "storage_mode", filters["storage_mode"])
	query = equalFilter(query, "mime_type", filters["mime_type"])
	if suffixes := splitCSV(filters["suffix"]); len(suffixes) > 0 {
		query = query.WhereIn("suffix", stringAny(suffixes))
	}
	return query
}

func stringAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func fileMD5(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyUploadedFile(source, root, storagePath string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	target := filepath.Join(root, filepath.FromSlash(storagePath))
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

func deleteLocalAttachmentFile(storagePath string) error {
	if strings.TrimSpace(storagePath) == "" {
		return nil
	}
	root := filepath.Clean(path.Storage("app/public"))
	target := filepath.Clean(filepath.Join(root, filepath.FromSlash(storagePath)))
	if target == root || !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		return fmt.Errorf("invalid attachment storage path")
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func normalizeMimeType(value, suffix string) string {
	if value != "application/octet-stream" || suffix == "" {
		return value
	}
	if guessed := mime.TypeByExtension("." + suffix); guessed != "" {
		return guessed
	}
	return value
}

func validateAttachmentType(suffix, mimeType string) error {
	if _, ok := allowedAttachmentSuffixes[suffix]; !ok {
		return BusinessError{Message: "不支持的文件类型"}
	}
	if _, blocked := blockedAttachmentMIMEs[baseMimeType(mimeType)]; blocked {
		return BusinessError{Message: "不支持的文件类型"}
	}
	return nil
}

func baseMimeType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if index := strings.Index(value, ";"); index >= 0 {
		return strings.TrimSpace(value[:index])
	}
	return value
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func sizeInfo(size int64) string {
	if size < 1024 {
		return strconv.FormatInt(size, 10) + " B"
	}
	return fmt.Sprintf("%.2f KB", float64(size)/1024)
}
