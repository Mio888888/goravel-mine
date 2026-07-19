package application

import (
	"context"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"github.com/goravel/framework/contracts/foundation"
	"github.com/goravel/framework/contracts/queue"
	"goravel/app/models"
	dictionaryservice "goravel/app/services/platform/dictionary"
	logservice "goravel/app/services/platform/logadmin"
	orgservice "goravel/app/services/platform/org"
	referencecaseservice "goravel/app/services/platform/referencecase"
	storageservice "goravel/app/services/platform/storage"
	migrationlockservice "goravel/app/services/runtime/migrationlock"
	observabilityservice "goravel/app/services/runtime/observability"
	queueservice "goravel/app/services/runtime/queue"
	scheduledtaskservice "goravel/app/services/runtime/scheduledtask"
	securityrotationservice "goravel/app/services/runtime/securityrotation"
	"time"
)

// Source: dictionary.go
const (
	DictStatusEnabled  = dictionaryservice.DictStatusEnabled
	DictStatusDisabled = dictionaryservice.DictStatusDisabled
)

type PlatformDictType = dictionaryservice.PlatformDictType
type PlatformDictItem = dictionaryservice.PlatformDictItem
type DictType = dictionaryservice.DictType
type DictItem = dictionaryservice.DictItem
type PlatformDictTypePayload = dictionaryservice.PlatformDictTypePayload
type DictItemPayload = dictionaryservice.DictItemPayload
type TenantDictTypePayload = dictionaryservice.TenantDictTypePayload
type TenantDictItemPayload = dictionaryservice.TenantDictItemPayload
type DictOption = dictionaryservice.DictOption
type TenantDictionaryService = dictionaryservice.TenantDictionaryService
type PlatformDictionaryService = dictionaryservice.PlatformDictionaryService

func init() {
	dictionaryservice.ConfigureDependencies(
		OrmForConnectionWithContext,
		PlatformConnection,
		RegisterTenantConnection,
	)
}

func NewPlatformDictionaryService() *PlatformDictionaryService {
	return dictionaryservice.NewPlatformDictionaryService()
}

func NewTenantDictionaryServiceForTenant(tenant Tenant) *TenantDictionaryService {
	return dictionaryservice.NewTenantDictionaryServiceForTenant(tenant)
}

// Source: jobs.go
func QueueJobs() []queue.Job {
	return []queue.Job{
		&OperationLogJob{},
		&QueueOutboxDispatchJob{},
	}
}

// Source: log.go
type LogAdminService = logservice.LogAdminService
type LoginLogRow = logservice.LoginLogRow
type OperationLogRow = logservice.OperationLogRow
type OperationLogPayload = logservice.OperationLogPayload
type OperationLogJob = logservice.OperationLogJob
type OperationLogRunner = logservice.OperationLogRunner

func init() {
	logservice.ConfigureORMFactory(OrmForConnectionWithContext)
}

func NewLogAdminService() *LogAdminService {
	return logservice.NewService("", OrmForConnectionWithContext)
}

func NewLogAdminServiceForTenant(tenant Tenant) *LogAdminService {
	return logservice.NewService(TenantConnectionName(tenant), OrmForConnectionWithContext)
}

func NewLogAdminServiceForConnection(connection string) *LogAdminService {
	return logservice.NewService(connection, OrmForConnectionWithContext)
}

func DispatchOperationLog(payload OperationLogPayload) {
	logservice.DispatchOperationLog(payload)
}

func NewOperationLogRunner() foundation.Runner {
	return logservice.NewOperationLogRunner()
}

func formatLogTime(value time.Time) string {
	return logservice.FormatTime(value)
}

// Source: migration_lock.go
const (
	MigrationScopePlatform = migrationlockservice.MigrationScopePlatform
	MigrationScopeTenants  = migrationlockservice.MigrationScopeTenants
	MigrationScopeAll      = migrationlockservice.MigrationScopeAll
)

