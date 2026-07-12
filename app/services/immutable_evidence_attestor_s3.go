package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"goravel/app/facades"
)

type s3ImmutableEvidenceAttestor struct{}

const defaultImmutableEvidenceMaxBytes int64 = 64 << 20

func NewS3ImmutableEvidenceAttestor() ImmutableEvidenceAttestor {
	return &s3ImmutableEvidenceAttestor{}
}

func (a *s3ImmutableEvidenceAttestor) Attest(ctx context.Context, evidence ImmutableEvidence) (ImmutableEvidenceAttestation, error) {
	storage, objectPath, err := immutableS3Object(evidence.URI)
	if err != nil {
		return ImmutableEvidenceAttestation{}, err
	}
	config, err := NewStorageConfigService().WithContext(ctx).ActiveDefault()
	if err != nil || config.Driver != storageDriverS3Compatible || config.Bucket != storage {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	client := newObjectStorageClient(config)
	maximum := int64(facades.Config().GetInt("security.audit.immutable_evidence_max_bytes", int(defaultImmutableEvidenceMaxBytes)))
	if maximum <= 0 {
		maximum = defaultImmutableEvidenceMaxBytes
	}
	metadata, err := immutableS3Request(ctx, client, http.MethodHead, objectPath, evidence.ObjectVersion)
	if err != nil {
		return ImmutableEvidenceAttestation{}, err
	}
	defer func() { _ = metadata.Body.Close() }()
	if metadata.ContentLength > maximum {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	if !strings.EqualFold(metadata.Header.Get("X-Amz-Object-Lock-Mode"), "COMPLIANCE") {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	immutableUntil, err := time.Parse(time.RFC3339, metadata.Header.Get("X-Amz-Object-Lock-Retain-Until-Date"))
	if err != nil || metadata.Header.Get("X-Amz-Version-Id") != evidence.ObjectVersion {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	object, err := immutableS3Request(ctx, client, http.MethodGet, objectPath, evidence.ObjectVersion)
	if err != nil {
		return ImmutableEvidenceAttestation{}, err
	}
	defer func() { _ = object.Body.Close() }()
	if object.ContentLength > maximum {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	manifest, err := readImmutableEvidenceBody(object.Body, maximum)
	if err != nil {
		return ImmutableEvidenceAttestation{}, ErrImmutableEvidenceUnattested
	}
	return ImmutableEvidenceAttestation{
		Manifest: manifest, ObjectVersion: evidence.ObjectVersion,
		ImmutableUntil: immutableUntil.UTC(), VerifiedAt: time.Now().UTC(),
	}, nil
}

func readImmutableEvidenceBody(body io.Reader, maximum int64) ([]byte, error) {
	if maximum <= 0 {
		return nil, ErrImmutableEvidenceUnattested
	}
	payload, err := io.ReadAll(io.LimitReader(body, maximum+1))
	if err != nil || len(payload) == 0 || int64(len(payload)) > maximum {
		return nil, ErrImmutableEvidenceUnattested
	}
	return payload, nil
}

func immutableS3Object(raw string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "s3" || parsed.Host == "" || strings.Trim(parsed.Path, "/") == "" {
		return "", "", ErrImmutableEvidenceInvalid
	}
	return parsed.Host, strings.TrimLeft(parsed.Path, "/"), nil
}

func immutableS3Request(ctx context.Context, client *objectStorageClient, method, objectPath, version string) (*http.Response, error) {
	endpoint, err := client.objectURL(objectPath)
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	query := parsed.Query()
	query.Set("versionId", version)
	parsed.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	client.sign(req, nil)
	response, err := client.client.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer func() { _ = response.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return nil, fmt.Errorf("immutable object request failed: %s %s", response.Status, strings.TrimSpace(string(body)))
	}
	return response, nil
}
