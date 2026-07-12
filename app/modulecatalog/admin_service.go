package modulecatalog

import (
	"context"

	"goravel/app/http/request"
	"goravel/app/modules"
)

type AdminService struct {
	registry           modules.Registry
	ctx                context.Context
	afterStaleLockRead func(context.Context, []AdminLockRow) error
}

var defaultAdminRegistry modules.Registry

func NewAdminService(registry modules.Registry) *AdminService {
	return &AdminService{registry: registry}
}

func SetDefaultAdminRegistry(registry modules.Registry) {
	defaultAdminRegistry = registry
}

func NewDefaultAdminService() *AdminService {
	return NewAdminService(defaultAdminRegistry)
}

func (s *AdminService) WithContext(ctx context.Context) *AdminService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *AdminService) State() (request.PageResult[ModuleStateItem], error) {
	return newAdminStateQuery(s.registry, s.ctx).state()
}

func (s *AdminService) Runs(filters map[string]string, page, pageSize int) (request.PageResult[AdminRunRow], error) {
	return newAdminRunQuery(s.ctx).runs(adminPageRequest{Filters: filters, Page: page, PageSize: pageSize})
}

func (s *AdminService) Steps(filters map[string]string, page, pageSize int) (request.PageResult[AdminStepRow], error) {
	return newAdminStepQuery(s.ctx).steps(adminPageRequest{Filters: filters, Page: page, PageSize: pageSize})
}

func (s *AdminService) Locks() (request.PageResult[AdminLockRow], error) {
	return newAdminLockQuery(s.ctx).locks()
}

func (s *AdminService) StateDiff() (request.PageResult[AdminStateDiffItem], error) {
	return newAdminStateQuery(s.registry, s.ctx).diff()
}
