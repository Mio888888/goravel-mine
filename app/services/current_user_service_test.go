package services

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/app/models"
)

func TestProfileUpdateValuesKeepsBackendSettingInUserUpdate(t *testing.T) {
	values, err := profileUpdateValues(ProfileUpdate{
		Nickname: "admin",
		BackendSetting: models.JSONMap{
			"app": map[string]any{"useLocale": "zh_CN"},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "admin", values["nickname"])
	raw, ok := values["backend_setting"].(string)
	require.True(t, ok)
	require.JSONEq(t, `{"app":{"useLocale":"zh_CN"}}`, raw)
	require.True(t, json.Valid([]byte(raw)))
}

func TestProfileValidationMessageReturnsBusinessRuleMessage(t *testing.T) {
	err := BusinessError{Message: "密码必须包含特殊字符"}

	require.True(t, IsProfileValidationError(err))
	require.Equal(t, "密码必须包含特殊字符", ProfileValidationMessage(err))
}
