package admin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	contractshttp "github.com/goravel/framework/contracts/testing/http"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/moduleboot"
	"goravel/app/services"
	"goravel/database/seeders"
	"goravel/tests/backend/testcase"
)

const (
	middlewareTestMessageType = "test.middleware.order-created"
	middlewareTestConsumerKey = "test.middleware.order-projector"
)

type MiddlewarePlatformTestSuite struct {
	suite.Suite
	tests.TestCase
	token        string
	consumerRuns int
}

func TestMiddlewarePlatformTestSuite(t *testing.T) {
	suite.Run(t, new(MiddlewarePlatformTestSuite))
}

func (s *MiddlewarePlatformTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.PlatformBootstrapSeeder{})
	s.token = s.loginAsMiddlewareAdmin()
	s.consumerRuns = 0
	services.UnregisterMessageConsumer(middlewareTestConsumerKey)
	services.UnregisterMessageType(middlewareTestMessageType)
	require.NoError(s.T(), services.RegisterMessageType(services.MessageTypeDefinition{
		MessageType: middlewareTestMessageType, Description: "测试订单事件",
		SupportedSchemaVersions: []int{1}, SensitiveFieldPaths: []string{"secret"},
		Validators: map[int]services.MessagePayloadValidator{
			1: func(payload models.JSONMap) error {
				if strings.TrimSpace(stringValue(payload["order_id"])) == "" {
					return errors.New("order_id is required")
				}
				return nil
			},
		},
	}))
	require.NoError(s.T(), services.RegisterMessageConsumer(services.ConsumerDefinition{
		ConsumerKey: middlewareTestConsumerKey, MessageType: middlewareTestMessageType,
		SupportedSchemaVersions: []int{1}, DefaultMode: services.ConsumptionModeCluster,
		Handler: func(_ context.Context, envelope services.MessageEnvelope) error {
			s.consumerRuns++
			if envelope.Payload["fail"] == true {
				return services.NonRetryableMessageError(errors.New("deterministic failure"))
			}
			return nil
		},
	}))
}

func (s *MiddlewarePlatformTestSuite) TearDownTest() {
	services.UnregisterMessageConsumer(middlewareTestConsumerKey)
	services.UnregisterMessageType(middlewareTestMessageType)
}

func (s *MiddlewarePlatformTestSuite) TestModuleArtifactsAndReliabilitySchemaExist() {
	registry := moduleboot.Modules()
	require.Contains(s.T(), registry.IDs(), "middleware-platform")
	require.Contains(s.T(), registry.OpenAPIFiles(), "docs/api-contract/openapi/middleware-platform.openapi.json")
	require.Contains(s.T(), registry.TestTemplates(), "tests/backend/feature/admin/middleware_platform_test.go")
	require.Contains(s.T(), registry.TestTemplates(), "tests/backend/unit/message_registry_test.go")

	for _, table := range []string{"middleware_adapter", "message_route", "message_delivery", "message_dead_letter"} {
		require.True(s.T(), facades.Schema().HasTable(table), table)
	}
	for _, table := range []string{"protection_rule_set", "protection_rule_version"} {
		require.True(s.T(), facades.Schema().HasTable(table), table)
	}
	for _, column := range []string{
		"message_id", "message_type", "schema_version", "route_id", "adapter_id", "envelope",
	} {
		require.True(s.T(), facades.Schema().HasColumn("queue_outbox", column), column)
	}
	for _, column := range []string{"consumer_key", "idempotency_key", "message_id"} {
		require.True(s.T(), facades.Schema().HasColumn("queue_idempotency", column), column)
	}
	require.True(s.T(), facades.Schema().HasColumn("message_dead_letter", "envelope_encrypted"))
}

