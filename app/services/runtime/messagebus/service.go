package messagebus

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	frameworkerrors "github.com/goravel/framework/errors"

	"goravel/app/facades"
	"goravel/app/http/request"
	"goravel/app/models"
	"goravel/app/scopes"
	queueservice "goravel/app/services/runtime/queue"
)

const (
	RouteStatusDraft     = "DRAFT"
	RouteStatusPublished = "PUBLISHED"

	DeliveryStatusProcessing     = "PROCESSING"
	DeliveryStatusSucceeded      = "SUCCEEDED"
	DeliveryStatusRetryScheduled = "RETRY_SCHEDULED"
	DeliveryStatusDeadLettered   = "DEAD_LETTERED"
	DeliveryStatusIgnored        = "IGNORED"

	DeadLetterStatusOpen     = "OPEN"
	DeadLetterStatusResolved = "RESOLVED"
)

type RoutePayload struct {
	Name             string         `json:"name"`
	MessageType      string         `json:"message_type"`
	AdapterID        uint64         `json:"adapter_id"`
	Destination      string         `json:"destination"`
	ConsumptionMode  string         `json:"consumption_mode"`
	ConsumerGroup    string         `json:"consumer_group"`
	Concurrency      int            `json:"concurrency"`
	OrderingEnabled  bool           `json:"ordering_enabled"`
	RetryPolicy      models.JSONMap `json:"retry_policy"`
	DeadLetterPolicy models.JSONMap `json:"dead_letter_policy"`
	Enabled          *bool          `json:"enabled"`
	Version          int            `json:"version"`
}

type AdapterPayload struct {
	Name       string `json:"name"`
	Connection string `json:"connection"`
	Enabled    *bool  `json:"enabled"`
	Version    int    `json:"version"`
	Confirm    bool   `json:"confirm"`
}

type AdapterConnectionTestResult struct {
	AdapterID uint64 `json:"adapter_id"`
	Status    string `json:"status"`
}

type RouteValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

type PublishReceipt struct {
	MessageID string `json:"message_id"`
	RouteID   uint64 `json:"route_id"`
	Status    string `json:"status"`
}

type ReplayReceipt struct {
	DeadLetterID uint64 `json:"dead_letter_id"`
	MessageID    string `json:"message_id"`
	Status       string `json:"status"`
}

type MiddlewarePlatformService struct {
	ctx context.Context
}

func NewMiddlewarePlatformService() *MiddlewarePlatformService {
	return &MiddlewarePlatformService{ctx: context.Background()}
}

func (s *MiddlewarePlatformService) WithContext(ctx context.Context) *MiddlewarePlatformService {
	return &MiddlewarePlatformService{ctx: contextOrBackground(ctx)}
}

func (s *MiddlewarePlatformService) Registry() RegistrySnapshot {
	return MessageRegistrySnapshot()
}

func (s *MiddlewarePlatformService) Adapters() ([]models.MiddlewareAdapter, error) {
	if err := EnsureConfiguredAdapters(s.ctx); err != nil {
		return nil, err
	}
	rows := make([]models.MiddlewareAdapter, 0)
	if err := s.query().Table("middleware_adapter").OrderBy("id").Get(&rows); err != nil {
		return nil, err
	}
	for index := range rows {
		rows[index].Configured = strings.TrimSpace(rows[index].ConfigEncrypted) != ""
		rows[index].ConfigEncrypted = ""
	}
	return rows, nil
}

func (s *MiddlewarePlatformService) Adapter(id uint64) (models.MiddlewareAdapter, error) {
	adapter, err := s.rawAdapter(id)
	if err != nil {
		return models.MiddlewareAdapter{}, err
	}
	return sanitizeAdapter(adapter), nil
}

func (s *MiddlewarePlatformService) rawAdapter(id uint64) (models.MiddlewareAdapter, error) {
	if err := EnsureConfiguredAdapters(s.ctx); err != nil {
		return models.MiddlewareAdapter{}, err
	}
	var adapter models.MiddlewareAdapter
	err := s.query().Table("middleware_adapter").Where("id", id).First(&adapter)
	if err != nil {
		if err == frameworkerrors.OrmRecordNotFound {
			return models.MiddlewareAdapter{}, BusinessError{Message: "中间件适配器不存在"}
		}
		return models.MiddlewareAdapter{}, err
	}
	return adapter, nil
}

