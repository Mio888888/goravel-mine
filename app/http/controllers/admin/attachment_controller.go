package admin

import (
	"net/http"
	"path/filepath"
	"strings"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

type AttachmentController struct {
	passport *services.PassportService
	service  *services.AttachmentService
}

func NewAttachmentController() *AttachmentController {
	return &AttachmentController{
		passport: services.NewPassportService(),
		service:  services.NewAttachmentService(),
	}
}

func (r *AttachmentController) List(ctx contractshttp.Context) contractshttp.Response {
	service, err := tenantAttachmentService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	result, err := service.List(queryFilters(ctx), page(ctx), pageSize(ctx))
	return jsonResult(ctx, result, err)
}

func (r *AttachmentController) Upload(ctx contractshttp.Context) contractshttp.Response {
	passport, err := tenantPassport(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	user, err := passport.UserFromAuthorization(ctx.Request().Header("Authorization", ""))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	originName, suffix := uploadOriginalName(ctx)
	file, err := ctx.Request().File("file")
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请选择上传文件")
	}
	service, err := tenantAttachmentService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	attachment, err := service.Upload(file, user.ID, originName, suffix)
	return jsonResult(ctx, attachment, err)
}

func (r *AttachmentController) PlatformUpload(ctx contractshttp.Context) contractshttp.Response {
	user, err := platformCurrentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	originName, suffix := uploadOriginalName(ctx)
	file, err := ctx.Request().File("file")
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请选择上传文件")
	}
	attachment, err := services.NewAttachmentService().WithContext(ctx.Context()).Upload(file, user.ID, originName, suffix)
	return jsonResult(ctx, attachment, err)
}

func (r *AttachmentController) Delete(ctx contractshttp.Context) contractshttp.Response {
	id, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	service, err := tenantAttachmentService(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	return jsonResult(ctx, nil, service.Delete(id))
}

func uploadOriginalName(ctx contractshttp.Context) (string, string) {
	source, header, err := ctx.Request().Origin().FormFile("file")
	if err != nil {
		return "", ""
	}
	_ = source.Close()
	name := header.Filename
	return name, strings.TrimPrefix(filepath.Ext(name), ".")
}
