package unit

import (
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/app/services"
	"goravel/app/support/apperror"
)

func TestLoginErrorResultMapsAccountLockout(t *testing.T) {
	result := services.LoginErrorResult(services.ErrAccountLocked)

	require.Equal(t, "账号已锁定，请稍后再试", result.Message)
}

func TestLoginErrorResultMapsBusinessRule(t *testing.T) {
	result := services.LoginErrorResult(apperror.BusinessError{Message: "密码已过期，请修改密码"})

	require.Equal(t, 422, result.Code)
	require.Equal(t, "密码已过期，请修改密码", result.Message)
}