func (s *MiddlewarePlatformService) CheckAdapterHealth(id uint64) (models.MiddlewareAdapter, error) {
	adapter, err := s.rawAdapter(id)
	if err != nil {
		return models.MiddlewareAdapter{}, err
	}
	_, healthErr := CheckAdapterHealth(s.ctx, adapter)
	updated, loadErr := s.Adapter(id)
	if loadErr != nil {
		return models.MiddlewareAdapter{}, loadErr
	}
	if healthErr != nil {
		return updated, BusinessError{Message: healthErr.Error()}
	}
	return updated, nil
}

func (s *MiddlewarePlatformService) TestAdapterConnection(id uint64) (AdapterConnectionTestResult, error) {
	adapter, err := s.rawAdapter(id)
	if err != nil {
		return AdapterConnectionTestResult{}, err
	}
	status, err := ProbeAdapterHealth(s.ctx, adapter)
	result := AdapterConnectionTestResult{AdapterID: id, Status: status}
	if err != nil {
		return result, BusinessError{Message: err.Error()}
	}
	return result, nil
}

func (s *MiddlewarePlatformService) RegisterConfiguredAdapter(payload AdapterPayload, operatorID uint64) (models.MiddlewareAdapter, error) {
	connection := strings.TrimSpace(payload.Connection)
	definition, err := ConfiguredAdapter(connection)
	if err != nil {
		return models.MiddlewareAdapter{}, BusinessError{Message: "仅支持注册已配置的 Goravel Queue 连接"}
	}
	if err := upsertAdapter(s.ctx, definition); err != nil {
		return models.MiddlewareAdapter{}, err
	}
	var row models.MiddlewareAdapter
	if err := s.query().Table("middleware_adapter").Where("adapter_key", definition.AdapterKey).First(&row); err != nil {
		return models.MiddlewareAdapter{}, err
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = row.Name
	}
	enabled := row.Enabled
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}
	if err := s.ensureAdapterNameAvailable(row.ID, name); err != nil {
		return models.MiddlewareAdapter{}, err
	}
	_, err = s.query().Table("middleware_adapter").Where("id", row.ID).Update(map[string]any{
		"name": name, "enabled": enabled, "updated_by": operatorID, "updated_at": time.Now(),
	})
	if err != nil {
		return models.MiddlewareAdapter{}, err
	}
	return s.Adapter(row.ID)
}

func (s *MiddlewarePlatformService) UpdateAdapter(id uint64, payload AdapterPayload, operatorID uint64) (models.MiddlewareAdapter, error) {
	existing, err := s.rawAdapter(id)
	if err != nil {
		return models.MiddlewareAdapter{}, err
	}
	if payload.Version < 1 || payload.Version != existing.Version {
		return models.MiddlewareAdapter{}, BusinessError{Message: "中间件适配器版本冲突，请刷新后重试"}
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" || len(name) > 120 {
		return models.MiddlewareAdapter{}, BusinessError{Message: "中间件适配器名称不能为空且不能超过 120 个字符"}
	}
	if connection := strings.TrimSpace(payload.Connection); connection != "" && connection != existing.Connection {
		return models.MiddlewareAdapter{}, BusinessError{Message: "适配器连接由服务端静态配置管理，不能在线修改"}
	}
	if err := s.ensureAdapterNameAvailable(id, name); err != nil {
		return models.MiddlewareAdapter{}, err
	}
	enabled := existing.Enabled
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}
	if !enabled {
		if err := s.ensureAdapterDisableConfirmed(id, payload.Confirm); err != nil {
			return models.MiddlewareAdapter{}, err
		}
	}
	result, err := s.query().Table("middleware_adapter").
		Where("id", id).Where("version", existing.Version).
		Update(map[string]any{
			"name": name, "enabled": enabled, "version": existing.Version + 1,
			"updated_by": operatorID, "updated_at": time.Now(),
		})
	if err != nil {
		return models.MiddlewareAdapter{}, err
	}
	if result.RowsAffected != 1 {
		return models.MiddlewareAdapter{}, BusinessError{Message: "中间件适配器版本冲突，请刷新后重试"}
	}
	return s.Adapter(id)
}

