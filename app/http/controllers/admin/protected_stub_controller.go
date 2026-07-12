package admin

import (
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"goravel/app/http/request"
	"goravel/app/http/response"
)

type ProtectedStubController struct{}

func NewProtectedStubController() *ProtectedStubController {
	return &ProtectedStubController{}
}

func (r *ProtectedStubController) List(ctx contractshttp.Context) contractshttp.Response {
	return ctx.Response().Json(http.StatusOK, response.Success(request.PageResult[any]{
		List:  []any{},
		Total: 0,
	}))
}
