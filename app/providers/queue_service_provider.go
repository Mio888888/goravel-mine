package providers

import (
	"github.com/goravel/framework/contracts/foundation"
	contractsqueue "github.com/goravel/framework/contracts/queue"
	frameworkqueue "github.com/goravel/framework/queue"
)

type QueueServiceProvider struct {
	frameworkqueue.ServiceProvider
}

func (r *QueueServiceProvider) Runners(app foundation.Application) []foundation.Runner {
	config := app.MakeConfig()
	connection := config.GetString("queue.default")
	driver := config.GetString("queue.connections." + connection + ".driver")
	if !shouldRunQueueWorker(config.GetBool("queue.worker.enabled", true), connection, driver) {
		return []foundation.Runner{}
	}
	return []foundation.Runner{queueRunner{worker: app.MakeQueue().Worker()}}
}

func shouldRunQueueWorker(enabled bool, connection, driver string) bool {
	return enabled && connection != "" && driver != "sync"
}

type queueRunner struct {
	worker contractsqueue.Worker
}

func (r queueRunner) Signature() string {
	return "queue"
}

func (r queueRunner) ShouldRun() bool {
	return r.worker != nil
}

func (r queueRunner) Run() error {
	return r.worker.Run()
}

func (r queueRunner) Shutdown() error {
	return r.worker.Shutdown()
}
