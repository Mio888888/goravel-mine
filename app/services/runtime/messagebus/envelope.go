package messagebus

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"goravel/app/models"
	"goravel/app/support/token"
)

const maxMessageMetadataEntries = 32

type MessageEnvelope struct {
	MessageID     string            `json:"message_id"`
	MessageType   string            `json:"message_type"`
	SchemaVersion int               `json:"schema_version"`
	OccurredAt    time.Time         `json:"occurred_at"`
	PublishedAt   time.Time         `json:"published_at"`
	CorrelationID string            `json:"correlation_id"`
	CausationID   string            `json:"causation_id,omitempty"`
	TenantID      string            `json:"tenant_id,omitempty"`
	PartitionKey  string            `json:"partition_key,omitempty"`
	Payload       models.JSONMap    `json:"payload"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

func NewMessageEnvelope(messageType string, schemaVersion int, payload models.JSONMap) MessageEnvelope {
	now := time.Now().UTC()
	messageID := token.RandomHex(16)
	return MessageEnvelope{
		MessageID:     messageID,
		MessageType:   strings.TrimSpace(messageType),
		SchemaVersion: schemaVersion,
		OccurredAt:    now,
		PublishedAt:   now,
		CorrelationID: messageID,
		Payload:       payload,
		Metadata:      map[string]string{},
	}
}

func ValidateMessageEnvelope(envelope MessageEnvelope) error {
	envelope.MessageID = strings.TrimSpace(envelope.MessageID)
	envelope.MessageType = strings.TrimSpace(envelope.MessageType)
	envelope.CorrelationID = strings.TrimSpace(envelope.CorrelationID)
	if envelope.MessageID == "" || envelope.MessageType == "" {
		return errors.New("message id and message type are required")
	}
	if envelope.SchemaVersion < 1 {
		return errors.New("message schema version must be positive")
	}
	if envelope.OccurredAt.IsZero() || envelope.PublishedAt.IsZero() {
		return errors.New("message occurred_at and published_at are required")
	}
	if envelope.CorrelationID == "" {
		return errors.New("message correlation id is required")
	}
	if envelope.Payload == nil {
		return errors.New("message payload is required")
	}
	if len(envelope.Metadata) > maxMessageMetadataEntries {
		return errors.New("message metadata contains too many entries")
	}
	for key, value := range envelope.Metadata {
		key = strings.TrimSpace(key)
		if key == "" || len(key) > 80 || len(value) > 500 {
			return errors.New("message metadata key or value is invalid")
		}
		if isSensitiveMetadataKey(key) {
			return fmt.Errorf("message metadata key is not allowed: %s", key)
		}
	}
	definition, ok := messageTypeDefinition(envelope.MessageType)
	if !ok {
		return fmt.Errorf("message type not registered: %s", envelope.MessageType)
	}
	if !containsVersion(definition.SupportedSchemaVersions, envelope.SchemaVersion) {
		return fmt.Errorf("message schema version unsupported: %d", envelope.SchemaVersion)
	}
	validator := definition.Validators[envelope.SchemaVersion]
	if validator == nil {
		return fmt.Errorf("message schema validator unavailable: %d", envelope.SchemaVersion)
	}
	return validator(envelope.Payload)
}

func EncodeMessageEnvelope(envelope MessageEnvelope) (string, error) {
	if err := ValidateMessageEnvelope(envelope); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func DecodeMessageEnvelope(payload string) (MessageEnvelope, error) {
	var envelope MessageEnvelope
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return MessageEnvelope{}, err
	}
	if err := ValidateMessageEnvelope(envelope); err != nil {
		return MessageEnvelope{}, err
	}
	return envelope, nil
}

func isSensitiveMetadataKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, fragment := range []string{"authorization", "cookie", "password", "secret", "token", "credential"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func redactEnvelope(envelope MessageEnvelope) MessageEnvelope {
	definition, ok := messageTypeDefinition(envelope.MessageType)
	if !ok {
		envelope.Payload = nil
		return envelope
	}
	payload := cloneJSONMap(envelope.Payload)
	for _, path := range definition.SensitiveFieldPaths {
		redactJSONPath(payload, strings.Split(path, "."))
	}
	envelope.Payload = payload
	return envelope
}

func redactJSONPath(value models.JSONMap, path []string) {
	if len(path) == 0 || value == nil {
		return
	}
	key := strings.TrimSpace(path[0])
	if len(path) == 1 {
		if _, exists := value[key]; exists {
			value[key] = "[REDACTED]"
		}
		return
	}
	child, ok := value[key].(map[string]any)
	if !ok {
		if typed, typedOK := value[key].(models.JSONMap); typedOK {
			child = typed
		} else {
			return
		}
	}
	redactJSONPath(models.JSONMap(child), path[1:])
}

func cloneJSONMap(value models.JSONMap) models.JSONMap {
	result := make(models.JSONMap, len(value))
	for key, item := range value {
		if nested, ok := item.(map[string]any); ok {
			result[key] = cloneJSONMap(models.JSONMap(nested))
			continue
		}
		result[key] = item
	}
	return result
}
