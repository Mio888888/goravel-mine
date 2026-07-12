package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

const tenantIsolationObjectRetention = 48 * time.Hour

type s3TenantIsolationArtifactStore struct {
	config StorageConfig
	now    func() time.Time
}

func configuredTenantIsolationArtifactStore(ctx context.Context) TenantIsolationArtifactStore {
	config, err := NewStorageConfigService().WithContext(ctx).ActiveDefault()
	if err != nil || config.Driver != storageDriverS3Compatible || strings.TrimSpace(config.Bucket) == "" {
		return nil
	}
	return &s3TenantIsolationArtifactStore{config: config, now: time.Now}
}

func (s *s3TenantIsolationArtifactStore) WriteImmutable(ctx context.Context, tenant Tenant, payload []byte) (TenantIsolationArtifact, error) {
	if s == nil || s.config.Driver != storageDriverS3Compatible || tenant.ID == 0 || len(payload) == 0 {
		return TenantIsolationArtifact{}, ErrTenantIsolationVerification
	}
	now := s.now
	if now == nil {
		now = time.Now
	}
	writtenAt := now().UTC()
	digest := digestBytes(payload)
	objectPath := path.Join(
		s.config.PathPrefix,
		"tenant-governance/isolation",
		fmt.Sprint(tenant.ID),
		fmt.Sprintf("%s-%s.json", writtenAt.Format("20060102T150405.000000000Z"), strings.TrimPrefix(digest, "sha256:")[:16]),
	)
	client := newObjectStorageClient(s.config)
	version, err := putImmutableS3Object(ctx, client, objectPath, payload, writtenAt.Add(tenantIsolationObjectRetention))
	if err != nil || strings.TrimSpace(version) == "" {
		return TenantIsolationArtifact{}, ErrTenantIsolationVerification
	}
	if err := verifyImmutableS3Object(ctx, client, objectPath, version, payload, writtenAt); err != nil {
		return TenantIsolationArtifact{}, ErrTenantIsolationVerification
	}
	return TenantIsolationArtifact{
		URI: "s3://" + s.config.Bucket + "/" + objectPath, ObjectVersion: version, SHA256: digest,
	}, nil
}

func putImmutableS3Object(ctx context.Context, client *objectStorageClient, objectPath string, payload []byte, retainUntil time.Time) (string, error) {
	endpoint, err := client.objectURL(objectPath)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Amz-Object-Lock-Mode", "COMPLIANCE")
	request.Header.Set("X-Amz-Object-Lock-Retain-Until-Date", retainUntil.UTC().Format(time.RFC3339))
	client.sign(request, payload)
	response, err := client.client.Do(request)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return "", fmt.Errorf("immutable object write failed: %s %s", response.Status, strings.TrimSpace(string(body)))
	}
	return strings.TrimSpace(response.Header.Get("X-Amz-Version-Id")), nil
}

func verifyImmutableS3Object(ctx context.Context, client *objectStorageClient, objectPath, version string, payload []byte, writtenAt time.Time) error {
	metadata, err := immutableS3Request(ctx, client, http.MethodHead, objectPath, version)
	if err != nil {
		return err
	}
	defer func() { _ = metadata.Body.Close() }()
	retainUntil, err := time.Parse(time.RFC3339, metadata.Header.Get("X-Amz-Object-Lock-Retain-Until-Date"))
	if err != nil || !strings.EqualFold(metadata.Header.Get("X-Amz-Object-Lock-Mode"), "COMPLIANCE") ||
		metadata.Header.Get("X-Amz-Version-Id") != version || !retainUntil.After(writtenAt) {
		return ErrTenantIsolationVerification
	}
	object, err := immutableS3Request(ctx, client, http.MethodGet, objectPath, version)
	if err != nil {
		return err
	}
	defer func() { _ = object.Body.Close() }()
	stored, err := io.ReadAll(io.LimitReader(object.Body, int64(len(payload)+1)))
	if err != nil || len(stored) != len(payload) || !sameDigest(digestBytes(stored), digestBytes(payload)) {
		return ErrTenantIsolationVerification
	}
	return nil
}