func (s *MiddlewarePlatformService) SetAdapterEnabled(
	id uint64,
	enabled bool,
	expectedVersion int,
	confirm bool,
	operatorID uint64,
) (models.MiddlewareAdapter, error) {
	adapter, err := s.rawAdapter(id)
	if err != nil {
		return models.MiddlewareAdapter{}, err
	}
	if expectedVersion != adapter.Version {
		return models.MiddlewareAdapter{}, BusinessError{Message: "中间件适配器版本冲突，请刷新后重试"}
	}
	if !enabled {
		if err := s.ensureAdapterDisableConfirmed(id, confirm); err != nil {
			return models.MiddlewareAdapter{}, err
		}
	}
	result, err := s.query().Table("middleware_adapter").
		Where("id", id).Where("version", expectedVersion).
		Update(map[string]any{
			"enabled": enabled, "version": expectedVersion + 1,
			"updated_by": operatorID, "updated_at": time.Now(),
		})
	if err != nil {
		return models.MiddlewareAdapter{}, err
	}
	if result.RowsAffected != 1 {
		return models.MiddlewareAdapter{}, BusinessError{Message: "中间件适配器版本冲突，请刷新后重试"}
	}
	return s.Adapter(id)
}

func (s *MiddlewarePlatformService) ReplaceAdapterConfig(id uint64, expectedVersion int) error {
	adapter, err := s.rawAdapter(id)
	if err != nil {
		return err
	}
	if expectedVersion != adapter.Version {
		return BusinessError{Message: "中间件适配器版本冲突，请刷新后重试"}
	}
	return BusinessError{Message: "当前 Goravel Queue 适配器使用服务端静态配置，不支持在线替换连接密钥"}
}

func (s *MiddlewarePlatformService) Routes(filters map[string]string, page, pageSize int) (request.PageResult[models.MessageRoute], error) {
	query := s.query().Table("message_route")
	query = query.Scopes(scopes.Contains("name", filters["name"]))
	query = query.Scopes(scopes.Equal("message_type", filters["message_type"]))
	query = query.Scopes(scopes.Equal("status", strings.ToUpper(filters["status"])))
	query = query.Scopes(scopes.EqualIfPresent("adapter_id", filters["adapter_id"]))
	return request.Paginate[models.MessageRoute](query.OrderByDesc("id"), page, pageSize)
}

func (s *MiddlewarePlatformService) Route(id uint64) (models.MessageRoute, error) {
	var route models.MessageRoute
	err := s.query().Table("message_route").Where("id", id).First(&route)
	if err != nil {
		if err == frameworkerrors.OrmRecordNotFound {
			return models.MessageRoute{}, BusinessError{Message: "消息路由不存在"}
		}
		return models.MessageRoute{}, err
	}
	adapter, adapterErr := s.Adapter(route.AdapterID)
	if adapterErr == nil {
		route.Adapter = &adapter
	}
	return route, nil
}

func (s *MiddlewarePlatformService) CreateRoute(payload RoutePayload, operatorID uint64) (models.MessageRoute, error) {
	route := payload.route(true)
	route.Status = RouteStatusDraft
	route.Version = 1
	route.CreatedBy = operatorID
	route.UpdatedBy = operatorID
	validation := s.ValidateRouteDefinition(route)
	if !validation.Valid {
		return models.MessageRoute{}, BusinessError{Message: strings.Join(validation.Errors, "；")}
	}
	now := time.Now()
	route.CreatedAt = now
	route.UpdatedAt = now
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		row := route
		row.RetryPolicy = nil
		row.DeadLetterPolicy = nil
		if err := tx.Table("message_route").Create(&row); err != nil {
			return err
		}
		route.ID = row.ID
		return updateMessageRouteJSON(tx, row.ID, route.RetryPolicy, route.DeadLetterPolicy)
	}); err != nil {
		return models.MessageRoute{}, err
	}
	return route, nil
}