var (
	ErrMigrationScope       = migrationlockservice.ErrMigrationScope
	ErrMigrationLockTimeout = migrationlockservice.ErrMigrationLockTimeout
)

type MigrationScope = migrationlockservice.MigrationScope
type MigrationLockMetadata = migrationlockservice.MigrationLockMetadata
type MigrationLock = migrationlockservice.MigrationLock
type MigrationLockProvider = migrationlockservice.MigrationLockProvider
type MigrationLockMetrics = migrationlockservice.MigrationLockMetrics
type MigrationLockService = migrationlockservice.MigrationLockService

var MigrationLockMetricsSnapshot = migrationlockservice.MetricsSnapshot
var ResetMigrationLockMetricsForTest = migrationlockservice.ResetMetricsForTest
var ParseMigrationScope = migrationlockservice.ParseMigrationScope

func init() {
	migrationlockservice.ConfigureDependencies(
		OrmForConnectionWithContext,
		PlatformConnection,
		RecordAuditEvent,
	)
}

func NewMigrationLockService() *MigrationLockService {
	return migrationlockservice.NewMigrationLockService()
}

// Source: observability.go
const (
	AuditOutcomeSuccess = observabilityservice.AuditOutcomeSuccess
	AuditOutcomeFailure = observabilityservice.AuditOutcomeFailure
)

type AuditEvent = observabilityservice.AuditEvent
type HTTPObservation = observabilityservice.HTTPObservation
type SlowRequest = observabilityservice.SlowRequest
type MetricsSnapshot = observabilityservice.MetricsSnapshot
type RouteMetric = observabilityservice.RouteMetric
type RouteDurationBucket = observabilityservice.RouteDurationBucket
type GoRuntimeMetric = observabilityservice.GoRuntimeMetric
type DBPoolMetric = observabilityservice.DBPoolMetric
type ObservabilityRecorder = observabilityservice.ObservabilityRecorder
type SlowSQL = observabilityservice.SlowSQL
type SlowSQLSnapshot = observabilityservice.SlowSQLSnapshot
type ObservabilityLogDriver = observabilityservice.ObservabilityLogDriver

var WithRequestID = observabilityservice.WithRequestID
var RequestID = observabilityservice.RequestID
var WithTraceID = observabilityservice.WithTraceID
var TraceID = observabilityservice.TraceID
var NewTraceID = observabilityservice.NewTraceID
var NewRequestID = observabilityservice.NewRequestID
var NormalizeObservabilityID = observabilityservice.NormalizeObservabilityID
var WithAuditOutcome = observabilityservice.WithAuditOutcome
var AuditOutcome = observabilityservice.AuditOutcome
var RecordAuditEvent = observabilityservice.RecordAuditEvent
var RecordTenantGovernanceEvent = observabilityservice.RecordTenantGovernanceEvent
var NewObservabilityRecorder = observabilityservice.NewObservabilityRecorder
var ConfigureObservabilityRecorder = observabilityservice.ConfigureObservabilityRecorder
var RecordHTTPObservation = observabilityservice.RecordHTTPObservation
var RecordHTTPObservationStart = observabilityservice.RecordHTTPObservationStart
var RecordHTTPObservationFinish = observabilityservice.RecordHTTPObservationFinish
var ResetObservabilityMetricsForTest = observabilityservice.ResetObservabilityMetricsForTest
var ConfigureSlowSQLRecorder = observabilityservice.ConfigureSlowSQLRecorder
var RecordSlowSQLFromMessage = observabilityservice.RecordSlowSQLFromMessage
var RecordSlowSQLFromContext = observabilityservice.RecordSlowSQLFromContext
var SlowSQLMetrics = observabilityservice.SlowSQLMetrics
var SlowSQLMetricsSnapshot = observabilityservice.SlowSQLMetricsSnapshot
var ResetSlowSQLMetricsForTest = observabilityservice.ResetSlowSQLMetricsForTest
var PrometheusMetricsText = observabilityservice.PrometheusMetricsText
var MetricsSummary = observabilityservice.MetricsSummary
var GoRuntimeMetrics = observabilityservice.GoRuntimeMetrics

