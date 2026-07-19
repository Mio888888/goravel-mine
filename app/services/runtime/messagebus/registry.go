package messagebus

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"goravel/app/models"
)

const (
	ConsumptionModeCluster   = "CLUSTER"
	ConsumptionModeBroadcast = "BROADCAST"
)

type MessagePayloadValidator func(models.JSONMap) error
type MessageHandler func(context.Context, MessageEnvelope) error
type IdempotencyKeyResolver func(MessageEnvelope) string

type MessageTypeDefinition struct {
	MessageType             string                          `json:"message_type"`
	Description             string                          `json:"description"`
	SupportedSchemaVersions []int                           `json:"supported_schema_versions"`
	SensitiveFieldPaths     []string                        `json:"sensitive_field_paths"`
	Validators              map[int]MessagePayloadValidator `json:"-"`
}

type ConsumerDefinition struct {
	ConsumerKey             string                 `json:"consumer_key"`
	MessageType             string                 `json:"message_type"`
	Description             string                 `json:"description"`
	SupportedSchemaVersions []int                  `json:"supported_schema_versions"`
	DefaultMode             string                 `json:"default_mode"`
	Handler                 MessageHandler         `json:"-"`
	IdempotencyKeyResolver  IdempotencyKeyResolver `json:"-"`
}

var messageRegistry = struct {
	sync.RWMutex
	types     map[string]MessageTypeDefinition
	consumers map[string]ConsumerDefinition
}{
	types:     make(map[string]MessageTypeDefinition),
	consumers: make(map[string]ConsumerDefinition),
}

func RegisterMessageType(definition MessageTypeDefinition) error {
	definition.MessageType = strings.TrimSpace(definition.MessageType)
	if definition.MessageType == "" {
		return errors.New("message type is required")
	}
	versions := normalizeVersions(definition.SupportedSchemaVersions)
	if len(versions) == 0 {
		return errors.New("message type supported schema versions are required")
	}
	for _, version := range versions {
		if definition.Validators == nil || definition.Validators[version] == nil {
			return fmt.Errorf("message type %s schema version %d validator is required", definition.MessageType, version)
		}
	}
	definition.SupportedSchemaVersions = versions
	messageRegistry.Lock()
	defer messageRegistry.Unlock()
	if _, exists := messageRegistry.types[definition.MessageType]; exists {
		return fmt.Errorf("message type already registered: %s", definition.MessageType)
	}
	messageRegistry.types[definition.MessageType] = definition
	return nil
}

func MustRegisterMessageType(definition MessageTypeDefinition) {
	if err := RegisterMessageType(definition); err != nil {
		panic(err)
	}
}

func UnregisterMessageType(messageType string) {
	messageRegistry.Lock()
	defer messageRegistry.Unlock()
	delete(messageRegistry.types, strings.TrimSpace(messageType))
}

func RegisterMessageConsumer(definition ConsumerDefinition) error {
	definition.ConsumerKey = strings.TrimSpace(definition.ConsumerKey)
	definition.MessageType = strings.TrimSpace(definition.MessageType)
	definition.DefaultMode = strings.ToUpper(strings.TrimSpace(definition.DefaultMode))
	if definition.ConsumerKey == "" || definition.MessageType == "" {
		return errors.New("consumer key and message type are required")
	}
	if definition.Handler == nil {
		return errors.New("message consumer handler is required")
	}
	if definition.DefaultMode == "" {
		definition.DefaultMode = ConsumptionModeCluster
	}
	if definition.DefaultMode != ConsumptionModeCluster && definition.DefaultMode != ConsumptionModeBroadcast {
		return errors.New("message consumer mode is invalid")
	}
	definition.SupportedSchemaVersions = normalizeVersions(definition.SupportedSchemaVersions)
	if len(definition.SupportedSchemaVersions) == 0 {
		return errors.New("consumer supported schema versions are required")
	}

	messageRegistry.Lock()
	defer messageRegistry.Unlock()
	messageType, exists := messageRegistry.types[definition.MessageType]
	if !exists {
		return fmt.Errorf("message type not registered: %s", definition.MessageType)
	}
	if _, exists := messageRegistry.consumers[definition.ConsumerKey]; exists {
		return fmt.Errorf("message consumer already registered: %s", definition.ConsumerKey)
	}
	for _, version := range definition.SupportedSchemaVersions {
		if !containsVersion(messageType.SupportedSchemaVersions, version) {
			return fmt.Errorf("consumer %s schema version %d is not supported by message type", definition.ConsumerKey, version)
		}
	}
	messageRegistry.consumers[definition.ConsumerKey] = definition
	return nil
}

func MustRegisterMessageConsumer(definition ConsumerDefinition) {
	if err := RegisterMessageConsumer(definition); err != nil {
		panic(err)
	}
}

func UnregisterMessageConsumer(consumerKey string) {
	messageRegistry.Lock()
	defer messageRegistry.Unlock()
	delete(messageRegistry.consumers, strings.TrimSpace(consumerKey))
}

func MessageRegistrySnapshot() RegistrySnapshot {
	messageRegistry.RLock()
	defer messageRegistry.RUnlock()
	types := make([]MessageTypeDefinition, 0, len(messageRegistry.types))
	for _, definition := range messageRegistry.types {
		definition.Validators = nil
		types = append(types, definition)
	}
	consumers := make([]ConsumerDefinition, 0, len(messageRegistry.consumers))
	for _, definition := range messageRegistry.consumers {
		definition.Handler = nil
		definition.IdempotencyKeyResolver = nil
		consumers = append(consumers, definition)
	}
	sort.Slice(types, func(i, j int) bool {
		return types[i].MessageType < types[j].MessageType
	})
	sort.Slice(consumers, func(i, j int) bool {
		return consumers[i].ConsumerKey < consumers[j].ConsumerKey
	})
	return RegistrySnapshot{MessageTypes: types, Consumers: consumers}
}

type RegistrySnapshot struct {
	MessageTypes []MessageTypeDefinition `json:"message_types"`
	Consumers    []ConsumerDefinition    `json:"consumers"`
}

func messageTypeDefinition(messageType string) (MessageTypeDefinition, bool) {
	messageRegistry.RLock()
	defer messageRegistry.RUnlock()
	definition, ok := messageRegistry.types[strings.TrimSpace(messageType)]
	return definition, ok
}

func messageConsumerDefinition(consumerKey string) (ConsumerDefinition, bool) {
	messageRegistry.RLock()
	defer messageRegistry.RUnlock()
	definition, ok := messageRegistry.consumers[strings.TrimSpace(consumerKey)]
	return definition, ok
}

func consumersForMessageType(messageType string) []ConsumerDefinition {
	messageRegistry.RLock()
	defer messageRegistry.RUnlock()
	consumers := make([]ConsumerDefinition, 0)
	for _, consumer := range messageRegistry.consumers {
		if consumer.MessageType == messageType {
			consumers = append(consumers, consumer)
		}
	}
	sort.Slice(consumers, func(i, j int) bool {
		return consumers[i].ConsumerKey < consumers[j].ConsumerKey
	})
	return consumers
}

func normalizeVersions(versions []int) []int {
	seen := make(map[int]struct{}, len(versions))
	result := make([]int, 0, len(versions))
	for _, version := range versions {
		if version < 1 {
			continue
		}
		if _, exists := seen[version]; exists {
			continue
		}
		seen[version] = struct{}{}
		result = append(result, version)
	}
	sort.Ints(result)
	return result
}

func containsVersion(versions []int, target int) bool {
	for _, version := range versions {
		if version == target {
			return true
		}
	}
	return false
}
