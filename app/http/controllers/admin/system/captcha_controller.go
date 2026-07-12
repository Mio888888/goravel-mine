package system

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type CaptchaController struct {
	service *services.CaptchaService
}

func NewCaptchaController() *CaptchaController {
	return &CaptchaController{service: services.NewCaptchaService()}
}

func (r *CaptchaController) Show(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.service.Generate()
	if err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}
	return ctx.Response().Json(http.StatusOK, response.Success(result))
}
