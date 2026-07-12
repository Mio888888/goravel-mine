package unit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/http/request"
	"goravel/app/http/response"
)

func TestResponseSuccessSnapshot(t *testing.T) {
	payload, err := json.Marshal(response.Success(map[string]any{
		"access_token":  "token",
		"refresh_token": "refresh",
		"expire_at":     3600,
	}))

	require.NoError(t, err)
	require.JSONEq(t, `{
		"code": 200,
		"message": "成功",
		"data": {
			"access_token": "token",
			"refresh_token": "refresh",
			"expire_at": 3600
		}
	}`, string(payload))
}

func TestResponseSuccessEmptyUsesArray(t *testing.T) {
	payload, err := json.Marshal(response.SuccessEmpty())

	require.NoError(t, err)
	require.JSONEq(t, `{
		"code": 200,
		"message": "成功",
		"data": []
	}`, string(payload))
}

func TestResponseValidationErrorSnapshot(t *testing.T) {
	payload, err := json.Marshal(response.Error(response.CodeUnprocessableEntity, "用户名或密码错误", []any{}))

	require.NoError(t, err)
	require.JSONEq(t, `{
		"code": 422,
		"message": "用户名或密码错误",
		"data": []
	}`, string(payload))
}

func TestPageResultEmptyListSnapshot(t *testing.T) {
	payload, err := json.Marshal(request.PageResult[string]{
		List:  []string{},
		Total: 0,
	})

	require.NoError(t, err)
	require.JSONEq(t, `{
		"list": [],
		"total": 0
	}`, string(payload))
}

func TestMineTimeFormat(t *testing.T) {
	mineTime := response.MineTime(time.Date(2026, 6, 29, 23, 45, 8, 0, time.UTC))

	payload, err := json.Marshal(struct {
		CreatedAt response.MineTime `json:"created_at"`
	}{
		CreatedAt: mineTime,
	})

	require.NoError(t, err)
	require.JSONEq(t, `{
		"created_at": "2026-06-29 23:45:08"
	}`, string(payload))
}