func (s *MiddlewarePlatformService) UpdateRoute(id uint64, payload RoutePayload, operatorID uint64) (models.MessageRoute, error) {
	existing, err := s.Route(id)
	if err != nil {
		return models.MessageRoute{}, err
	}
	if payload.Version < 1 || payload.Version != existing.Version {
		return models.MessageRoute{}, BusinessError{Message: "消息路由版本冲突，请刷新后重试"}
	}
	route := payload.route(existing.Enabled)
	route.ID = id
	route.Status = RouteStatusDraft
	route.Version = existing.Version + 1
	validation := s.ValidateRouteDefinition(route)
	if !validation.Valid {
		return models.MessageRoute{}, BusinessError{Message: strings.Join(validation.Errors, "；")}
	}
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		result, updateErr := tx.Table("message_route").
			Where("id", id).
			Where("version", existing.Version).
			Update(map[string]any{
				"name": route.Name, "message_type": route.MessageType, "adapter_id": route.AdapterID,
				"destination": route.Destination, "consumption_mode": route.ConsumptionMode,
				"consumer_group": route.ConsumerGroup, "concurrency": route.Concurrency,
				"ordering_enabled": route.OrderingEnabled, "status": RouteStatusDraft,
				"enabled": route.Enabled, "version": route.Version, "published_at": nil,
				"updated_by": operatorID, "updated_at": time.Now(),
			})
		if updateErr != nil {
			return updateErr
		}
		if result.RowsAffected != 1 {
			return BusinessError{Message: "消息路由版本冲突，请刷新后重试"}
		}
		return updateMessageRouteJSON(tx, id, route.RetryPolicy, route.DeadLetterPolicy)
	}); err != nil {
		return models.MessageRoute{}, err
	}
	return s.Route(id)
}

func (s *MiddlewarePlatformService) ValidateRoute(id uint64) (RouteValidationResult, error) {
	route, err := s.Route(id)
	if err != nil {
		return RouteValidationResult{}, err
	}
	return s.ValidateRouteDefinition(route), nil
}

func (s *MiddlewarePlatformService) ValidateRouteDefinition(route models.MessageRoute) RouteValidationResult {
	result := RouteValidationResult{Valid: true, Errors: []string{}, Warnings: []string{}}
	fail := func(message string) {
		result.Valid = false
		result.Errors = append(result.Errors, message)
	}
	if strings.TrimSpace(route.Name) == "" || strings.TrimSpace(route.MessageType) == "" {
		fail("路由名称和消息类型不能为空")
	}
	if _, exists := messageTypeDefinition(route.MessageType); !exists {
		fail("消息类型未在代码注册表中注册")
	}
	if len(consumersForMessageType(route.MessageType)) == 0 {
		fail("消息类型没有已注册消费者")
	}
	adapter, err := s.Adapter(route.AdapterID)
	if err != nil {
		fail("消息适配器不存在")
		return result
	}
	if !adapter.Enabled {
		fail("消息适配器已禁用")
	}
	capabilities := adapterCapabilities(adapter)
	switch route.ConsumptionMode {
	case ConsumptionModeCluster:
		if !capabilities.Cluster {
			fail("适配器不支持集群消费")
		}
		if adapter.AdapterType != "memory" && strings.TrimSpace(route.ConsumerGroup) == "" {
			fail("集群消费必须配置消费者组")
		}
	case ConsumptionModeBroadcast:
		if !capabilities.Broadcast {
			fail("适配器不支持广播消费")
		}
		if !capabilities.Persistent {
			result.Warnings = append(result.Warnings, "广播适配器不持久化，离线实例可能丢失消息")
		}
	default:
		fail("消费模式无效")
	}
	if route.Concurrency < 1 {
		fail("消费者并发必须为正整数")
	}
	if strings.TrimSpace(route.Destination) == "" {
		fail("消息目标不能为空")
	}
	if route.OrderingEnabled {
		if !capabilities.Ordering {
			fail("适配器不支持顺序消息")
		}
		if route.Concurrency != 1 {
			fail("顺序消息路由并发必须为 1")
		}
	}
	if retryPolicy(route).MaxAttempts < 1 {
		fail("重试最大尝试次数必须为正整数")
	}
	return result
}

func (s *MiddlewarePlatformService) PublishRoute(id uint64, expectedVersion int, operatorID uint64) (models.MessageRoute, error) {
	route, err := s.Route(id)
	if err != nil {
		return models.MessageRoute{}, err
	}
	if expectedVersion != route.Version {
		return models.MessageRoute{}, BusinessError{Message: "消息路由版本冲突，请刷新后重试"}
	}
	validation := s.ValidateRouteDefinition(route)
	if !validation.Valid {
		return models.MessageRoute{}, BusinessError{Message: strings.Join(validation.Errors, "；")}
	}
	now := time.Now()
	result, err := s.query().Table("message_route").
		Where("id", id).
		Where("version", expectedVersion).
		Update(map[string]any{
			"status": RouteStatusPublished, "version": expectedVersion + 1,
			"published_at": now, "updated_by": operatorID, "updated_at": now,
		})
	if err != nil {
		return models.MessageRoute{}, err
	}
	if result.RowsAffected != 1 {
		return models.MessageRoute{}, BusinessError{Message: "消息路由版本冲突，请刷新后重试"}
	}
	return s.Route(id)
}

