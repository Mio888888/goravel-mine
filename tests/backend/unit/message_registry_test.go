package unit

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/app/models"
	"goravel/app/services"
)

func TestMessageEnvelopeValidatesSchemaVersionAndMetadata(t *testing.T) {
	const messageType = "test.registry.order-created"
	services.UnregisterMessageType(messageType)
	t.Cleanup(func() {
		services.UnregisterMessageType(messageType)
	})
	require.NoError(t, services.RegisterMessageType(services.MessageTypeDefinition{
		MessageType:             messageType,
		SupportedSchemaVersions: []int{1},
		SensitiveFieldPaths:     []string{"customer.secret"},
		Validators: map[int]services.MessagePayloadValidator{
			1: func(payload models.JSONMap) error {
				if _, ok := payload["order_id"].(string); !ok {
					return errors.New("order_id is required")
				}
				return nil
			},
		},
	}))

	envelope := services.NewMessageEnvelope(messageType, 1, models.JSONMap{
		"order_id": "order-1",
		"customer": map[string]any{"secret": "hidden"},
	})
	envelope.Metadata = map[string]string{"traceparent": "00-test"}
	require.NoError(t, services.ValidateMessageEnvelope(envelope))

	envelope.SchemaVersion = 2
	require.ErrorContains(t, services.ValidateMessageEnvelope(envelope), "unsupported")

	envelope.SchemaVersion = 1
	envelope.Metadata = map[string]string{"authorization": "Bearer secret"}
	require.ErrorContains(t, services.ValidateMessageEnvelope(envelope), "not allowed")

	envelope.Metadata = nil
	envelope.Payload = models.JSONMap{"order_id": 42}
	require.ErrorContains(t, services.ValidateMessageEnvelope(envelope), "order_id is required")
}

func TestMessageRegistryRejectsDuplicatesAndIncompatibleConsumers(t *testing.T) {
	const (
		messageType = "test.registry.invoice-issued"
		consumerKey = "test.registry.invoice-projector"
	)
	services.UnregisterMessageConsumer(consumerKey)
	services.UnregisterMessageType(messageType)
	t.Cleanup(func() {
		services.UnregisterMessageConsumer(consumerKey)
		services.UnregisterMessageType(messageType)
	})
	definition := services.MessageTypeDefinition{
		MessageType:             messageType,
		SupportedSchemaVersions: []int{1},
		Validators: map[int]services.MessagePayloadValidator{
			1: func(models.JSONMap) error { return nil },
		},
	}
	require.NoError(t, services.RegisterMessageType(definition))
	require.ErrorContains(t, services.RegisterMessageType(definition), "already registered")

	incompatible := services.ConsumerDefinition{
		ConsumerKey: consumerKey, MessageType: messageType,
		SupportedSchemaVersions: []int{2}, DefaultMode: services.ConsumptionModeCluster,
		Handler: func(context.Context, services.MessageEnvelope) error { return nil },
	}
	require.ErrorContains(t, services.RegisterMessageConsumer(incompatible), "not supported")

	incompatible.SupportedSchemaVersions = []int{1}
	require.NoError(t, services.RegisterMessageConsumer(incompatible))
	require.ErrorContains(t, services.RegisterMessageConsumer(incompatible), "already registered")

	snapshot := services.MessageRegistrySnapshot()
	require.Contains(t, snapshot.MessageTypes, services.MessageTypeDefinition{
		MessageType:             messageType,
		SupportedSchemaVersions: []int{1},
	})
	require.Len(t, snapshot.Consumers, 1)
	require.Equal(t, consumerKey, snapshot.Consumers[0].ConsumerKey)
}

func TestMessageConsumerRequiresRegisteredTypeAndHandler(t *testing.T) {
	const consumerKey = "test.registry.missing-handler"
	services.UnregisterMessageConsumer(consumerKey)
	t.Cleanup(func() {
		services.UnregisterMessageConsumer(consumerKey)
	})

	err := services.RegisterMessageConsumer(services.ConsumerDefinition{
		ConsumerKey: consumerKey, MessageType: "test.registry.not-registered",
		SupportedSchemaVersions: []int{1},
		Handler:                 func(context.Context, services.MessageEnvelope) error { return nil },
	})
	require.ErrorContains(t, err, "not registered")

	err = services.RegisterMessageConsumer(services.ConsumerDefinition{
		ConsumerKey: consumerKey, MessageType: "test.registry.not-registered",
		SupportedSchemaVersions: []int{1},
	})
	require.ErrorContains(t, err, "handler is required")
}
