package modulecatalog

import "goravel/app/modules"

type lifecyclePersistencePorts interface {
	LifecycleRepository
	LifecycleLockManager
}

func bindLifecyclePersistence(service *LifecycleService, ports lifecyclePersistencePorts) {
	service.repository = ports
	service.lockManager = ports
}

func newAlphaLifecycleService(lifecycle modules.Lifecycle) (*LifecycleService, *MemoryLifecycleStore) {
	store := NewMemoryLifecycleStore()
	return newAlphaLifecycleServiceWithPorts(lifecycle, store), store
}

func newAlphaLifecycleServiceWithPorts(lifecycle modules.Lifecycle, ports lifecyclePersistencePorts) *LifecycleService {
	registry := modules.NewRegistry([]modules.Module{
		lifecycleStubModule{id: "alpha", metadata: modules.Metadata{
			Name: "Alpha", Version: "1.0.0", Lifecycle: lifecycle,
		}},
	})
	service := NewLifecycleService(registry)
	bindLifecyclePersistence(service, ports)
	return service
}