func (s *MiddlewarePlatformService) PublishRouteIdempotent(
	id uint64,
	expectedVersion int,
	operatorID uint64,
	idempotencyKey string,
) (models.MessageRoute, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return models.MessageRoute{}, BusinessError{Message: "消息路由发布幂等键不能为空"}
	}
	store := queueservice.NewDBQueueIdempotencyStore(PlatformConnection())
	result, err := store.Once(
		s.ctx,
		fmt.Sprintf("middleware-route-publish:%d:%x", id, sha256.Sum256([]byte(idempotencyKey))),
		func(context.Context) (queueservice.QueueIdempotencyResult, error) {
			route, publishErr := s.PublishRoute(id, expectedVersion, operatorID)
			if publishErr != nil {
				return queueservice.QueueIdempotencyResult{}, publishErr
			}
			encoded, marshalErr := json.Marshal(route)
			if marshalErr != nil {
				return queueservice.QueueIdempotencyResult{}, marshalErr
			}
			return queueservice.QueueIdempotencyResult{
				Status: queueservice.QueueIdempotencyStatusSuccess,
				Result: string(encoded),
			}, nil
		},
	)
	if err != nil {
		return models.MessageRoute{}, err
	}
	if result.Status == queueservice.QueueIdempotencyStatusRunning || strings.TrimSpace(result.Result) == "" {
		return models.MessageRoute{}, BusinessError{Message: "消息路由发布操作正在处理中，请稍后重试"}
	}
	var route models.MessageRoute
	if err := json.Unmarshal([]byte(result.Result), &route); err != nil {
		return models.MessageRoute{}, err
	}
	return route, nil
}

func (s *MiddlewarePlatformService) SetRouteEnabled(id uint64, enabled bool, expectedVersion int, operatorID uint64) (models.MessageRoute, error) {
	route, err := s.Route(id)
	if err != nil {
		return models.MessageRoute{}, err
	}
	if expectedVersion != route.Version {
		return models.MessageRoute{}, BusinessError{Message: "消息路由版本冲突，请刷新后重试"}
	}
	result, err := s.query().Table("message_route").
		Where("id", id).
		Where("version", expectedVersion).
		Update(map[string]any{
			"enabled": enabled, "version": expectedVersion + 1,
			"updated_by": operatorID, "updated_at": time.Now(),
		})
	if err != nil {
		return models.MessageRoute{}, err
	}
	if result.RowsAffected != 1 {
		return models.MessageRoute{}, BusinessError{Message: "消息路由版本冲突，请刷新后重试"}
	}
	return s.Route(id)
}

func (s *MiddlewarePlatformService) Publish(envelope MessageEnvelope) (PublishReceipt, error) {
	return s.publishWithQuery(nil, envelope)
}

func (s *MiddlewarePlatformService) PublishWithQuery(query contractsorm.Query, envelope MessageEnvelope) (PublishReceipt, error) {
	if query == nil {
		return PublishReceipt{}, BusinessError{Message: "消息 Outbox 事务查询不能为空"}
	}
	return s.publishWithQuery(query, envelope)
}