func DBPoolMetrics() DBPoolMetric {
	return observabilityservice.DBPoolMetrics(PlatformConnection())
}

func ObservabilityMetrics() MetricsSnapshot {
	snapshot := observabilityservice.Metrics()
	snapshot.DBPool = DBPoolMetrics()
	snapshot.SchedulerNodes = SchedulerHeartbeatSnapshot(time.Now()).SchedulerNodes
	snapshot.Queue = QueueBacklogMetrics(context.Background())
	snapshot.CasbinCache = CasbinEnforcerCacheSnapshot()
	snapshot.TenantConnections = TenantConnectionCapacitySnapshot()
	snapshot.MigrationLocks = MigrationLockMetricsSnapshot()
	if governance, err := TenantGovernanceObservabilitySnapshot(context.Background(), time.Now()); err == nil {
		snapshot.TenantGovernance = governance
	}
	return snapshot
}

// Source: org.go
type OrgAdminService = orgservice.OrgAdminService
type DepartmentPayload = orgservice.DepartmentPayload
type DepartmentRow = orgservice.DepartmentRow
type DepartmentUser = orgservice.DepartmentUser
type PositionPayload = orgservice.PositionPayload
type PositionRow = orgservice.PositionRow
type PositionPolicy = orgservice.PositionPolicy
type LeaderPayload = orgservice.LeaderPayload
type LeaderRow = orgservice.LeaderRow

func NewOrgAdminService() *OrgAdminService {
	return orgservice.NewAdminService("", OrmForConnectionWithContext)
}

func NewOrgAdminServiceForTenant(tenant Tenant) *OrgAdminService {
	return orgservice.NewAdminService(TenantConnectionName(tenant), OrmForConnectionWithContext)
}

// Source: queue.go
const (
	QueueOutboxStatusPending    = queueservice.QueueOutboxStatusPending
	QueueOutboxStatusProcessing = queueservice.QueueOutboxStatusProcessing
	QueueOutboxStatusSent       = queueservice.QueueOutboxStatusSent
	QueueOutboxStatusFailed     = queueservice.QueueOutboxStatusFailed

	QueueIdempotencyStatusRunning = queueservice.QueueIdempotencyStatusRunning
	QueueIdempotencyStatusSuccess = queueservice.QueueIdempotencyStatusSuccess
	QueueIdempotencyStatusFailed  = queueservice.QueueIdempotencyStatusFailed
)

type QueueFailedJobFilters = queueservice.QueueFailedJobFilters
type QueueFailedJobRow = queueservice.QueueFailedJobRow
type QueueFailedJobRetryResult = queueservice.QueueFailedJobRetryResult
type QueueFailedJobDeleteResult = queueservice.QueueFailedJobDeleteResult
type QueueFailedJobService = queueservice.QueueFailedJobService
type QueueIdempotencyResult = queueservice.QueueIdempotencyResult
type QueueIdempotencyStore = queueservice.QueueIdempotencyStore
type MemoryQueueIdempotencyStore = queueservice.MemoryQueueIdempotencyStore
type DBQueueIdempotencyStore = queueservice.DBQueueIdempotencyStore
type QueueBacklogMetric = queueservice.QueueBacklogMetric
type QueueClassMetric = queueservice.QueueClassMetric
type QueueOutboxEvent = queueservice.QueueOutboxEvent
type QueueOutboxHandler = queueservice.QueueOutboxHandler
type QueueOutboxStore = queueservice.QueueOutboxStore
type QueueOutboxDispatchResult = queueservice.QueueOutboxDispatchResult
type QueueOutboxDispatcher = queueservice.QueueOutboxDispatcher
type DBQueueOutboxStore = queueservice.DBQueueOutboxStore
type MemoryQueueOutboxStore = queueservice.MemoryQueueOutboxStore
type QueueOutboxDispatchJob = queueservice.QueueOutboxDispatchJob
type QueueOutboxRunner = queueservice.QueueOutboxRunner
type QueueRetryPolicy = queueservice.QueueRetryPolicy

