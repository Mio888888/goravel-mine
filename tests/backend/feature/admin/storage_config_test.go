package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	contractshttp "github.com/goravel/framework/contracts/testing/http"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/services"
	"goravel/database/seeders"
	"goravel/tests/backend/testcase"
)

type StorageConfigTestSuite struct {
	suite.Suite
	tests.TestCase
	platformToken string
	tenantToken   string
}

func TestStorageConfigTestSuite(t *testing.T) {
	suite.Run(t, new(StorageConfigTestSuite))
}

func (s *StorageConfigTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.TenantPlanSeeder{})
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.MenuSeeder{})
	s.Seed(&seeders.DepartmentSeeder{})
	s.Seed(&seeders.CasbinSeeder{})
	s.Seed(&seeders.PlatformBootstrapSeeder{})
	s.platformToken = s.loginAsPlatformAdmin()
	s.tenantToken = s.loginAsTenantAdmin()
}

func (s *StorageConfigTestSuite) TestPlatformStorageConfigLifecycleAndProviderValidation() {
	create := s.postJSON("/admin/platform/storage-config", `{
		"name": "主 S3",
		"provider": "aws_s3",
		"driver": "s3_compatible",
		"bucket": "mine-prod",
		"endpoint": "https://s3.example.test",
		"region": "ap-east-1",
		"access_key": "ak",
		"secret_key": "sk",
		"base_url": "https://cdn.example.test",
		"path_prefix": "assets",
		"is_default": true,
		"status": 1,
		"remark": "primary"
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	data := create["data"].(map[string]any)
	require.Equal(s.T(), "aws_s3", data["provider"])
	require.Equal(s.T(), "s3_compatible", data["driver"])
	require.Equal(s.T(), true, data["is_default"])
	require.NotContains(s.T(), data, "secret_key")
	require.Contains(s.T(), data["path_prefix"], "assets")

	invalid := s.postJSON("/admin/platform/storage-config", `{
		"name": "非法",
		"provider": "ftp",
		"driver": "ftp",
		"bucket": "bucket",
		"status": 1
	}`)
	require.Equal(s.T(), float64(422), invalid["code"])

	list := s.getJSON("/admin/platform/storage-config/list?provider=aws_s3")
	require.Equal(s.T(), float64(200), list["code"])
	rows := list["data"].(map[string]any)["list"].([]any)
	require.Len(s.T(), rows, 1)
	require.Equal(s.T(), data["id"], rows[0].(map[string]any)["id"])
}

func (s *StorageConfigTestSuite) TestPlatformStorageConfigSensitiveMutationsRequireEvidence() {
	service := services.NewStorageConfigService().WithContext(s.T().Context())
	fixture, err := service.Create(services.StorageConfigPayload{
		Name: "guarded-local", Provider: "local", Driver: "local", PathPrefix: "uploads", Status: 1,
	}, 1)
	require.NoError(s.T(), err)

	before, err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("storage_config").Count()
	require.NoError(s.T(), err)
	create := s.rawPlatformMutation("POST", "/admin/platform/storage-config", `{
		"name":"guarded-s3","provider":"minio","driver":"s3_compatible","bucket":"guarded",
		"endpoint":"https://storage.example.test","access_key":"ak","secret_key":"sk","status":1
	}`)
	require.Equal(s.T(), float64(422), create["code"])
	after, err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("storage_config").Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), before, after)

	update := s.rawPlatformMutation("PUT", "/admin/platform/storage-config/"+itoa(fixture.ID), `{
		"name":"guarded-local","provider":"local","driver":"local","path_prefix":"uploads","is_default":true,"status":1
	}`)
	require.Equal(s.T(), float64(422), update["code"])
	deleted := s.rawPlatformMutation("DELETE", "/admin/platform/storage-config", `{"ids":[`+itoa(fixture.ID)+`]}`)
	require.Equal(s.T(), float64(422), deleted["code"])
	var existing struct {
		IsDefault bool `gorm:"column:is_default"`
	}
	require.NoError(s.T(), facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("storage_config").Where("id", fixture.ID).First(&existing))
	require.False(s.T(), existing.IsDefault)
}

func (s *StorageConfigTestSuite) TestDefaultStorageConfigIsUnique() {
	first := s.postJSON("/admin/platform/storage-config", `{
		"name": "默认 MinIO",
		"provider": "minio",
		"driver": "s3_compatible",
		"bucket": "one",
		"endpoint": "https://minio-one.example.test",
		"access_key": "ak",
		"secret_key": "sk",
		"is_default": true,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), first["code"])

	second := s.postJSON("/admin/platform/storage-config", `{
		"name": "默认 OSS",
		"provider": "aliyun_oss",
		"driver": "s3_compatible",
		"bucket": "two",
		"endpoint": "https://oss-two.example.test",
		"access_key": "ak",
		"secret_key": "sk",
		"is_default": true,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), second["code"])

	list := s.getJSON("/admin/platform/storage-config/list?status=1")
	rows := list["data"].(map[string]any)["list"].([]any)
	defaults := 0
	for _, row := range rows {
		if row.(map[string]any)["is_default"] == true {
			defaults++
		}
	}
	require.Equal(s.T(), 1, defaults)
}

func (s *StorageConfigTestSuite) TestDisabledStorageConfigCannotBeDefault() {
	create := s.postJSON("/admin/platform/storage-config", `{
		"name": "禁用默认",
		"provider": "local",
		"driver": "local",
		"path_prefix": "uploads",
		"is_default": true,
		"status": 2
	}`)
	require.Equal(s.T(), float64(422), create["code"])

	enabled := s.postJSON("/admin/platform/storage-config", `{
		"name": "启用默认",
		"provider": "local",
		"driver": "local",
		"path_prefix": "uploads",
		"is_default": true,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), enabled["code"])
	id := uint64(enabled["data"].(map[string]any)["id"].(float64))

	update := s.putJSON("/admin/platform/storage-config/"+itoa(id), `{
		"name": "改成禁用默认",
		"provider": "local",
		"driver": "local",
		"path_prefix": "uploads",
		"is_default": true,
		"status": 2
	}`)
	require.Equal(s.T(), float64(422), update["code"])
}