func (s *MiddlewarePlatformService) publishWithQuery(query contractsorm.Query, envelope MessageEnvelope) (PublishReceipt, error) {
	if envelope.MessageID == "" {
		envelope.MessageID = NewMessageEnvelope(envelope.MessageType, envelope.SchemaVersion, envelope.Payload).MessageID
	}
	if envelope.OccurredAt.IsZero() {
		envelope.OccurredAt = time.Now().UTC()
	}
	if envelope.PublishedAt.IsZero() {
		envelope.PublishedAt = time.Now().UTC()
	}
	if envelope.CorrelationID == "" {
		envelope.CorrelationID = envelope.MessageID
	}
	if err := ValidateMessageEnvelope(envelope); err != nil {
		return PublishReceipt{}, BusinessError{Message: err.Error()}
	}
	var route models.MessageRoute
	err := s.query().Table("message_route").
		Where("message_type", envelope.MessageType).
		Where("status", RouteStatusPublished).
		Where("enabled", true).
		OrderByDesc("published_at").
		First(&route)
	if err != nil {
		if err == frameworkerrors.OrmRecordNotFound {
			return PublishReceipt{}, BusinessError{Message: "消息类型没有已发布路由"}
		}
		return PublishReceipt{}, err
	}
	adapter, err := s.Adapter(route.AdapterID)
	if err != nil {
		return PublishReceipt{}, err
	}
	if !adapter.Enabled {
		return PublishReceipt{}, BusinessError{Message: "消息路由适配器已停用"}
	}
	dispatchPayload := messageDispatchPayload{RouteID: route.ID, AdapterID: adapter.ID, Envelope: envelope}
	encoded, err := json.Marshal(dispatchPayload)
	if err != nil {
		return PublishReceipt{}, err
	}
	event := queueservice.QueueOutboxEvent{
		Topic: "message.bus.publish", Connection: adapter.Connection, Queue: route.Destination,
		Payload: string(encoded), MessageID: envelope.MessageID, MessageType: envelope.MessageType,
		SchemaVersion: envelope.SchemaVersion, RouteID: route.ID, AdapterID: adapter.ID,
		Envelope: envelopeJSONMap(envelope), CorrelationID: envelope.CorrelationID, TenantID: envelope.TenantID,
	}
	if adapter.AdapterType == "memory" && query == nil {
		if err := dispatchMessageOutboxEvent(s.ctx, event); err != nil {
			return PublishReceipt{}, err
		}
		return PublishReceipt{MessageID: envelope.MessageID, RouteID: route.ID, Status: "DISPATCHED"}, nil
	}
	if query != nil {
		err = queueservice.EnqueueQueueOutboxEventWithQuery(query, event)
	} else {
		err = queueservice.EnqueueQueueOutboxEvent(s.ctx, event)
	}
	if err != nil {
		return PublishReceipt{}, err
	}
	return PublishReceipt{MessageID: envelope.MessageID, RouteID: route.ID, Status: "QUEUED"}, nil
}

func (s *MiddlewarePlatformService) Deliveries(filters map[string]string, page, pageSize int) (request.PageResult[models.MessageDelivery], error) {
	query := s.query().Table("message_delivery")
	query = query.Scopes(scopes.Equal("message_id", filters["message_id"]))
	query = query.Scopes(scopes.Equal("message_type", filters["message_type"]))
	query = query.Scopes(scopes.Equal("consumer_key", filters["consumer_key"]))
	query = query.Scopes(scopes.Equal("status", strings.ToUpper(filters["status"])))
	return request.Paginate[models.MessageDelivery](query.OrderByDesc("id"), page, pageSize)
}

func (s *MiddlewarePlatformService) DeadLetters(filters map[string]string, page, pageSize int) (request.PageResult[models.MessageDeadLetter], error) {
	query := s.query().Table("message_dead_letter").
		Select("id", "message_id", "message_type", "consumer_key", "route_id", "adapter_id",
			"failure_class", "error_summary", "first_failed_at", "last_failed_at", "replay_count",
			"resolution_status", "resolved_by", "resolved_at", "created_at", "updated_at")
	query = query.Scopes(scopes.Equal("message_id", filters["message_id"]))
	query = query.Scopes(scopes.Equal("message_type", filters["message_type"]))
	query = query.Scopes(scopes.Equal("consumer_key", filters["consumer_key"]))
	query = query.Scopes(scopes.Equal("failure_class", strings.ToUpper(filters["failure_class"])))
	query = query.Scopes(scopes.Equal("resolution_status", strings.ToUpper(filters["resolution_status"])))
	return request.Paginate[models.MessageDeadLetter](query.OrderByDesc("id"), page, pageSize)
}

func (s *MiddlewarePlatformService) DeadLetter(id uint64) (models.MessageDeadLetter, error) {
	var row models.MessageDeadLetter
	err := s.query().Table("message_dead_letter").Where("id", id).First(&row)
	if err != nil {
		if err == frameworkerrors.OrmRecordNotFound {
			return models.MessageDeadLetter{}, BusinessError{Message: "消息死信不存在"}
		}
		return models.MessageDeadLetter{}, err
	}
	return row, nil
}