type queueFailer = queueservice.QueueFailer
type queueFailedJob = queueservice.QueueFailedJob

func init() {
	queueservice.ConfigureORMFactory(OrmForConnectionWithContext)
}

func NewQueueFailedJobService() *QueueFailedJobService {
	return queueservice.NewQueueFailedJobService()
}

func NewQueueFailedJobServiceWithFailer(failer queueFailer) *QueueFailedJobService {
	return queueservice.NewQueueFailedJobServiceWithFailer(failer)
}

func NewMemoryQueueIdempotencyStore() *MemoryQueueIdempotencyStore {
	return queueservice.NewMemoryQueueIdempotencyStore()
}

func NewDBQueueIdempotencyStore(connection string) *DBQueueIdempotencyStore {
	return queueservice.NewDBQueueIdempotencyStore(connection)
}

func QueueBacklogMetrics(ctx context.Context) QueueBacklogMetric {
	return queueservice.QueueBacklogMetrics(ctx)
}

func QueueClassMetricsFromOutbox(events []QueueOutboxEvent, now time.Time) []QueueClassMetric {
	return queueservice.QueueClassMetricsFromOutbox(events, now)
}

func EnqueueQueueOutboxEvent(ctx context.Context, event QueueOutboxEvent) error {
	return queueservice.EnqueueQueueOutboxEvent(ctx, event)
}

func EnqueueQueueOutboxEventWithQuery(query contractsorm.Query, event QueueOutboxEvent) error {
	return queueservice.EnqueueQueueOutboxEventWithQuery(query, event)
}

func NewDBQueueOutboxStore(connection string) *DBQueueOutboxStore {
	return queueservice.NewDBQueueOutboxStore(connection)
}

func NewMemoryQueueOutboxStore(events []*QueueOutboxEvent) *MemoryQueueOutboxStore {
	return queueservice.NewMemoryQueueOutboxStore(events)
}

func RegisterQueueOutboxHandler(topic string, handler QueueOutboxHandler) {
	queueservice.RegisterQueueOutboxHandler(topic, handler)
}

func UnregisterQueueOutboxHandler(topic string) {
	queueservice.UnregisterQueueOutboxHandler(topic)
}

func DispatchQueueOutboxEvent(ctx context.Context, event QueueOutboxEvent) error {
	return queueservice.DispatchQueueOutboxEvent(ctx, event)
}

func NewQueueOutboxRunner() foundation.Runner {
	return queueservice.NewQueueOutboxRunner()
}

func ShouldRunQueueOutboxRunner(outboxEnabled, queueWorkerEnabled bool, connection string) bool {
	return queueservice.ShouldRunQueueOutboxRunner(outboxEnabled, queueWorkerEnabled, connection)
}

func RunQueueOutboxDispatchOnceForTest(ctx context.Context, dispatcher QueueOutboxDispatcher, owner string, batch int, logger func(string, map[string]any)) {
	queueservice.RunQueueOutboxDispatchOnceForTest(ctx, dispatcher, owner, batch, logger)
}

// Source: reference_case.go
const ReferenceCaseStatusEnabled = referencecaseservice.ReferenceCaseStatusEnabled

type ReferenceCase = referencecaseservice.ReferenceCase
type ReferenceCasePayload = referencecaseservice.ReferenceCasePayload
type ReferenceCaseService = referencecaseservice.ReferenceCaseService

func NewReferenceCaseService() *ReferenceCaseService {
	return referencecaseservice.NewService(PlatformConnection(), OrmForConnectionWithContext)
}

func ApplyReferenceCaseUpgrade(ctx context.Context) error {
	return referencecaseservice.ApplyUpgrade(ctx, PlatformConnection(), OrmForConnectionWithContext)
}

func RollbackReferenceCaseUpgrade(ctx context.Context) error {
	return referencecaseservice.RollbackUpgrade(ctx, PlatformConnection(), OrmForConnectionWithContext)
}

