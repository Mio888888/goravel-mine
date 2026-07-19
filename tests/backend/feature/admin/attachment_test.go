package admin

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	contractshttp "github.com/goravel/framework/contracts/testing/http"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/database/seeders"
	"goravel/tests/backend/testcase"
)

type AttachmentTestSuite struct {
	suite.Suite
	tests.TestCase
	token string
}

func TestAttachmentTestSuite(t *testing.T) {
	suite.Run(t, new(AttachmentTestSuite))
}

func (s *AttachmentTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.MenuSeeder{})
	s.Seed(&seeders.DepartmentSeeder{})
	s.Seed(&seeders.CasbinSeeder{})
	s.token = s.loginAs("admin", "123456")
}

func (s *AttachmentTestSuite) TestUploadListStaticAccessDedupeAndDelete() {
	first := s.upload("hello.txt", "hello attachment")
	require.Equal(s.T(), float64(200), first["code"])
	data := first["data"].(map[string]any)
	require.Equal(s.T(), "hello.txt", data["origin_name"])
	require.Equal(s.T(), "txt", data["suffix"])
	require.Equal(s.T(), "text/plain; charset=utf-8", data["mime_type"])
	require.Equal(s.T(), float64(len("hello attachment")), data["size_byte"])
	require.NotEmpty(s.T(), data["hash"])
	require.NotEmpty(s.T(), data["object_name"])
	require.Contains(s.T(), data["storage_path"], "uploads/tenants/default/")
	require.True(s.T(), strings.HasPrefix(data["url"].(string), "/storage/uploads/tenants/default/"))

	path := filepath.Join(
		facades.App().BasePath("storage/app/public"),
		strings.TrimPrefix(data["url"].(string), "/storage/"),
	)
	require.FileExists(s.T(), path)

	staticRes, err := s.Http(s.T()).Get(data["url"].(string))
	require.NoError(s.T(), err)
	staticRes.AssertStatus(http.StatusOK)

	second := s.upload("copy.txt", "hello attachment")
	require.Equal(s.T(), data["id"], second["data"].(map[string]any)["id"])

	list := s.getJSON("/admin/attachment/list?suffix=txt,png&page=1&page_size=10")
	require.Equal(s.T(), float64(200), list["code"])
	page := list["data"].(map[string]any)
	require.Equal(s.T(), float64(1), page["total"])
	rows := page["list"].([]any)
	require.Len(s.T(), rows, 1)
	require.Equal(s.T(), "hello.txt", rows[0].(map[string]any)["origin_name"])

	id := uint64(data["id"].(float64))
	s.assertOK(s.Http(s.T()).WithToken(s.token).Delete("/admin/attachment/"+itoa(id), nil))

	count, err := facades.Orm().Query().Table("attachment").Where("id", id).Count()
	require.NoError(s.T(), err)
	require.Zero(s.T(), count)
	require.NoFileExists(s.T(), path)
}

func (s *AttachmentTestSuite) TestUploadRejectsActiveContent() {
	res := s.upload("payload.html", "<!doctype html><script>alert(1)</script>")
	require.Equal(s.T(), float64(422), res["code"])
}

func (s *AttachmentTestSuite) TestUploadRejectsExistingActiveContentBeforeDedupe() {
	content := "<!doctype html><script>alert(1)</script>"
	hash := md5.Sum([]byte(content))
	legacy := models.Attachment{
		StorageMode: "local",
		OriginName:  "payload.html",
		ObjectName:  "legacy.html",
		Hash:        fmt.Sprintf("%x", hash),
		MimeType:    "text/html",
		StoragePath: "uploads/default/legacy.html",
		Suffix:      "html",
		SizeByte:    int64(len(content)),
		SizeInfo:    "40 B",
		URL:         "/storage/uploads/default/legacy.html",
	}
	require.NoError(s.T(), facades.Orm().Query().Create(&legacy))

	res := s.upload("payload.html", content)
	require.Equal(s.T(), float64(422), res["code"])
}

func (s *AttachmentTestSuite) upload(filename, content string) map[string]any {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(s.T(), err)
	_, err = part.Write([]byte(content))
	require.NoError(s.T(), err)
	require.NoError(s.T(), writer.Close())

	res, err := s.Http(s.T()).
		WithToken(s.token).
		WithHeader("Content-Type", writer.FormDataContentType()).
		Post("/admin/attachment/upload", body)
	require.NoError(s.T(), err)
	res.AssertOk()

	payload, err := res.Json()
	require.NoError(s.T(), err)
	return payload
}

func (s *AttachmentTestSuite) getJSON(uri string) map[string]any {
	res, err := s.Http(s.T()).WithToken(s.token).Get(uri)
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	return body
}

func (s *AttachmentTestSuite) assertOK(response contractshttp.Response, err error) {
	require.NoError(s.T(), err)
	response.AssertOk()

	body, err := response.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(200), body["code"], "body=%v", body)
}

func (s *AttachmentTestSuite) loginAs(username, password string) string {
	res, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), username, password)),
	)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)
	return body.Data.AccessToken
}
