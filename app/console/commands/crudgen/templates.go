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

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
	"goravel/app/http/request"
	"goravel/app/models"
)

type {{ .StructName }}Repository struct{}

func New{{ .StructName }}Repository() *{{ .StructName }}Repository {
	return &{{ .StructName }}Repository{}
}

func (r *{{ .StructName }}Repository) List(filters map[string]string, page, pageSize int) (request.PageResult[models.{{ .StructName }}], error) {
	query := facades.Orm().Query().Table("{{ .Name }}")
{{- range .Columns }}
{{- if .IsStringSearch }}
	query = applyStringFilter(query, "{{ .Name }}", filters["{{ .Name }}"])
{{- else if .IsExactSearch }}
	if filters["{{ .Name }}"] != "" {
		query = query.Where("{{ .Name }}", filters["{{ .Name }}"])
	}
{{- end }}
{{- end }}

	total, err := query.Count()
	if err != nil {
		return request.PageResult[models.{{ .StructName }}]{}, err
	}

	rows := make([]models.{{ .StructName }}, 0)
	err = query.OrderBy("id", "desc").Offset((page - 1) * pageSize).Limit(pageSize).Get(&rows)
	if err != nil {
		return request.PageResult[models.{{ .StructName }}]{}, err
	}
	return request.PageResult[models.{{ .StructName }}]{List: rows, Total: total}, nil
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
	values := make([]any, 0, len(ids))
	for _, id := range ids {
		values = append(values, id)
	}
	_, err := facades.Orm().Query().Table("{{ .Name }}").WhereIn("id", values).Delete()
	return err
}

func applyStringFilter(query contractsorm.Query, column, value string) contractsorm.Query {
	if value == "" {
		return query
	}
	return query.Where(column+" ILIKE ?", "%"+value+"%")
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
	result, err := r.repository.List(ctx.Request().Queries(), page(ctx), pageSize(ctx))
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

func page(ctx contractshttp.Context) int {
	value := ctx.Request().QueryInt("page", 1)
	if value < 1 {
		return 1
	}
	return value
}

func pageSize(ctx contractshttp.Context) int {
	value := ctx.Request().QueryInt("per_page", 15)
	if value < 1 {
		return 15
	}
	return value
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