// Source: scheduled_task.go
const (
	ScheduledTaskStatusEnabled  = scheduledtaskservice.ScheduledTaskStatusEnabled
	ScheduledTaskStatusDisabled = scheduledtaskservice.ScheduledTaskStatusDisabled

	ScheduledTaskTypeURL        = scheduledtaskservice.ScheduledTaskTypeURL
	ScheduledTaskTypeScript     = scheduledtaskservice.ScheduledTaskTypeScript
	ScheduledTaskTypeMethod     = scheduledtaskservice.ScheduledTaskTypeMethod
	ScheduledTaskTypeBackup     = scheduledtaskservice.ScheduledTaskTypeBackup
	ScheduledTaskTypeGovernance = scheduledtaskservice.ScheduledTaskTypeGovernance

	ScheduledTaskLogStatusRunning = scheduledtaskservice.ScheduledTaskLogStatusRunning
	ScheduledTaskLogStatusSuccess = scheduledtaskservice.ScheduledTaskLogStatusSuccess
	ScheduledTaskLogStatusFailed  = scheduledtaskservice.ScheduledTaskLogStatusFailed
	ScheduledTaskLogStatusSkipped = scheduledtaskservice.ScheduledTaskLogStatusSkipped

	ScheduledTaskTriggerSchedule = scheduledtaskservice.ScheduledTaskTriggerSchedule
	ScheduledTaskTriggerManual   = scheduledtaskservice.ScheduledTaskTriggerManual
)

type ScheduledTask = scheduledtaskservice.ScheduledTask
type ScheduledTaskLog = scheduledtaskservice.ScheduledTaskLog
type ScheduledTaskPayload = scheduledtaskservice.ScheduledTaskPayload
type ScheduledTaskService = scheduledtaskservice.ScheduledTaskService
type ScheduledTaskHandler = scheduledtaskservice.ScheduledTaskHandler
type ScheduledTaskExecutionResult = scheduledtaskservice.ScheduledTaskExecutionResult
type ScheduledTaskTenantOption = scheduledtaskservice.ScheduledTaskTenantOption
type SchedulerNodeMetric = scheduledtaskservice.SchedulerNodeMetric
type SchedulerHeartbeatMetrics = scheduledtaskservice.SchedulerHeartbeatMetrics

var RecordSchedulerHeartbeat = scheduledtaskservice.RecordSchedulerHeartbeat
var SchedulerHeartbeatSnapshot = scheduledtaskservice.SchedulerHeartbeatSnapshot
var ResetSchedulerHeartbeatForTest = scheduledtaskservice.ResetSchedulerHeartbeatForTest

func init() {
	scheduledtaskservice.ConfigureDependencies(
		OrmForConnectionWithContext,
		PlatformConnection,
		RecordTenantGovernanceEvent,
	)
	scheduledtaskservice.RegisterScheduledTaskHandler(
		"scheduler.tenant_retention",
		tenantRetentionScheduledTaskHandler,
	)
	scheduledtaskservice.RegisterScheduledTaskHandler(
		"scheduler.tenant_isolation_verify",
		tenantIsolationScheduledTaskHandler,
	)
}

func NewScheduledTaskService() *ScheduledTaskService {
	return scheduledtaskservice.NewScheduledTaskService()
}

func NewScheduledTaskServiceForNode(nodeIP string) *ScheduledTaskService {
	return scheduledtaskservice.NewScheduledTaskServiceForNode(nodeIP)
}

func NewScheduledTaskRunner() foundation.Runner {
	return scheduledtaskservice.NewScheduledTaskRunner()
}

func RegisterScheduledTaskHandler(name string, handler ScheduledTaskHandler) {
	scheduledtaskservice.RegisterScheduledTaskHandler(name, handler)
}

func UnregisterScheduledTaskHandler(name string) {
	scheduledtaskservice.UnregisterScheduledTaskHandler(name)
}

