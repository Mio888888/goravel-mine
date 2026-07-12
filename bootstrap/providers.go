package bootstrap

import (
	"github.com/goravel/framework/auth"
	"github.com/goravel/framework/cache"
	"github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/crypt"
	"github.com/goravel/framework/database"
	"github.com/goravel/framework/event"
	"github.com/goravel/framework/filesystem"
	"github.com/goravel/framework/grpc"
	"github.com/goravel/framework/hash"
	"github.com/goravel/framework/http"
	"github.com/goravel/framework/log"
	"github.com/goravel/framework/mail"
	"github.com/goravel/framework/route"
	"github.com/goravel/framework/schedule"
	"github.com/goravel/framework/session"
	"github.com/goravel/framework/telemetry"
	"github.com/goravel/framework/testing"
	"github.com/goravel/framework/translation"
	"github.com/goravel/framework/validation"
	"github.com/goravel/framework/view"
	"github.com/goravel/gin"
	"github.com/goravel/postgres"
	"github.com/goravel/redis"

	appproviders "goravel/app/providers"
)

func Providers() []foundation.ServiceProvider {
	return []foundation.ServiceProvider{
		&http.ServiceProvider{},
		&log.ServiceProvider{},
		&cache.ServiceProvider{},
		&session.ServiceProvider{},
		&telemetry.ServiceProvider{},
		&validation.ServiceProvider{},
		&view.ServiceProvider{},
		&route.ServiceProvider{},
		&gin.ServiceProvider{},
		&database.ServiceProvider{},
		&postgres.ServiceProvider{},
		&redis.ServiceProvider{},
		&auth.ServiceProvider{},
		&crypt.ServiceProvider{},
		&appproviders.QueueServiceProvider{},
		&event.ServiceProvider{},
		&grpc.ServiceProvider{},
		&hash.ServiceProvider{},
		&translation.ServiceProvider{},
		&mail.ServiceProvider{},
		&schedule.ServiceProvider{},
		&filesystem.ServiceProvider{},
		&testing.ServiceProvider{},
		&appproviders.PlatformBootstrapServiceProvider{},
	}
}