func (s *MiddlewarePlatformTestSuite) TestProtectionRulePublishDraftIsolationAndRollbackAppendVersion() {
	create := s.postJSON("/admin/platform/middleware/protection-rules", `{
		"name": "订单接口保护",
		"scope": "ENDPOINT",
		"resource_pattern": "/orders/create",
		"rules": {"rules": [
			{"type": "RATE_LIMIT", "limit": 1, "window_ms": 60000},
			{"type": "CONCURRENCY", "max_concurrency": 2}
		]}
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	created := create["data"].(map[string]any)
	require.Equal(s.T(), services.ProtectionRuleStatusDraft, created["status"])
	id := uint64(created["id"].(float64))

	published := s.postJSONWithHeaders(
		"/admin/platform/middleware/protection-rules/"+itoa(id)+"/publish",
		`{"version":1}`,
		map[string]string{"Idempotency-Key": "publish-protection-order-v1"},
	)
	require.Equal(s.T(), float64(200), published["code"])
	publishedData := published["data"].(map[string]any)
	require.Equal(s.T(), services.ProtectionRuleStatusPublished, publishedData["status"])
	require.Equal(s.T(), float64(1), publishedData["published_version"])
	require.Equal(s.T(), float64(2), publishedData["version"])

	duplicatePublish := s.postJSONWithHeaders(
		"/admin/platform/middleware/protection-rules/"+itoa(id)+"/publish",
		`{"version":1}`,
		map[string]string{"Idempotency-Key": "publish-protection-order-v1"},
	)
	require.Equal(s.T(), float64(200), duplicatePublish["code"])
	require.Equal(s.T(), publishedData, duplicatePublish["data"])

	publishedVersionCount, err := facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("protection_rule_version").Where("rule_set_id", id).Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), publishedVersionCount)

	protection := services.NewProtectionRuleSetService().WithContext(s.T().Context())
	first, err := protection.Evaluate("/orders/create", services.ProtectionRequestContext{
		Endpoint: "/orders/create", RateLimitKey: "tenant-a",
	})
	require.NoError(s.T(), err)
	require.True(s.T(), first.Allowed)
	second, err := protection.Evaluate("/orders/create", services.ProtectionRequestContext{
		Endpoint: "/orders/create", RateLimitKey: "tenant-a",
	})
	require.NoError(s.T(), err)
	require.False(s.T(), second.Allowed)
	require.Equal(s.T(), services.ProtectionRejectionRateLimited, second.Rejection)

	update := s.putJSON("/admin/platform/middleware/protection-rules/"+itoa(id), `{
		"name": "订单接口保护草稿",
		"scope": "ENDPOINT",
		"resource_pattern": "/orders/create",
		"rules": {"rules": [
			{"type": "RATE_LIMIT", "limit": 100, "window_ms": 60000}
		]},
		"version": 2
	}`)
	require.Equal(s.T(), float64(200), update["code"])
	require.Equal(s.T(), services.ProtectionRuleStatusDraft, update["data"].(map[string]any)["status"])

	services.ResetProtectionRuntimeForTest()
	stillPublished, err := protection.Evaluate("/orders/create", services.ProtectionRequestContext{
		Endpoint: "/orders/create", RateLimitKey: "tenant-b",
	})
	require.NoError(s.T(), err)
	require.True(s.T(), stillPublished.Allowed)
	stillLimited, err := protection.Evaluate("/orders/create", services.ProtectionRequestContext{
		Endpoint: "/orders/create", RateLimitKey: "tenant-b",
	})
	require.NoError(s.T(), err)
	require.False(s.T(), stillLimited.Allowed)

	secondPublish := s.postJSONWithHeaders(
		"/admin/platform/middleware/protection-rules/"+itoa(id)+"/publish",
		`{"version":3}`,
		map[string]string{"Idempotency-Key": "publish-protection-order-v2"},
	)
	require.Equal(s.T(), float64(200), secondPublish["code"])
	require.Equal(s.T(), float64(2), secondPublish["data"].(map[string]any)["published_version"])

	rollback := s.postJSON(
		"/admin/platform/middleware/protection-rules/"+itoa(id)+"/rollback",
		`{"version":4,"target_version":1}`,
	)
	require.Equal(s.T(), float64(200), rollback["code"])
	rollbackData := rollback["data"].(map[string]any)
	require.Equal(s.T(), float64(3), rollbackData["published_version"])
	require.Equal(s.T(), float64(5), rollbackData["version"])

	versions := s.getJSON("/admin/platform/middleware/protection-rules/" + itoa(id) + "/versions")
	require.Equal(s.T(), float64(200), versions["code"])
	versionRows := versions["data"].([]any)
	require.Len(s.T(), versionRows, 3)
	require.Equal(s.T(), float64(3), versionRows[0].(map[string]any)["version"])
	require.Equal(s.T(), float64(2), versionRows[1].(map[string]any)["version"])
	require.Equal(s.T(), float64(1), versionRows[2].(map[string]any)["version"])
	require.Equal(
		s.T(),
		versionRows[0].(map[string]any)["rules"],
		versionRows[2].(map[string]any)["rules"],
	)
}

func (s *MiddlewarePlatformTestSuite) TestRouteLifecycleDefaultsEnabledAndRejectsStaleVersion() {
	adapterID := s.databaseAdapterID()
	create := s.postJSON("/admin/platform/middleware/routes", `{
		"name": "订单投影",
		"message_type": "`+middlewareTestMessageType+`",
		"adapter_id": `+itoa(adapterID)+`,
		"destination": "middleware-test",
		"consumption_mode": "CLUSTER",
		"consumer_group": "middleware-test-group",
		"concurrency": 2,
		"retry_policy": {"max_attempts": 2}
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	route := create["data"].(map[string]any)
	require.Equal(s.T(), true, route["enabled"])
	require.Equal(s.T(), services.RouteStatusDraft, route["status"])

	id := uint64(route["id"].(float64))
	missingKey := s.postJSON("/admin/platform/middleware/routes/"+itoa(id)+"/publish", `{"version":1}`)
	require.Equal(s.T(), float64(422), missingKey["code"])
	require.Contains(s.T(), missingKey["message"], "Idempotency-Key")

	publish := s.postJSONWithHeaders(
		"/admin/platform/middleware/routes/"+itoa(id)+"/publish",
		`{"version":1}`,
		map[string]string{"Idempotency-Key": "publish-route-order-v1"},
	)
	require.Equal(s.T(), float64(200), publish["code"])
	require.Equal(s.T(), services.RouteStatusPublished, publish["data"].(map[string]any)["status"])
	require.Equal(s.T(), float64(2), publish["data"].(map[string]any)["version"])

	duplicatePublish := s.postJSONWithHeaders(
		"/admin/platform/middleware/routes/"+itoa(id)+"/publish",
		`{"version":1}`,
		map[string]string{"Idempotency-Key": "publish-route-order-v1"},
	)
	require.Equal(s.T(), float64(200), duplicatePublish["code"])
	require.Equal(s.T(), publish["data"], duplicatePublish["data"])

	var persistedRoute models.MessageRoute
	require.NoError(s.T(), facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("message_route").Where("id", id).First(&persistedRoute))
	require.Equal(s.T(), 2, persistedRoute.Version)

	stale := s.putJSON("/admin/platform/middleware/routes/"+itoa(id), `{
		"name": "过期更新",
		"message_type": "`+middlewareTestMessageType+`",
		"adapter_id": `+itoa(adapterID)+`,
		"destination": "middleware-test",
		"consumption_mode": "CLUSTER",
		"consumer_group": "middleware-test-group",
		"concurrency": 1,
		"version": 1
	}`)
	require.Equal(s.T(), float64(422), stale["code"])
	require.Contains(s.T(), stale["message"], "版本冲突")
}

func (s *MiddlewarePlatformTestSuite) TestProtectionRulePublishRequiresIdempotencyKey() {
	create := s.postJSON("/admin/platform/middleware/protection-rules", `{
		"name": "缺少幂等键规则",
		"scope": "ENDPOINT",
		"resource_pattern": "/orders/missing-key",
		"rules": {"rules": [
			{"type": "RATE_LIMIT", "limit": 1, "window_ms": 60000}
		]}
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	id := uint64(create["data"].(map[string]any)["id"].(float64))

	publish := s.postJSON(
		"/admin/platform/middleware/protection-rules/"+itoa(id)+"/publish",
		`{"version":1}`,
	)
	require.Equal(s.T(), float64(422), publish["code"])
	require.Contains(s.T(), publish["message"], "Idempotency-Key")
}

func (s *MiddlewarePlatformTestSuite) TestMemoryAdapterExplicitlyRejectsClusterMode() {
	adapterID := s.adapterIDByType("memory")
	create := s.postJSON("/admin/platform/middleware/routes", `{
		"name": "错误内存集群路由",
		"message_type": "`+middlewareTestMessageType+`",
		"adapter_id": `+itoa(adapterID)+`,
		"destination": "middleware-memory-test",
		"consumption_mode": "CLUSTER",
		"consumer_group": "middleware-test-group",
		"concurrency": 1
	}`)
	require.Equal(s.T(), float64(422), create["code"])
	require.Contains(s.T(), create["message"], "不支持集群消费")
}

func (s *MiddlewarePlatformTestSuite) TestAdapterManagementRequiresDisableConfirmationAndRejectsDynamicConfig() {
	_, adapterID := s.createPublishedDatabaseRoute("适配器停用确认")
	detail := s.getJSON("/admin/platform/middleware/adapters/" + itoa(adapterID))
	require.Equal(s.T(), float64(200), detail["code"])
	adapter := detail["data"].(map[string]any)
	require.NotContains(s.T(), adapter, "config_encrypted")
	require.Equal(s.T(), "database", adapter["connection"])

	testConnection := s.postJSON(
		"/admin/platform/middleware/adapters/"+itoa(adapterID)+"/test",
		`{}`,
	)
	require.Equal(s.T(), float64(200), testConnection["code"])
	require.Equal(s.T(), "UP", testConnection["data"].(map[string]any)["status"])

	unconfirmed := s.putJSON(
		"/admin/platform/middleware/adapters/"+itoa(adapterID)+"/disable",
		`{"version":1}`,
	)
	require.Equal(s.T(), float64(422), unconfirmed["code"])
	require.Contains(s.T(), unconfirmed["message"], "确认影响")

	disabled := s.putJSON(
		"/admin/platform/middleware/adapters/"+itoa(adapterID)+"/disable",
		`{"version":1,"confirm":true}`,
	)
	require.Equal(s.T(), float64(200), disabled["code"])
	require.Equal(s.T(), false, disabled["data"].(map[string]any)["enabled"])
	require.Equal(s.T(), float64(2), disabled["data"].(map[string]any)["version"])

	replaceConfig := s.putJSON(
		"/admin/platform/middleware/adapters/"+itoa(adapterID)+"/config",
		`{"version":2}`,
	)
	require.Equal(s.T(), float64(422), replaceConfig["code"])
	require.Contains(s.T(), replaceConfig["message"], "不支持在线替换")

	metrics := s.getJSON("/admin/platform/middleware/metrics")
	require.Equal(s.T(), float64(200), metrics["code"])
	metricData := metrics["data"].(map[string]any)
	require.Contains(s.T(), metricData, "message")
	require.Contains(s.T(), metricData, "outbox")
	require.Contains(s.T(), metricData, "protection")
}

func (s *MiddlewarePlatformTestSuite) TestDisabledAdapterRejectsPublishAndQueuedDispatch() {
	routeID, adapterID := s.createPublishedDatabaseRoute("停用适配器投递")
	envelope := services.NewMessageEnvelope(middlewareTestMessageType, 1, models.JSONMap{
		"order_id": "order-disabled-adapter",
	})
	receipt, err := services.NewMiddlewarePlatformService().Publish(envelope)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "QUEUED", receipt.Status)

	adapter, err := services.NewMiddlewarePlatformService().Adapter(adapterID)
	require.NoError(s.T(), err)
	_, err = services.NewMiddlewarePlatformService().SetAdapterEnabled(adapterID, false, adapter.Version, true, 1)
	require.NoError(s.T(), err)

	_, err = services.NewMiddlewarePlatformService().Publish(
		services.NewMessageEnvelope(middlewareTestMessageType, 1, models.JSONMap{
			"order_id": "order-after-disable",
		}),
	)
	require.ErrorContains(s.T(), err, "已停用")

	var outbox services.QueueOutboxEvent
	require.NoError(s.T(), facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("queue_outbox").Where("message_id", envelope.MessageID).First(&outbox))
	err = (&services.QueueOutboxDispatchJob{}).Handle(
		outbox.ID,
		outbox.Topic,
		outbox.Connection,
		outbox.Queue,
		outbox.Payload,
	)
	require.ErrorContains(s.T(), err, "disabled")
	require.Zero(s.T(), s.consumerRuns)
	require.Equal(s.T(), routeID, outbox.RouteID)
}

func (s *MiddlewarePlatformTestSuite) TestPublishPersistsEnvelopeAndConsumptionInboxMetadata() {
	routeID, adapterID := s.createPublishedDatabaseRoute("订单可靠投递")
	envelope := services.NewMessageEnvelope(middlewareTestMessageType, 1, models.JSONMap{
		"order_id": "order-42", "secret": "top-secret",
	})
	receipt, err := services.NewMiddlewarePlatformService().Publish(envelope)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "QUEUED", receipt.Status)
	require.Equal(s.T(), routeID, receipt.RouteID)

	var outbox services.QueueOutboxEvent
	require.NoError(s.T(), facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("queue_outbox").Where("message_id", envelope.MessageID).First(&outbox))
	require.Equal(s.T(), middlewareTestMessageType, outbox.MessageType)
	require.Equal(s.T(), routeID, outbox.RouteID)
	require.Equal(s.T(), adapterID, outbox.AdapterID)
	require.Equal(s.T(), "order-42", outbox.Envelope["payload"].(map[string]any)["order_id"])

	dispatch := map[string]any{"route_id": routeID, "adapter_id": adapterID, "envelope": envelope}
	encoded, err := json.Marshal(dispatch)
	require.NoError(s.T(), err)
	job := &services.MessageConsumeJob{}
	require.NoError(s.T(), job.Handle(string(encoded), middlewareTestConsumerKey, uint64(1)))
	require.NoError(s.T(), job.Handle(string(encoded), middlewareTestConsumerKey, uint64(1)))
	require.Equal(s.T(), 1, s.consumerRuns)

	var inbox struct {
		ConsumerKey    string `gorm:"column:consumer_key"`
		IdempotencyKey string `gorm:"column:idempotency_key"`
		MessageID      string `gorm:"column:message_id"`
	}
	require.NoError(s.T(), facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("queue_idempotency").Where("consumer_key", middlewareTestConsumerKey).First(&inbox))
	require.Equal(s.T(), middlewareTestConsumerKey, inbox.ConsumerKey)
	require.Equal(s.T(), envelope.MessageID, inbox.IdempotencyKey)
	require.Equal(s.T(), envelope.MessageID, inbox.MessageID)

	var statuses []string
	require.NoError(s.T(), facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("message_delivery").Where("message_id", envelope.MessageID).OrderBy("id").Pluck("status", &statuses))
	require.Equal(s.T(), []string{"SUCCEEDED", "IGNORED"}, statuses)
}

func (s *MiddlewarePlatformTestSuite) TestDeadLetterIsRedactedButReplayUsesEncryptedOriginalEnvelope() {
	routeID, adapterID := s.createPublishedDatabaseRoute("订单死信")
	envelope := services.NewMessageEnvelope(middlewareTestMessageType, 1, models.JSONMap{
		"order_id": "order-dead", "secret": "top-secret", "fail": true,
	})
	dispatch := map[string]any{"route_id": routeID, "adapter_id": adapterID, "envelope": envelope}
	encoded, err := json.Marshal(dispatch)
	require.NoError(s.T(), err)
	require.NoError(s.T(), (&services.MessageConsumeJob{}).Handle(string(encoded), middlewareTestConsumerKey, uint64(1)))

	var deadLetter models.MessageDeadLetter
	require.NoError(s.T(), facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("message_dead_letter").Where("message_id", envelope.MessageID).First(&deadLetter))
	require.Equal(s.T(), "[REDACTED]", deadLetter.Envelope["payload"].(map[string]any)["secret"])
	require.NotEmpty(s.T(), deadLetter.EnvelopeEncrypted)

	detail := s.getJSON("/admin/platform/middleware/dead-letters/" + itoa(deadLetter.ID))
	require.Equal(s.T(), float64(200), detail["code"])
	detailData := detail["data"].(map[string]any)
	require.NotContains(s.T(), detailData, "envelope_encrypted")
	require.Equal(s.T(), "[REDACTED]", detailData["envelope"].(map[string]any)["payload"].(map[string]any)["secret"])

	replay := s.postJSONWithHeaders(
		"/admin/platform/middleware/dead-letters/"+itoa(deadLetter.ID)+"/replay",
		`{}`,
		map[string]string{"Idempotency-Key": "replay-order-dead"},
	)
	require.Equal(s.T(), float64(200), replay["code"])
	require.Equal(s.T(), "QUEUED", replay["data"].(map[string]any)["status"])

	var replayOutbox services.QueueOutboxEvent
	require.NoError(s.T(), facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("queue_outbox").Where("message_id", envelope.MessageID).OrderByDesc("id").First(&replayOutbox))
	var replayPayload struct {
		Envelope services.MessageEnvelope `json:"envelope"`
	}
	require.NoError(s.T(), json.Unmarshal([]byte(replayOutbox.Payload), &replayPayload))
	require.Equal(s.T(), "top-secret", replayPayload.Envelope.Payload["secret"])

	duplicate := s.postJSONWithHeaders(
		"/admin/platform/middleware/dead-letters/"+itoa(deadLetter.ID)+"/replay",
		`{}`,
		map[string]string{"Idempotency-Key": "replay-order-dead"},
	)
	require.Equal(s.T(), float64(200), duplicate["code"])
	require.Equal(s.T(), "ALREADY_QUEUED", duplicate["data"].(map[string]any)["status"])
}

func (s *MiddlewarePlatformTestSuite) createPublishedDatabaseRoute(name string) (uint64, uint64) {
	adapterID := s.databaseAdapterID()
	route, err := services.NewMiddlewarePlatformService().CreateRoute(services.RoutePayload{
		Name: name, MessageType: middlewareTestMessageType, AdapterID: adapterID,
		Destination: "middleware-test", ConsumptionMode: services.ConsumptionModeCluster,
		ConsumerGroup: "middleware-test-group", Concurrency: 1,
	}, 1)
	require.NoError(s.T(), err)
	route, err = services.NewMiddlewarePlatformService().PublishRoute(route.ID, route.Version, 1)
	require.NoError(s.T(), err)
	return route.ID, adapterID
}

func (s *MiddlewarePlatformTestSuite) databaseAdapterID() uint64 {
	return s.adapterIDByType("goravel_queue")
}

func (s *MiddlewarePlatformTestSuite) adapterIDByType(adapterType string) uint64 {
	adapters, err := services.NewMiddlewarePlatformService().Adapters()
	require.NoError(s.T(), err)
	for _, adapter := range adapters {
		if adapter.AdapterType == adapterType {
			return adapter.ID
		}
	}
	s.T().Fatalf("adapter type %s not found", adapterType)
	return 0
}

func (s *MiddlewarePlatformTestSuite) loginAsMiddlewareAdmin() string {
	result, err := services.NewPlatformPassportService().Login("admin", "123456")
	require.NoError(s.T(), err)
	return result.AccessToken
}

func (s *MiddlewarePlatformTestSuite) getJSON(path string) map[string]any {
	res, err := s.Http(s.T()).WithToken(s.token).Get(path)
	return s.jsonMap(res, err)
}

func (s *MiddlewarePlatformTestSuite) postJSON(path, body string) map[string]any {
	return s.postJSONWithHeaders(path, body, nil)
}

func (s *MiddlewarePlatformTestSuite) postJSONWithHeaders(path, body string, headers map[string]string) map[string]any {
	request := s.Http(s.T()).WithToken(s.token)
	if len(headers) > 0 {
		request = request.WithHeaders(headers)
	}
	res, err := request.Post(path, strings.NewReader(body))
	return s.jsonMap(res, err)
}

func (s *MiddlewarePlatformTestSuite) putJSON(path, body string) map[string]any {
	res, err := s.Http(s.T()).WithToken(s.token).Put(path, strings.NewReader(body))
	return s.jsonMap(res, err)
}

func (s *MiddlewarePlatformTestSuite) jsonMap(res contractshttp.Response, err error) map[string]any {
	require.NoError(s.T(), err)
	res.AssertOk()
	payload, err := res.Json()
	require.NoError(s.T(), err)
	return payload
}

func stringValue(value any) string {
	typed, _ := value.(string)
	return typed
}
