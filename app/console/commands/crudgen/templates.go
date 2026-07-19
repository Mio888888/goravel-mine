package crudgen

const modelTemplate = `package models

{{- if .HasTime }}
import (
	"time"
)
{{- end }}

type {{ .StructName }} struct {
{{- range .Columns }}
	{{ .FieldName }} {{ .GoType }} ` + "`gorm:\"column:{{ .Name }}\" json:\"{{ .JSONName }}\"`" + `
{{- end }}
}

func ({{ .StructName }}) TableName() string {
	return "{{ .Name }}"
}
`

const repositoryTemplate = `package {{ .Module }}

import (
	"time"

	"github.com/goravel/framework/support/collect"

	"goravel/app/facades"
	"goravel/app/http/request"
	"goravel/app/models"
	"goravel/app/scopes"
)

type {{ .StructName }}Repository struct{}

func New{{ .StructName }}Repository() *{{ .StructName }}Repository {
	return &{{ .StructName }}Repository{}
}

func (r *{{ .StructName }}Repository) List(filters map[string]string, page, pageSize int) (request.PageResult[models.{{ .StructName }}], error) {
	query := facades.Orm().Query().Table("{{ .Name }}")
{{- range .Columns }}
{{- if .IsStringSearch }}
	query = query.Scopes(scopes.ContainsFoldIfPresent("{{ .Name }}", filters["{{ .Name }}"]))
{{- else if .IsExactSearch }}
	query = query.Scopes(scopes.EqualIfPresent("{{ .Name }}", filters["{{ .Name }}"]))
{{- end }}
{{- end }}

	return request.Paginate[models.{{ .StructName }}](query.OrderByDesc("id"), page, pageSize)
}

func (r *{{ .StructName }}Repository) Create(row *models.{{ .StructName }}) error {
	return facades.Orm().Query().Create(row)
}

func (r *{{ .StructName }}Repository) Update(id uint64, values map[string]any) error {
	values["updated_at"] = time.Now()
	_, err := facades.Orm().Query().Table("{{ .Name }}").Where("id", id).Update(values)
	return err
}

func (r *{{ .StructName }}Repository) Delete(ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := facades.Orm().Query().Table("{{ .Name }}").WhereIn("id", collect.Map(ids, func(id uint64, _ int) any {
		return id
	})).Delete()
	return err
}
`

const requestTemplate = `package {{ .Module }}

type {{ .StructName }}Payload struct {
{{- range .Columns }}
{{- if .IsWritable }}
	{{ .FieldName }} {{ .GoType }} ` + "`json:\"{{ .JSONName }}\"`" + `
{{- end }}
{{- end }}
}

func (p {{ .StructName }}Payload) Values() map[string]any {
	return map[string]any{
{{- range .Columns }}
{{- if .IsWritable }}
		"{{ .Name }}": p.{{ .FieldName }},
{{- end }}
{{- end }}
	}
}
`

const controllerTemplate = `package {{ .Module }}

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	demoRequest "goravel/app/http/request/{{ .Module }}"
	sharedRequest "goravel/app/http/request"
	"goravel/app/http/response"
	"goravel/app/models"
	demoRepository "goravel/app/repositories/{{ .Module }}"
)

type {{ .StructName }}Controller struct {
	repository *demoRepository.{{ .StructName }}Repository
}

func New{{ .StructName }}Controller() *{{ .StructName }}Controller {
	return &{{ .StructName }}Controller{repository: demoRepository.New{{ .StructName }}Repository()}
}

func (r *{{ .StructName }}Controller) List(ctx contractshttp.Context) contractshttp.Response {
	result, err := r.repository.List(ctx.Request().Queries(), sharedRequest.Page(ctx.Request()), sharedRequest.PageSize(ctx.Request()))
	if err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}
	return ctx.Response().Json(http.StatusOK, response.Success(result))
}

func (r *{{ .StructName }}Controller) Create(ctx contractshttp.Context) contractshttp.Response {
	var payload demoRequest.{{ .StructName }}Payload
	if err := ctx.Request().Bind(&payload); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	row := models.{{ .StructName }}{
{{- range .Columns }}
{{- if .IsWritable }}
		{{ .FieldName }}: payload.{{ .FieldName }},
{{- end }}
{{- end }}
	}
	if err := r.repository.Create(&row); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}
	return ctx.Response().Json(http.StatusOK, response.SuccessEmpty())
}

func (r *{{ .StructName }}Controller) Update(ctx contractshttp.Context) contractshttp.Response {
	var payload demoRequest.{{ .StructName }}Payload
	if err := ctx.Request().Bind(&payload); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	if err := r.repository.Update(uint64(ctx.Request().RouteInt64("id")), payload.Values()); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}
	return ctx.Response().Json(http.StatusOK, response.SuccessEmpty())
}

func (r *{{ .StructName }}Controller) Delete(ctx contractshttp.Context) contractshttp.Response {
	var ids []uint64
	if err := ctx.Request().Bind(&ids); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeUnprocessableEntity, "请求参数错误", []any{}))
	}
	if err := r.repository.Delete(ids); err != nil {
		return ctx.Response().Json(http.StatusOK, response.Error(response.CodeFail, "服务器错误", []any{}))
	}
	return ctx.Response().Json(http.StatusOK, response.SuccessEmpty())
}
`

const routeTemplate = `package routes

/*
Register this snippet in routes.Web():

controller := {{ .Module }}.New{{ .StructName }}Controller()
router.Get("GET /admin/{{ .Module }}/{{ .RouteName }}/list", controller.List)
router.Post("POST /admin/{{ .Module }}/{{ .RouteName }}", controller.Create)
router.Put("PUT /admin/{{ .Module }}/{{ .RouteName }}/{id}", controller.Update)
router.Delete("DELETE /admin/{{ .Module }}/{{ .RouteName }}", controller.Delete)
*/
`
