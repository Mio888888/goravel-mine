package providers

import (
	"github.com/goravel/framework/contracts/foundation"

	"goravel/app/facades"
	"goravel/app/services"
)

type PlatformBootstrapServiceProvider struct{}

func (r *PlatformBootstrapServiceProvider) Register(app foundation.Application) {}

func (r *PlatformBootstrapServiceProvider) Boot(app foundation.Application) {
	if err := services.NewPlatformBootstrapService().EnsureLocalDefaults(); err != nil {
		facades.Log().Error("platform bootstrap defaults failed", map[string]any{"error": err.Error()})
	}
}