func (s *MiddlewarePlatformService) ReplayDeadLetter(id uint64, idempotencyKey string) (ReplayReceipt, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return ReplayReceipt{}, BusinessError{Message: "死信重放幂等键不能为空"}
	}
	deadLetter, err := s.DeadLetter(id)
	if err != nil {
		return ReplayReceipt{}, err
	}
	envelope, err := deadLetterEnvelope(deadLetter)
	if err != nil {
		return ReplayReceipt{}, err
	}
	dispatch := messageDispatchPayload{
		RouteID: deadLetter.RouteID, AdapterID: deadLetter.AdapterID,
		Envelope: envelope, ReplayDeadLetterID: deadLetter.ID,
	}
	encoded, err := json.Marshal(dispatch)
	if err != nil {
		return ReplayReceipt{}, err
	}
	store := queueservice.NewDBQueueIdempotencyStore(PlatformConnection())
	var receipt ReplayReceipt
	_, err = store.Once(s.ctx, "message-replay:"+idempotencyKey, func(context.Context) (queueservice.QueueIdempotencyResult, error) {
		event := queueservice.QueueOutboxEvent{
			Topic: "message.bus.publish", Payload: string(encoded),
			MessageID: envelope.MessageID, MessageType: envelope.MessageType,
			SchemaVersion: envelope.SchemaVersion, RouteID: deadLetter.RouteID, AdapterID: deadLetter.AdapterID,
			Envelope: envelopeJSONMap(envelope), CorrelationID: envelope.CorrelationID, TenantID: envelope.TenantID,
		}
		route, routeErr := s.Route(deadLetter.RouteID)
		if routeErr != nil {
			return queueservice.QueueIdempotencyResult{}, routeErr
		}
		adapter, adapterErr := s.Adapter(deadLetter.AdapterID)
		if adapterErr != nil {
			return queueservice.QueueIdempotencyResult{}, adapterErr
		}
		if !adapter.Enabled {
			return queueservice.QueueIdempotencyResult{}, BusinessError{Message: "消息路由适配器已停用"}
		}
		event.Connection = adapter.Connection
		event.Queue = route.Destination
		if err := queueservice.EnqueueQueueOutboxEvent(s.ctx, event); err != nil {
			return queueservice.QueueIdempotencyResult{}, err
		}
		_, err := s.query().Table("message_dead_letter").Where("id", id).Update(map[string]any{
			"replay_count": deadLetter.ReplayCount + 1, "updated_at": time.Now(),
		})
		if err != nil {
			return queueservice.QueueIdempotencyResult{}, err
		}
		receipt = ReplayReceipt{DeadLetterID: id, MessageID: envelope.MessageID, Status: "QUEUED"}
		encodedReceipt, _ := json.Marshal(receipt)
		return queueservice.QueueIdempotencyResult{Status: queueservice.QueueIdempotencyStatusSuccess, Result: string(encodedReceipt)}, nil
	})
	if err != nil {
		return ReplayReceipt{}, err
	}
	if receipt.MessageID == "" {
		receipt = ReplayReceipt{DeadLetterID: id, MessageID: envelope.MessageID, Status: "ALREADY_QUEUED"}
	}
	return receipt, nil
}

func (s *MiddlewarePlatformService) ResolveDeadLetter(id, operatorID uint64) (models.MessageDeadLetter, error) {
	now := time.Now()
	result, err := s.query().Table("message_dead_letter").
		Where("id", id).
		Where("resolution_status", DeadLetterStatusOpen).
		Update(map[string]any{
			"resolution_status": DeadLetterStatusResolved, "resolved_by": operatorID,
			"resolved_at": now, "updated_at": now,
		})
	if err != nil {
		return models.MessageDeadLetter{}, err
	}
	if result.RowsAffected != 1 {
		return models.MessageDeadLetter{}, BusinessError{Message: "消息死信不存在或已解决"}
	}
	return s.DeadLetter(id)
}

func (s *MiddlewarePlatformService) query() contractsorm.Query {
	return s.orm().Query()
}

func (s *MiddlewarePlatformService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, PlatformConnection())
}

func (s *MiddlewarePlatformService) ensureAdapterNameAvailable(id uint64, name string) error {
	query := s.query().Table("middleware_adapter").Where("name", name)
	if id > 0 {
		query = query.Where("id != ?", id)
	}
	exists, err := query.Exists()
	if err != nil {
		return err
	}
	if exists {
		return BusinessError{Message: "中间件适配器名称已存在"}
	}
	return nil
}

