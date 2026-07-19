package scheduledtask

import (
	"errors"
	"sort"
	"strings"

	"goravel/app/models"
)

const (
	ScheduledTaskTenantGlobalOnly       = "GLOBAL_ONLY"
	ScheduledTaskTenantPerTenantAllowed = "PER_TENANT_ALLOWED"
)

type ScheduledTaskParameterValidator func(models.JSONMap) error

type ScheduledTaskHandlerDefinition struct {
	HandlerKey           string                          `json:"handler_key"`
	Description          string                          `json:"description"`
	ParameterSchema      models.JSONMap                  `json:"parameter_schema"`
	DefaultTimeout       int                             `json:"default_timeout"`
	TenantCapability     string                          `json:"tenant_capability"`
	SupportsCancellation bool                            `json:"supports_cancellation"`
	Privileged           bool                            `json:"privileged"`
	ValidateParameters   ScheduledTaskParameterValidator `json:"-"`
	Handler              ScheduledTaskHandler            `json:"-"`
}

func RegisterScheduledTaskHandlerDefinition(definition ScheduledTaskHandlerDefinition) error {
	definition.HandlerKey = strings.TrimSpace(definition.HandlerKey)
	if definition.HandlerKey == "" || definition.Handler == nil {
		return errors.New("scheduled task handler key and handler are required")
	}
	if definition.DefaultTimeout < 1 {
		definition.DefaultTimeout = 60
	}
	if definition.TenantCapability == "" {
		definition.TenantCapability = ScheduledTaskTenantPerTenantAllowed
	}
	if definition.TenantCapability != ScheduledTaskTenantGlobalOnly &&
		definition.TenantCapability != ScheduledTaskTenantPerTenantAllowed {
		return errors.New("scheduled task tenant capability is invalid")
	}
	if definition.ParameterSchema == nil {
		definition.ParameterSchema = models.JSONMap{"type": "object"}
	}
	if definition.ValidateParameters == nil {
		definition.ValidateParameters = func(models.JSONMap) error { return nil }
	}

	scheduledTaskHandlers.Lock()
	defer scheduledTaskHandlers.Unlock()
	if _, exists := scheduledTaskHandlers.items[definition.HandlerKey]; exists {
		return errors.New("scheduled task handler already registered: " + definition.HandlerKey)
	}
	scheduledTaskHandlers.items[definition.HandlerKey] = definition
	return nil
}

func MustRegisterScheduledTaskHandlerDefinition(definition ScheduledTaskHandlerDefinition) {
	if err := RegisterScheduledTaskHandlerDefinition(definition); err != nil {
		panic(err)
	}
}

func ScheduledTaskHandlerDefinitions() []ScheduledTaskHandlerDefinition {
	scheduledTaskHandlers.RLock()
	defer scheduledTaskHandlers.RUnlock()
	result := make([]ScheduledTaskHandlerDefinition, 0, len(scheduledTaskHandlers.items))
	for _, definition := range scheduledTaskHandlers.items {
		definition.Handler = nil
		definition.ValidateParameters = nil
		result = append(result, definition)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].HandlerKey < result[j].HandlerKey
	})
	return result
}

func scheduledTaskHandlerDefinition(name string) (ScheduledTaskHandlerDefinition, bool) {
	scheduledTaskHandlers.RLock()
	defer scheduledTaskHandlers.RUnlock()
	definition, ok := scheduledTaskHandlers.items[strings.TrimSpace(name)]
	return definition, ok
}

func ScheduledTaskHandlerPrivileged(name string) bool {
	definition, ok := scheduledTaskHandlerDefinition(name)
	return ok && definition.Privileged
}
