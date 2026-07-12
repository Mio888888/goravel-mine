package services

import (
	"container/list"
	"errors"
	"sync"
	"time"

	"goravel/app/facades"
)

var (
	ErrTenantConnectionBudgetExceeded = errors.New("tenant database connection budget exceeded")
)

type tenantConnectionEntry struct {
	name     string
	lastUsed time.Time
}

type tenantConnectionRegistry struct {
	mu       sync.Mutex
	capacity int
	now      func() time.Time
	entries  map[string]*list.Element
	lru      *list.List
}

type TenantConnectionBudget struct {
	Pods              int
	ActiveTenantPools int
	MaxOpenPerPool    int
	PostgreSQLBudget  int
	AllowOvercommit   bool
}

type TenantConnectionBudgetReport struct {
	RequiredConnections int
	PostgreSQLBudget    int
	Safe                bool
}

type TenantConnectionCapacityMetrics struct {
	Pools               int
	RequiredConnections int
	PostgreSQLBudget    int
	Safe                bool
}

var (
	configuredTenantConnectionRegistry     *tenantConnectionRegistry
	configuredTenantConnectionRegistryOnce sync.Once
)

func newTenantConnectionRegistry(capacity int, now func() time.Time) *tenantConnectionRegistry {
	if capacity < 1 {
		capacity = 64
	}
	if now == nil {
		now = time.Now
	}
	return &tenantConnectionRegistry{capacity: capacity, now: now, entries: make(map[string]*list.Element), lru: list.New()}
}

func (r *tenantConnectionRegistry) Acquire(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if element := r.entries[name]; element != nil {
		entry := element.Value.(*tenantConnectionEntry)
		entry.lastUsed = r.now()
		r.lru.MoveToFront(element)
		return nil
	}
	entry := &tenantConnectionEntry{name: name, lastUsed: r.now()}
	r.entries[name] = r.lru.PushFront(entry)
	return nil
}

func (r *tenantConnectionRegistry) Release(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if element := r.entries[name]; element != nil {
		entry := element.Value.(*tenantConnectionEntry)
		entry.lastUsed = r.now()
	}
}

func (r *tenantConnectionRegistry) Contains(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.entries[name]
	return ok
}

func (r *tenantConnectionRegistry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lru.Len()
}

func ValidateTenantConnectionBudget(budget TenantConnectionBudget) (TenantConnectionBudgetReport, error) {
	required := budget.Pods * budget.ActiveTenantPools * budget.MaxOpenPerPool
	report := TenantConnectionBudgetReport{
		RequiredConnections: required,
		PostgreSQLBudget:    budget.PostgreSQLBudget,
		Safe:                required <= budget.PostgreSQLBudget,
	}
	if !report.Safe && !budget.AllowOvercommit {
		return report, ErrTenantConnectionBudgetExceeded
	}
	return report, nil
}

func TenantConnectionBudgetFromConfig() TenantConnectionBudget {
	config := facades.Config()
	return TenantConnectionBudget{
		Pods:              config.GetInt("tenant.pod_count", 1),
		ActiveTenantPools: TenantConnectionRegistryCount(),
		MaxOpenPerPool:    config.GetInt("database.pool.max_open_conns", 20),
		PostgreSQLBudget:  config.GetInt("tenant.database_connection_budget", 500),
		AllowOvercommit:   config.GetBool("tenant.database_connection_budget_override", false),
	}
}

func RegisterTenantConnectionCapacity(name string) error {
	configuredTenantConnectionRegistryOnce.Do(func() {
		configuredTenantConnectionRegistry = newTenantConnectionRegistry(1, time.Now)
	})
	budget := TenantConnectionBudgetFromConfig()
	if !configuredTenantConnectionRegistry.Contains(name) {
		budget.ActiveTenantPools++
	}
	if _, err := ValidateTenantConnectionBudget(budget); err != nil {
		return err
	}
	return configuredTenantConnectionRegistry.Acquire(name)
}

func TenantConnectionRegistryCount() int {
	if configuredTenantConnectionRegistry == nil {
		return 0
	}
	return configuredTenantConnectionRegistry.Count()
}

func TenantConnectionRegistered(name string) bool {
	return configuredTenantConnectionRegistry != nil && configuredTenantConnectionRegistry.Contains(name)
}

func ResetTenantConnectionRegistryForTest() {
	configuredTenantConnectionRegistry = nil
	configuredTenantConnectionRegistryOnce = sync.Once{}
}

func TenantConnectionCapacitySnapshot() TenantConnectionCapacityMetrics {
	budget := TenantConnectionBudgetFromConfig()
	report, err := ValidateTenantConnectionBudget(budget)
	return TenantConnectionCapacityMetrics{
		Pools: TenantConnectionRegistryCount(), RequiredConnections: report.RequiredConnections,
		PostgreSQLBudget: report.PostgreSQLBudget, Safe: err == nil,
	}
}
