package messagebus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	contractsqueue "github.com/goravel/framework/contracts/queue"

	"goravel/app/facades"
	"goravel/app/models"
	queueservice "goravel/app/services/runtime/queue"
	"goravel/app/support/jobarg"
)

type messageDispatchPayload struct {
	RouteID            uint64          `json:"route_id"`
	AdapterID          uint64          `json:"adapter_id"`
	Envelope           MessageEnvelope `json:"envelope"`
	ReplayDeadLetterID uint64          `json:"replay_dead_letter_id,omitempty"`
}

type MessageConsumeJob struct{}

func (j *MessageConsumeJob) Signature() string {
	return "middleware_message_consume"
}

func (j *MessageConsumeJob) Handle(args ...any) error {
	payload := jobarg.String(args, 0)
	consumerKey := jobarg.String(args, 1)
	attempt := int(jobarg.Uint64(args, 2))
	if attempt < 1 {
		attempt = 1
	}
	var dispatch messageDispatchPayload
	if err := json.Unmarshal([]byte(payload), &dispatch); err != nil {
		return err
	}
	return consumeMessage(context.Background(), dispatch, consumerKey, attempt)
}

func init() {
	queueservice.RegisterQueueOutboxHandler("message.bus.publish", dispatchMessageOutboxEvent)
}

func dispatchMessageOutboxEvent(ctx context.Context, event queueservice.QueueOutboxEvent) error {
	var dispatch messageDispatchPayload
	if err := json.Unmarshal([]byte(event.Payload), &dispatch); err != nil {
		return err
	}
	if err := ValidateMessageEnvelope(dispatch.Envelope); err != nil {
		return NonRetryableMessageError(err)
	}
	consumers := consumersForMessageType(dispatch.Envelope.MessageType)
	if len(consumers) == 0 {
		return NonRetryableMessageError(errors.New("message has no registered consumers"))
	}
	service := NewMiddlewarePlatformService().WithContext(ctx)
	route, err := service.Route(dispatch.RouteID)
	if err != nil {
		return err
	}
	adapter, err := service.Adapter(dispatch.AdapterID)
	if err != nil {
		return err
	}
	if !adapter.Enabled {
		return NonRetryableMessageError(errors.New("message adapter is disabled"))
	}
	encoded, err := json.Marshal(dispatch)
	if err != nil {
		return err
	}
	for _, consumer := range consumers {
		pending := facades.Queue().Job(&MessageConsumeJob{}, []contractsqueue.Arg{
			{Type: "string", Value: string(encoded)},
			{Type: "string", Value: consumer.ConsumerKey},
			{Type: "uint64", Value: uint64(1)},
		})
		if adapter.Connection != "" {
			pending = pending.OnConnection(adapter.Connection)
		}
		if route.Destination != "" {
			pending = pending.OnQueue(route.Destination)
		}
		if adapter.AdapterType == "memory" {
			if err := pending.DispatchSync(); err != nil {
				return err
			}
			continue
		}
		if err := pending.Dispatch(); err != nil {
			return err
		}
	}
	return nil
}

func consumeMessage(ctx context.Context, dispatch messageDispatchPayload, consumerKey string, attempt int) (err error) {
	envelope := dispatch.Envelope
	if err := ValidateMessageEnvelope(envelope); err != nil {
		return createDeadLetterWithoutDelivery(ctx, dispatch, consumerKey, attempt, NonRetryableMessageError(err))
	}
	consumer, ok := messageConsumerDefinition(consumerKey)
	if !ok {
		return createDeadLetterWithoutDelivery(ctx, dispatch, consumerKey, attempt, NonRetryableMessageError(errors.New("message consumer not registered")))
	}
	if consumer.MessageType != envelope.MessageType || !containsVersion(consumer.SupportedSchemaVersions, envelope.SchemaVersion) {
		return createDeadLetterWithoutDelivery(ctx, dispatch, consumerKey, attempt, NonRetryableMessageError(errors.New("message consumer schema is incompatible")))
	}
	started := time.Now()
	delivery := models.MessageDelivery{
		MessageID: envelope.MessageID, MessageType: envelope.MessageType, ConsumerKey: consumerKey,
		RouteID: dispatch.RouteID, AdapterID: dispatch.AdapterID, Status: DeliveryStatusProcessing,
		Attempt: attempt, ReceivedAt: &started, CorrelationID: envelope.CorrelationID,
		Timestamps: models.Timestamps{CreatedAt: started, UpdatedAt: started},
	}
	query := OrmForConnectionWithContext(ctx, PlatformConnection()).Query()
	if err := query.Table("message_delivery").Create(&delivery); err != nil {
		return err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = handleMessageFailure(ctx, dispatch, consumer, delivery, attempt, fmt.Errorf("panic: %v", recovered))
		}
	}()

	idempotencyKey := envelope.MessageID
	if consumer.IdempotencyKeyResolver != nil {
		if resolved := strings.TrimSpace(consumer.IdempotencyKeyResolver(envelope)); resolved != "" {
			idempotencyKey = resolved
		}
	}
	store := queueservice.NewDBQueueIdempotencyStore(PlatformConnection())
	result, handlerErr := store.OnceWithMetadata(
		ctx,
		consumer.ConsumerKey+":"+idempotencyKey,
		queueservice.QueueIdempotencyMetadata{
			ConsumerKey: consumer.ConsumerKey, IdempotencyKey: idempotencyKey, MessageID: envelope.MessageID,
		},
		func(runCtx context.Context) (queueservice.QueueIdempotencyResult, error) {
			if err := consumer.Handler(runCtx, envelope); err != nil {
				return queueservice.QueueIdempotencyResult{}, err
			}
			return queueservice.QueueIdempotencyResult{Status: queueservice.QueueIdempotencyStatusSuccess, Result: envelope.MessageID}, nil
		},
	)
	if handlerErr != nil {
		return handleMessageFailure(ctx, dispatch, consumer, delivery, attempt, handlerErr)
	}
	status := DeliveryStatusSucceeded
	if result.Duplicate || result.Status == queueservice.QueueIdempotencyStatusRunning {
		status = DeliveryStatusIgnored
	}
	return finishMessageDelivery(ctx, delivery, status, "")
}

