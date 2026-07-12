package services

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestImmutableEvidenceReadLimitRejectsOversizedBody(t *testing.T) {
	_, err := readImmutableEvidenceBody(bytes.NewReader([]byte("123456789")), 8)
	require.ErrorIs(t, err, ErrImmutableEvidenceUnattested)
	payload, err := readImmutableEvidenceBody(bytes.NewReader([]byte("12345678")), 8)
	require.NoError(t, err)
	require.Equal(t, []byte("12345678"), payload)
}

func TestImmutableS3RequestPinsObjectVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		require.Equal(t, "version-7", request.URL.Query().Get("versionId"))
		response.Header().Set("X-Amz-Version-Id", "version-7")
		response.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	client := newObjectStorageClient(StorageConfig{Endpoint: server.URL, Bucket: "audit"})

	response, err := immutableS3Request(t.Context(), client, http.MethodHead, "archive/manifest.json", "version-7")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
}

func TestImmutableEvidenceVerifierRejectsVersionMismatch(t *testing.T) {
	now := time.Now().UTC()
	verifier := &ImmutableEvidenceVerifier{
		now: time.Now, maxFreshness: time.Hour,
		attestor: immutableEvidenceAttestorFunc(func(ImmutableEvidence) (ImmutableEvidenceAttestation, error) {
			return ImmutableEvidenceAttestation{
				Manifest: []byte(`{}`), ObjectVersion: "v1",
				ImmutableUntil: now.Add(time.Hour), VerifiedAt: now,
			}, nil
		}),
	}

	_, err := verifier.Verify(t.Context(), ImmutableEvidence{
		URI: "s3://audit/archive.json", ObjectVersion: "v2", SHA256: auditPruneTestDigest("manifest"),
	})
	require.ErrorIs(t, err, ErrImmutableEvidenceInvalid)
}