func (s *StorageConfigTestSuite) TestTenantUploadUsesTenantIsolatedPathAndActiveStorageMode() {
	fake := newFakeObjectStorage()
	defer fake.Close()

	create := s.postJSON("/admin/platform/storage-config", `{
		"name": "MinIO 默认",
		"provider": "minio",
		"driver": "s3_compatible",
		"bucket": "tenant-assets",
		"endpoint": "`+fake.URL+`",
		"access_key": "ak",
		"secret_key": "sk",
		"path_prefix": "mine",
		"is_default": true,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), create["code"])

	upload := s.upload("tenant.txt", "tenant object")
	require.Equal(s.T(), float64(200), upload["code"])
	data := upload["data"].(map[string]any)
	require.Equal(s.T(), "minio", data["storage_mode"])
	require.Equal(s.T(), create["data"].(map[string]any)["id"], data["storage_config_id"])
	require.Contains(s.T(), data["storage_path"], "mine/tenants/default/")
	require.NotContains(s.T(), data["storage_path"], "uploads/default/")
	require.True(s.T(), strings.HasPrefix(data["url"].(string), fake.URL+"/tenant-assets/mine/tenants/default/"))
	require.Len(s.T(), fake.requests, 1)
	require.Equal(s.T(), http.MethodPut, fake.requests[0].method)
	require.Contains(s.T(), fake.requests[0].path, "/tenant-assets/mine/tenants/default/")
	require.Contains(s.T(), fake.requests[0].authorization, "AWS4-HMAC-SHA256")
}

func (s *StorageConfigTestSuite) TestS3CompatibleDeleteRemovesRemoteObject() {
	fake := newFakeObjectStorage()
	defer fake.Close()

	create := s.postJSON("/admin/platform/storage-config", `{
		"name": "默认 S3",
		"provider": "aws_s3",
		"driver": "s3_compatible",
		"bucket": "tenant-assets",
		"endpoint": "`+fake.URL+`",
		"access_key": "ak",
		"secret_key": "sk",
		"path_prefix": "mine",
		"is_default": true,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), create["code"])

	upload := s.upload("delete.txt", "remote delete")
	require.Equal(s.T(), float64(200), upload["code"])
	id := uint64(upload["data"].(map[string]any)["id"].(float64))

	local := s.postJSON("/admin/platform/storage-config", `{
		"name": "本地默认",
		"provider": "local",
		"driver": "local",
		"path_prefix": "uploads",
		"is_default": true,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), local["code"])

	s.assertOK(s.Http(s.T()).WithToken(s.tenantToken).Delete("/admin/attachment/"+itoa(id), nil))

	require.Len(s.T(), fake.requests, 2)
	require.Equal(s.T(), http.MethodPut, fake.requests[0].method)
	require.Equal(s.T(), http.MethodDelete, fake.requests[1].method)
	require.Equal(s.T(), fake.requests[0].path, fake.requests[1].path)
}

func (s *StorageConfigTestSuite) TestSameHashCanUploadAfterDefaultStorageChanges() {
	first := s.postJSON("/admin/platform/storage-config", `{
		"name": "本地默认一",
		"provider": "local",
		"driver": "local",
		"path_prefix": "uploads",
		"is_default": true,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), first["code"])

	upload := s.upload("same.txt", "same object")
	require.Equal(s.T(), float64(200), upload["code"])

	second := s.postJSON("/admin/platform/storage-config", `{
		"name": "本地默认二",
		"provider": "local",
		"driver": "local",
		"path_prefix": "assets",
		"is_default": true,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), second["code"])

	next := s.upload("same.txt", "same object")
	require.Equal(s.T(), float64(200), next["code"])
	require.NotEqual(s.T(), upload["data"].(map[string]any)["id"], next["data"].(map[string]any)["id"])
	require.Contains(s.T(), next["data"].(map[string]any)["storage_path"], "assets/tenants/default/")
}

func (s *StorageConfigTestSuite) TestReferencedStorageConfigCannotChangeBackendOrBeDeleted() {
	fake := newFakeObjectStorage()
	defer fake.Close()

	create := s.postJSON("/admin/platform/storage-config", `{
		"name": "默认 S3",
		"provider": "aws_s3",
		"driver": "s3_compatible",
		"bucket": "tenant-assets",
		"endpoint": "`+fake.URL+`",
		"access_key": "ak",
		"secret_key": "sk",
		"path_prefix": "mine",
		"is_default": true,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	id := uint64(create["data"].(map[string]any)["id"].(float64))

	upload := s.upload("referenced.txt", "referenced object")
	require.Equal(s.T(), float64(200), upload["code"])

	changed := s.putJSON("/admin/platform/storage-config/"+itoa(id), `{
		"name": "默认 S3",
		"provider": "aws_s3",
		"driver": "s3_compatible",
		"bucket": "other-assets",
		"endpoint": "`+fake.URL+`",
		"access_key": "ak",
		"secret_key": "sk",
		"path_prefix": "mine",
		"is_default": true,
		"status": 1
	}`)
	require.Equal(s.T(), float64(422), changed["code"])

	deleted := s.deleteJSON("/admin/platform/storage-config", "["+itoa(id)+"]")
	require.Equal(s.T(), float64(422), deleted["code"])
}

func (s *StorageConfigTestSuite) TestPlatformUploadUsesPlatformIsolatedPath() {
	upload := s.platformUpload("platform.txt", "platform object")
	require.Equal(s.T(), float64(200), upload["code"])
	data := upload["data"].(map[string]any)
	require.Equal(s.T(), "local", data["storage_mode"])
	require.Contains(s.T(), data["storage_path"], "uploads/platform/")
	require.NotContains(s.T(), data["storage_path"], "uploads/tenants/")
	require.True(s.T(), strings.HasPrefix(data["url"].(string), "/storage/uploads/platform/"))
}

func (s *StorageConfigTestSuite) TestPlatformAndTenantCanUploadSameHash() {
	platformUpload := s.platformUpload("same.txt", "shared object")
	require.Equal(s.T(), float64(200), platformUpload["code"])
	platformData := platformUpload["data"].(map[string]any)
	require.Contains(s.T(), platformData["storage_path"], "uploads/platform/")

	tenantUpload := s.upload("same.txt", "shared object")
	require.Equal(s.T(), float64(200), tenantUpload["code"])
	tenantData := tenantUpload["data"].(map[string]any)
	require.Contains(s.T(), tenantData["storage_path"], "uploads/tenants/default/")

	require.Equal(s.T(), platformData["hash"], tenantData["hash"])
	require.NotEqual(s.T(), platformData["storage_path"], tenantData["storage_path"])
}

func (s *StorageConfigTestSuite) postJSON(uri, body string) map[string]any {
	body = s.withStorageSensitiveEvidence(body, "storage-config:create")
	res, err := s.Http(s.T()).
		WithToken(s.platformToken).
		Post(uri, strings.NewReader(body))
	require.NoError(s.T(), err)
	res.AssertOk()

	payload, err := res.Json()
	require.NoError(s.T(), err)
	return payload
}

func (s *StorageConfigTestSuite) getJSON(uri string) map[string]any {
	res, err := s.Http(s.T()).
		WithToken(s.platformToken).
		Get(uri)
	require.NoError(s.T(), err)
	res.AssertOk()

	payload, err := res.Json()
	require.NoError(s.T(), err)
	return payload
}

func (s *StorageConfigTestSuite) putJSON(uri, body string) map[string]any {
	id := strings.TrimPrefix(uri, "/admin/platform/storage-config/")
	body = s.withStorageSensitiveEvidence(body, "storage-config:update:"+id)
	res, err := s.Http(s.T()).
		WithToken(s.platformToken).
		Put(uri, strings.NewReader(body))
	require.NoError(s.T(), err)
	res.AssertOk()

	payload, err := res.Json()
	require.NoError(s.T(), err)
	return payload
}

func (s *StorageConfigTestSuite) deleteJSON(uri, body string) map[string]any {
	var ids []uint64
	require.NoError(s.T(), json.Unmarshal([]byte(body), &ids))
	parts := make([]string, len(ids))
	for index, id := range ids {
		parts[index] = itoa(id)
	}
	evidence := s.storageSensitiveEvidence("storage-config:delete:" + strings.Join(parts, ","))
	requestBody, err := json.Marshal(map[string]any{
		"ids": ids, "reauth_token": evidence.ReAuthToken, "approval_id": evidence.ApprovalID,
	})
	require.NoError(s.T(), err)
	res, err := s.Http(s.T()).
		WithToken(s.platformToken).
		Delete(uri, bytes.NewReader(requestBody))
	require.NoError(s.T(), err)
	res.AssertOk()

	payload, err := res.Json()
	require.NoError(s.T(), err)
	return payload
}

func (s *StorageConfigTestSuite) rawPlatformMutation(method, uri, body string) map[string]any {
	request := s.Http(s.T()).WithToken(s.platformToken)
	var response contractshttp.Response
	var err error
	switch method {
	case "POST":
		response, err = request.Post(uri, strings.NewReader(body))
	case "PUT":
		response, err = request.Put(uri, strings.NewReader(body))
	default:
		response, err = request.Delete(uri, strings.NewReader(body))
	}
	require.NoError(s.T(), err)
	response.AssertOk()
	payload, err := response.Json()
	require.NoError(s.T(), err)
	return payload
}

func (s *StorageConfigTestSuite) withStorageSensitiveEvidence(body, resource string) string {
	var payload map[string]any
	require.NoError(s.T(), json.Unmarshal([]byte(body), &payload))
	evidence := s.storageSensitiveEvidence(resource)
	payload["reauth_token"] = evidence.ReAuthToken
	payload["approval_id"] = evidence.ApprovalID
	encoded, err := json.Marshal(payload)
	require.NoError(s.T(), err)
	return string(encoded)
}

func (s *StorageConfigTestSuite) storageSensitiveEvidence(resource string) services.SensitiveOperationEvidence {
	var requesterID uint64
	require.NoError(s.T(), facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("platform_user").Where("username", "admin").Pluck("id", &requesterID))
	security := services.NewEnterpriseSecurityControlService()
	approval, err := security.CreatePlatformApproval(s.T().Context(), services.PlatformApprovalCreateRequest{
		RequesterID: requesterID, PolicyKey: "storage.secret.change", Resource: resource, Reason: "storage feature test",
	})
	require.NoError(s.T(), err)
	result, err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("enterprise_security_approval").
		Where("approval_id", approval.ApprovalID).Where("status", "pending").Update(map[string]any{
		"approver_id": 99, "status": "approved", "updated_at": time.Now(),
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), result.RowsAffected)

	res, err := s.Http(s.T()).WithToken(s.platformToken).Post("/admin/platform/security/reauth-token", strings.NewReader(fmt.Sprintf(`{
		"password":"123456","operation":"storage.secret.change","resource":%q
	}`, resource)))
	s.assertOK(res, err)
	response, err := res.Json()
	require.NoError(s.T(), err)
	return services.SensitiveOperationEvidence{
		ReAuthToken: response["data"].(map[string]any)["reauth_token"].(string),
		ApprovalID:  approval.ApprovalID,
	}
}

func (s *StorageConfigTestSuite) upload(filename, content string) map[string]any {
	return s.multipartUpload("/admin/attachment/upload", s.tenantToken, filename, content)
}

func (s *StorageConfigTestSuite) platformUpload(filename, content string) map[string]any {
	return s.multipartUpload("/admin/platform/attachment/upload", s.platformToken, filename, content)
}

func (s *StorageConfigTestSuite) multipartUpload(uri, token, filename, content string) map[string]any {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(s.T(), err)
	_, err = part.Write([]byte(content))
	require.NoError(s.T(), err)
	require.NoError(s.T(), writer.Close())

	res, err := s.Http(s.T()).
		WithToken(token).
		WithHeader("Content-Type", writer.FormDataContentType()).
		Post(uri, body)
	require.NoError(s.T(), err)
	res.AssertOk()

	payload, err := res.Json()
	require.NoError(s.T(), err)
	return payload
}

func (s *StorageConfigTestSuite) assertOK(response contractshttp.Response, err error) {
	require.NoError(s.T(), err)
	response.AssertOk()

	body, err := response.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
}

func (s *StorageConfigTestSuite) loginAsPlatformAdmin() string {
	res, err := s.Http(s.T()).Post(
		"/admin/platform/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)
	return body.Data.AccessToken
}

type objectStorageRequest struct {
	method        string
	path          string
	authorization string
}

type fakeObjectStorage struct {
	*httptest.Server
	mu       sync.Mutex
	requests []objectStorageRequest
}

func newFakeObjectStorage() *fakeObjectStorage {
	fake := &fakeObjectStorage{}
	fake.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		fake.requests = append(fake.requests, objectStorageRequest{
			method:        r.Method,
			path:          r.URL.Path,
			authorization: r.Header.Get("Authorization"),
		})
		fake.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	return fake
}

func (s *StorageConfigTestSuite) loginAsTenantAdmin() string {
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post(
			"/admin/passport/login",
			strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
		)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)
	return body.Data.AccessToken
}