func handleMessageFailure(
	ctx context.Context,
	dispatch messageDispatchPayload,
	consumer ConsumerDefinition,
	delivery models.MessageDelivery,
	attempt int,
	handlerErr error,
) error {
	service := NewMiddlewarePlatformService().WithContext(ctx)
	route, routeErr := service.Route(dispatch.RouteID)
	if routeErr != nil {
		return routeErr
	}
	failureClass := classifyMessageFailure(handlerErr)
	policy := retryPolicy(route)
	if failureClass == FailureClassRetryable {
		if retryable, delay := policy.ShouldRetry(handlerErr, attempt); retryable {
			if err := finishMessageDelivery(ctx, delivery, DeliveryStatusRetryScheduled, handlerErr.Error()); err != nil {
				return err
			}
			return scheduleMessageRetry(dispatch, consumer.ConsumerKey, attempt+1, route, delay)
		}
	}
	if err := finishMessageDelivery(ctx, delivery, DeliveryStatusDeadLettered, handlerErr.Error()); err != nil {
		return err
	}
	return createDeadLetter(ctx, dispatch, consumer.ConsumerKey, failureClass, handlerErr)
}

func scheduleMessageRetry(dispatch messageDispatchPayload, consumerKey string, attempt int, route models.MessageRoute, delay time.Duration) error {
	adapter, err := NewMiddlewarePlatformService().Adapter(dispatch.AdapterID)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(dispatch)
	if err != nil {
		return err
	}
	pending := facades.Queue().Job(&MessageConsumeJob{}, []contractsqueue.Arg{
		{Type: "string", Value: string(encoded)},
		{Type: "string", Value: consumerKey},
		{Type: "uint64", Value: uint64(attempt)},
	}).Delay(time.Now().Add(delay))
	if adapter.Connection != "" {
		pending = pending.OnConnection(adapter.Connection)
	}
	if route.Destination != "" {
		pending = pending.OnQueue(route.Destination)
	}
	return pending.Dispatch()
}

func finishMessageDelivery(ctx context.Context, delivery models.MessageDelivery, status, errorSummary string) error {
	finished := time.Now()
	duration := int(finished.Sub(*delivery.ReceivedAt).Milliseconds())
	_, err := OrmForConnectionWithContext(ctx, PlatformConnection()).Query().
		Table("message_delivery").
		Where("id", delivery.ID).
		Update(map[string]any{
			"status": status, "finished_at": finished, "duration_ms": duration,
			"error_summary": errorSummary, "updated_at": finished,
		})
	return err
}

func createDeadLetterWithoutDelivery(ctx context.Context, dispatch messageDispatchPayload, consumerKey string, attempt int, failure error) error {
	now := time.Now()
	delivery := models.MessageDelivery{
		MessageID: dispatch.Envelope.MessageID, MessageType: dispatch.Envelope.MessageType,
		ConsumerKey: consumerKey, RouteID: dispatch.RouteID, AdapterID: dispatch.AdapterID,
		Status: DeliveryStatusDeadLettered, Attempt: attempt, ReceivedAt: &now, FinishedAt: &now,
		CorrelationID: dispatch.Envelope.CorrelationID, ErrorSummary: failure.Error(),
		Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
	}
	if err := OrmForConnectionWithContext(ctx, PlatformConnection()).Query().Table("message_delivery").Create(&delivery); err != nil {
		return err
	}
	return createDeadLetter(ctx, dispatch, consumerKey, classifyMessageFailure(failure), failure)
}

func createDeadLetter(ctx context.Context, dispatch messageDispatchPayload, consumerKey, failureClass string, failure error) error {
	now := time.Now()
	rawEnvelope, err := json.Marshal(dispatch.Envelope)
	if err != nil {
		return err
	}
	encryptedEnvelope, err := facades.Crypt().EncryptString(string(rawEnvelope))
	if err != nil {
		return err
	}
	envelope := redactEnvelope(dispatch.Envelope)
	row := models.MessageDeadLetter{
		MessageID: dispatch.Envelope.MessageID, MessageType: dispatch.Envelope.MessageType,
		ConsumerKey: consumerKey, RouteID: dispatch.RouteID, AdapterID: dispatch.AdapterID,
		Envelope: envelopeJSONMap(envelope), EnvelopeEncrypted: encryptedEnvelope,
		FailureClass: failureClass, ErrorSummary: failure.Error(),
		FirstFailedAt: &now, LastFailedAt: &now, ResolutionStatus: DeadLetterStatusOpen,
		Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
	}
	orm := OrmForConnectionWithContext(ctx, PlatformConnection())
	return orm.Transaction(func(tx contractsorm.Query) error {
		redactedEnvelope, err := json.Marshal(row.Envelope)
		if err != nil {
			return err
		}
		scalar := row
		scalar.Envelope = nil
		if err := tx.Table("message_dead_letter").Create(&scalar); err != nil {
			return err
		}
		_, err = tx.Exec(
			"UPDATE message_dead_letter SET envelope = ?::jsonb WHERE id = ?",
			string(redactedEnvelope), scalar.ID,
		)
		return err
	})
}