func NextScheduledRun(expression string, location *time.Location, after time.Time) (time.Time, error) {
	return scheduledtaskservice.NextScheduledRun(expression, location, after)
}

func ScheduledTaskTargetsNode(targetIPs []string, nodeIP string) bool {
	return scheduledtaskservice.ScheduledTaskTargetsNode(targetIPs, nodeIP)
}

func SchedulerNodeIP() string {
	return scheduledtaskservice.SchedulerNodeIP()
}

func ScheduledTaskUsesPrivilegedHandler(taskType string, payload models.JSONMap) bool {
	return scheduledtaskservice.ScheduledTaskUsesPrivilegedHandler(taskType, payload)
}

func taskFailure(message string) ScheduledTaskExecutionResult {
	return scheduledtaskservice.Failure(message)
}

func governanceTaskOutcome(status string) string {
	return scheduledtaskservice.GovernanceTaskOutcome(status)
}

func validateScheduledTaskPayload(task ScheduledTask) error {
	return scheduledtaskservice.ValidateScheduledTaskPayload(task)
}

func isPrivilegedScheduledTaskHandler(name string) bool {
	return scheduledtaskservice.IsPrivilegedScheduledTaskHandler(name)
}

// Source: security_rotation.go
type KeyRotationFinding = securityrotationservice.KeyRotationFinding
type StoredSecretRotationSource = securityrotationservice.StoredSecretRotationSource

func init() {
	securityrotationservice.ConfigureDependencies(
		OrmForConnectionWithContext,
		PlatformConnection,
		RegisterTenantConnection,
	)
}

func CheckKeyRotation(now time.Time) []KeyRotationFinding {
	return securityrotationservice.CheckKeyRotation(now)
}

func CheckDatabaseKeyRotation(ctx context.Context, now time.Time) ([]KeyRotationFinding, error) {
	return securityrotationservice.CheckDatabaseKeyRotation(ctx, now)
}

func StoredSecretRotationFindings(sources []StoredSecretRotationSource, now time.Time, days int) []KeyRotationFinding {
	return securityrotationservice.StoredSecretRotationFindings(sources, now, days)
}

// Source: storage.go
const (
	StorageConfigStatusEnabled = storageservice.StorageConfigStatusEnabled
	storageDriverLocal         = storageservice.DriverLocal
	storageDriverS3Compatible  = storageservice.DriverS3Compatible
)

type StorageConfig = storageservice.StorageConfig
type StorageConfigPayload = storageservice.StorageConfigPayload
type AttachmentService = storageservice.AttachmentService
type objectStorageClient = storageservice.ObjectStorageClient

type StorageConfigService struct {
	*storageservice.StorageConfigService
	ctx context.Context
}

func init() {
	storageservice.ConfigureDependencies(
		OrmForConnectionWithContext,
		PlatformConnection,
		RegisterTenantConnection,
		func(ctx context.Context, tenant Tenant) (int64, error) {
			quotas := NewTenantRuntimeService().WithContext(ctx).EffectiveQuotas(tenant)
			return jsonInt64(quotas, "max_storage_mb"), nil
		},
	)
}

func NewStorageConfigService() *StorageConfigService {
	return &StorageConfigService{StorageConfigService: storageservice.NewStorageConfigService()}
}

func (s *StorageConfigService) WithContext(ctx context.Context) *StorageConfigService {
	return &StorageConfigService{
		StorageConfigService: s.StorageConfigService.WithContext(ctx),
		ctx:                  contextOrBackground(ctx),
	}
}

func (s *StorageConfigService) find(id uint64) (StorageConfig, error) {
	return s.StorageConfigService.Find(id)
}

func NewAttachmentService() *AttachmentService {
	return storageservice.NewAttachmentService()
}

func NewAttachmentServiceForTenant(tenant Tenant) *AttachmentService {
	return storageservice.NewAttachmentServiceForTenant(tenant)
}

func newObjectStorageClient(config StorageConfig) *objectStorageClient {
	return storageservice.NewObjectStorageClient(config)
}
