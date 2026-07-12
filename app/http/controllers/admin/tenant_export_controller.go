package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/response"
	"goravel/app/services"
)

func (r *TenantAdminController) RequestExport(ctx contractshttp.Context) contractshttp.Response {
	operator, err := r.currentOperator(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	tenantID, err := routeID(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	tenant, err := r.service.WithContext(ctx.Context()).FindByID(tenantID)
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	var req services.TenantDataExportRequest
	if err := bindJSONBody(ctx, &req); err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	req.OperatorID = operator.ID
	run, err := services.NewTenantDataExportService().WithContext(ctx.Context()).Request(tenant, req)
	return jsonResult(ctx, run, err)
}

func (r *TenantAdminController) ExportStatus(ctx contractshttp.Context) contractshttp.Response {
	operator, err := r.currentOperator(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	tenantID, runID, err := tenantExportRouteIDs(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	status, err := services.NewTenantDataExportService().WithContext(ctx.Context()).StatusForOperator(operator.ID, tenantID, runID)
	return jsonResult(ctx, status, err)
}

func (r *TenantAdminController) DownloadExport(ctx contractshttp.Context) contractshttp.Response {
	operator, err := r.currentOperator(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusOK, services.LoginErrorResult(err))
	}
	tenantID, runID, err := tenantExportRouteIDs(ctx)
	if err != nil {
		return jsonError(ctx, response.CodeUnprocessableEntity, "请求参数错误")
	}
	content, format, err := services.NewTenantDataExportService().WithContext(ctx.Context()).Download(operator.ID, tenantID, runID, ctx.Request().Query("token"))
	if err != nil {
		return jsonResult(ctx, nil, err)
	}
	contentType := "application/x-ndjson"
	if format == "csv" {
		contentType = "text/csv; charset=utf-8"
	}
	return ctx.Response().Header("Content-Disposition", fmt.Sprintf(`attachment; filename="tenant-%d-export-%d.%s"`, tenantID, runID, format)).Data(http.StatusOK, contentType, content)
}

func tenantExportRouteIDs(ctx contractshttp.Context) (uint64, uint64, error) {
	tenantID, err := routeID(ctx)
	if err != nil {
		return 0, 0, err
	}
	runID, err := strconv.ParseUint(strings.TrimSpace(ctx.Request().Route("run_id")), 10, 64)
	return tenantID, runID, err
}