func (s *MiddlewarePlatformService) ensureAdapterDisableConfirmed(id uint64, confirm bool) error {
	count, err := s.query().Table("message_route").
		Where("adapter_id", id).Where("status", RouteStatusPublished).Where("enabled", true).Count()
	if err != nil {
		return err
	}
	if count > 0 && !confirm {
		return BusinessError{Message: fmt.Sprintf("适配器仍被 %d 条已发布路由使用，确认影响后才能停用", count)}
	}
	return nil
}

func sanitizeAdapter(adapter models.MiddlewareAdapter) models.MiddlewareAdapter {
	adapter.Configured = strings.TrimSpace(adapter.ConfigEncrypted) != ""
	adapter.ConfigEncrypted = ""
	return adapter
}

func (p RoutePayload) route(enabledFallback bool) models.MessageRoute {
	mode := strings.ToUpper(strings.TrimSpace(p.ConsumptionMode))
	if mode == "" {
		mode = ConsumptionModeCluster
	}
	concurrency := p.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	enabled := enabledFallback
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	return models.MessageRoute{
		Name: strings.TrimSpace(p.Name), MessageType: strings.TrimSpace(p.MessageType),
		AdapterID: p.AdapterID, Destination: strings.TrimSpace(p.Destination),
		ConsumptionMode: mode, ConsumerGroup: strings.TrimSpace(p.ConsumerGroup),
		Concurrency: concurrency, OrderingEnabled: p.OrderingEnabled,
		RetryPolicy: p.RetryPolicy, DeadLetterPolicy: p.DeadLetterPolicy,
		Enabled: enabled, Version: p.Version,
	}
}

func retryPolicy(route models.MessageRoute) queueservice.QueueRetryPolicy {
	maxAttempts := jsonInt(route.RetryPolicy, "max_attempts", 4)
	initialSeconds := jsonInt(route.RetryPolicy, "initial_delay_seconds", 1)
	maxSeconds := jsonInt(route.RetryPolicy, "max_delay_seconds", 30)
	return queueservice.QueueRetryPolicy{
		MaxAttempts: maxAttempts, InitialDelay: time.Duration(initialSeconds) * time.Second,
		MaxDelay: time.Duration(maxSeconds) * time.Second,
	}
}

func jsonInt(value models.JSONMap, key string, fallback int) int {
	switch typed := value[key].(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed)
		}
	}
	return fallback
}

func envelopeJSONMap(envelope MessageEnvelope) models.JSONMap {
	encoded, _ := json.Marshal(envelope)
	var result models.JSONMap
	_ = json.Unmarshal(encoded, &result)
	return result
}

func envelopeFromJSONMap(value models.JSONMap) (MessageEnvelope, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return MessageEnvelope{}, err
	}
	var envelope MessageEnvelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		return MessageEnvelope{}, err
	}
	if err := ValidateMessageEnvelope(envelope); err != nil {
		return MessageEnvelope{}, fmt.Errorf("dead-letter envelope invalid: %w", err)
	}
	return envelope, nil
}

func updateMessageRouteJSON(
	query contractsorm.Query,
	id uint64,
	retryPolicy models.JSONMap,
	deadLetterPolicy models.JSONMap,
) error {
	retryJSON, err := json.Marshal(retryPolicy)
	if err != nil {
		return err
	}
	deadLetterJSON, err := json.Marshal(deadLetterPolicy)
	if err != nil {
		return err
	}
	_, err = query.Exec(
		`UPDATE message_route
		 SET retry_policy = ?::jsonb, dead_letter_policy = ?::jsonb
		 WHERE id = ?`,
		string(retryJSON), string(deadLetterJSON), id,
	)
	return err
}

func deadLetterEnvelope(deadLetter models.MessageDeadLetter) (MessageEnvelope, error) {
	if strings.TrimSpace(deadLetter.EnvelopeEncrypted) == "" {
		return envelopeFromJSONMap(deadLetter.Envelope)
	}
	plain, err := facades.Crypt().DecryptString(deadLetter.EnvelopeEncrypted)
	if err != nil {
		return MessageEnvelope{}, fmt.Errorf("dead-letter envelope decrypt failed: %w", err)
	}
	var envelope MessageEnvelope
	if err := json.Unmarshal([]byte(plain), &envelope); err != nil {
		return MessageEnvelope{}, fmt.Errorf("dead-letter envelope invalid: %w", err)
	}
	if err := ValidateMessageEnvelope(envelope); err != nil {
		return MessageEnvelope{}, fmt.Errorf("dead-letter envelope invalid: %w", err)
	}
	return envelope, nil
}
