package services

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"goravel/app/models"
)

type TenantExportArtifactStore interface {
	WriteImmutable(context.Context, models.TenantGovernanceRun, []byte) (TenantIsolationArtifact, error)
}

type s3TenantExportArtifactStore struct{ config StorageConfig }

func configuredTenantExportArtifactStore(ctx context.Context) TenantExportArtifactStore {
	config, err := NewStorageConfigService().WithContext(ctx).ActiveDefault()
	if err != nil || config.Driver != storageDriverS3Compatible || strings.TrimSpace(config.Bucket) == "" {
		return nil
	}
	return &s3TenantExportArtifactStore{config: config}
}

func (s *s3TenantExportArtifactStore) WriteImmutable(ctx context.Context, run models.TenantGovernanceRun, payload []byte) (TenantIsolationArtifact, error) {
	if s == nil || run.ID == 0 || run.TenantID == 0 || len(payload) == 0 {
		return TenantIsolationArtifact{}, ErrTenantExportInvalid
	}
	digest := digestBytes(payload)
	objectPath := path.Join(s.config.PathPrefix, "tenant-governance/export", fmt.Sprint(run.TenantID), fmt.Sprintf("run-%d-%s.enc", run.ID, strings.TrimPrefix(digest, "sha256:")[:16]))
	client := newObjectStorageClient(s.config)
	writtenAt := time.Now().UTC()
	version, err := putImmutableS3Object(ctx, client, objectPath, payload, writtenAt.Add(48*time.Hour))
	if err != nil || version == "" || verifyImmutableS3Object(ctx, client, objectPath, version, payload, writtenAt) != nil {
		return TenantIsolationArtifact{}, ErrTenantExportInvalid
	}
	return TenantIsolationArtifact{URI: "s3://" + s.config.Bucket + "/" + objectPath, ObjectVersion: version, SHA256: digest}, nil
}
