package admin

import (
	"encoding/json"
	"testing"

	contractshttp "github.com/goravel/framework/contracts/testing/http"
	"github.com/stretchr/testify/require"

	"goravel/app/facades"
)

func loginJSONWithCaptcha(t testing.TB, request contractshttp.Request, username, password string) string {
	key, answer := captchaAnswer(t, request)
	payload, err := json.Marshal(map[string]string{
		"username":    username,
		"password":    password,
		"captcha_key": key,
		"code":        answer,
	})
	require.NoError(t, err)
	return string(payload)
}

func captchaAnswer(t testing.TB, request contractshttp.Request) (string, string) {
	res, err := request.Get("/admin/passport/captcha")
	require.NoError(t, err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(t, err)
	require.Equal(t, float64(200), body["code"])
	data := body["data"].(map[string]any)
	key := data["key"].(string)
	answer := facades.Cache().GetString("captcha:" + key)
	require.NotEmpty(t, key)
	require.NotEmpty(t, answer)
	return key, answer
}
