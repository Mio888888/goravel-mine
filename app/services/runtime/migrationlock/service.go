package migrationlock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	observabilitycontract "goravel/app/contracts/observability"
	observabilityservice "goravel/app/services/runtime/observability"
)

const (
	MigrationScopePlatform MigrationScope = "platform"
	MigrationScopeTenants  MigrationScope = "tenants"
	MigrationScopeAll      MigrationScope = "all"
)

var (
	ErrMigrationScope       = errors.New("migration scope must be platform, tenants, or all")
	ErrMigrationLockTimeout = errors.New("migration advisory lock timeout")
)

type MigrationScope string

type MigrationLockMetadata struct {
	Scope        MigrationScope
	Database     string
	RunID        string
	Hostname     string
	PID          int
	AdvisoryKeys []int64
	AcquiredAt   time.Time
	WaitDuration time.Duration
}

type MigrationLock interface {
	Metadata() MigrationLockMetadata
	Release(context.Context) error
}

type MigrationLockProvider interface {
	Acquire(context.Context, MigrationScope, time.Duration) (MigrationLock, error)
}

type MigrationLockMetrics = observabilitycontract.MigrationLockMetrics
type AuditEvent = observabilityservice.AuditEvent

const (
	AuditOutcomeSuccess = observabilityservice.AuditOutcomeSuccess
	AuditOutcomeFailure = observabilityservice.AuditOutcomeFailure
)

type migrationLockMetricsRecorder struct {
	mu      sync.Mutex
	metrics MigrationLockMetrics
}

var defaultMigrationLockMetrics = &migrationLockMetricsRecorder{}

func MetricsSnapshot() MigrationLockMetrics {
	defaultMigrationLockMetrics.mu.Lock()
	defer defaultMigrationLockMetrics.mu.Unlock()
	return defaultMigrationLockMetrics.metrics
}

func ResetMetricsForTest() {
	defaultMigrationLockMetrics.mu.Lock()
	defer defaultMigrationLockMetrics.mu.Unlock()
	defaultMigrationLockMetrics.metrics = MigrationLockMetrics{}
}

func (r *migrationLockMetricsRecorder) recordAcquire(wait time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics.AcquiredTotal++
	r.metrics.WaitDurationSecondsTotal += wait.Seconds()
}

func (r *migrationLockMetricsRecorder) recordRelease(held time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics.ReleasedTotal++
	r.metrics.HoldDurationSecondsTotal += held.Seconds()
}

func (r *migrationLockMetricsRecorder) recordTimeout() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics.TimeoutTotal++
}

func (r *migrationLockMetricsRecorder) recordFailure() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics.FailureTotal++
}

type migrationLockSession interface {
	DatabaseIdentity(context.Context) (string, error)
	TryAdvisoryLock(context.Context, int64) (bool, error)
	Unlock(context.Context, int64) error
	Close() error
}

type postgresMigrationLockSession struct {
	connection *sql.Conn
}

func newPostgresMigrationLockSession(ctx context.Context) (migrationLockSession, error) {
	database, err := OrmForConnectionWithContext(ctx, PlatformConnection()).DB()
	if err != nil {
		return nil, fmt.Errorf("open migration lock database connection: %w", err)
	}
	connection, err := database.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("reserve migration lock database connection: %w", err)
	}
	return &postgresMigrationLockSession{connection: connection}, nil
}

func (s *postgresMigrationLockSession) DatabaseIdentity(ctx context.Context) (string, error) {
	var database string
	if err := s.connection.QueryRowContext(ctx, "SELECT current_database()").Scan(&database); err != nil {
		return "", err
	}
	return strings.TrimSpace(database), nil
}

func (s *postgresMigrationLockSession) TryAdvisoryLock(ctx context.Context, key int64) (bool, error) {
	var acquired bool
	err := s.connection.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&acquired)
	return acquired, err
}

func (s *postgresMigrationLockSession) Unlock(ctx context.Context, key int64) error {
	var released bool
	if err := s.connection.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", key).Scan(&released); err != nil {
		return err
	}
	if !released {
		return errors.New("migration advisory lock was not held by this session")
	}
	return nil
}

func (s *postgresMigrationLockSession) Close() error {
	return s.connection.Close()
}

type MigrationLockService struct {
	sessionFactory func(context.Context) (migrationLockSession, error)
	audit          func(context.Context, AuditEvent)
	now            func() time.Time
	newRunID       func() string
	hostname       func() string
	pid            int
	retryInterval  time.Duration
}

func NewMigrationLockService() *MigrationLockService {
	return newMigrationLockService(newPostgresMigrationLockSession)
}

func newMigrationLockService(factory func(context.Context) (migrationLockSession, error)) *MigrationLockService {
	return &MigrationLockService{
		sessionFactory: factory,
		audit:          RecordAuditEvent,
		now:            time.Now,
		newRunID:       migrationLockRunID,
		hostname:       migrationLockHostname,
		pid:            os.Getpid(),
		retryInterval:  50 * time.Millisecond,
	}
}

func (s *MigrationLockService) Acquire(ctx context.Context, scope MigrationScope, timeout time.Duration) (MigrationLock, error) {
	ctx = contextOrBackground(ctx)
	if !validMigrationScope(scope) {
		return nil, ErrMigrationScope
	}
	session, err := s.sessionFactory(ctx)
	if err != nil {
		defaultMigrationLockMetrics.recordFailure()
		return nil, err
	}
	database, err := session.DatabaseIdentity(ctx)
	if err != nil {
		_ = session.Close()
		defaultMigrationLockMetrics.recordFailure()
		return nil, fmt.Errorf("identify migration lock database: %w", err)
	}
	keys, err := migrationLockKeys(database, scope)
	if err != nil {
		_ = session.Close()
		defaultMigrationLockMetrics.recordFailure()
		return nil, err
	}
	metadata := MigrationLockMetadata{
		Scope: scope, Database: database, RunID: s.newRunID(), Hostname: s.hostname(), PID: s.pid, AdvisoryKeys: keys,
	}
	s.audit(ctx, migrationLockAuditEvent("migration.lock.wait", AuditOutcomeSuccess, metadata, 0))

	waited, err := s.acquireAll(ctx, session, keys, timeout)
	if err != nil {
		_ = session.Close()
		if errors.Is(err, ErrMigrationLockTimeout) {
			defaultMigrationLockMetrics.recordTimeout()
			s.audit(ctx, migrationLockAuditEvent("migration.lock.timeout", AuditOutcomeFailure, metadata, timeout))
			return nil, err
		}
		defaultMigrationLockMetrics.recordFailure()
		s.audit(ctx, migrationLockAuditEvent("migration.lock.acquire", AuditOutcomeFailure, metadata, waited))
		return nil, fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	metadata.AcquiredAt = s.now()
	metadata.WaitDuration = waited
	defaultMigrationLockMetrics.recordAcquire(waited)
	s.audit(ctx, migrationLockAuditEvent("migration.lock.acquire", AuditOutcomeSuccess, metadata, waited))
	return &postgresMigrationLock{session: session, metadata: metadata, audit: s.audit, now: s.now}, nil
}

func (s *MigrationLockService) acquireAll(ctx context.Context, session migrationLockSession, keys []int64, timeout time.Duration) (time.Duration, error) {
	startedAt := s.now()
	deadline := startedAt.Add(timeout)
	for {
		acquired := make([]int64, 0, len(keys))
		for _, key := range keys {
			locked, err := session.TryAdvisoryLock(ctx, key)
			if err != nil {
				s.releaseKeys(context.Background(), session, acquired)
				return s.now().Sub(startedAt), err
			}
			if !locked {
				s.releaseKeys(context.Background(), session, acquired)
				acquired = nil
				break
			}
			acquired = append(acquired, key)
		}
		if len(acquired) == len(keys) {
			return s.now().Sub(startedAt), nil
		}
		if timeout <= 0 || !s.now().Before(deadline) {
			return s.now().Sub(startedAt), ErrMigrationLockTimeout
		}
		wait := s.retryInterval
		if remaining := deadline.Sub(s.now()); remaining < wait {
			wait = remaining
		}
		if err := waitForMigrationLock(ctx, wait); err != nil {
			return s.now().Sub(startedAt), err
		}
	}
}

func (s *MigrationLockService) releaseKeys(ctx context.Context, session migrationLockSession, keys []int64) {
	for index := len(keys) - 1; index >= 0; index-- {
		_ = session.Unlock(ctx, keys[index])
	}
}

type postgresMigrationLock struct {
	mu       sync.Mutex
	session  migrationLockSession
	metadata MigrationLockMetadata
	audit    func(context.Context, AuditEvent)
	now      func() time.Time
	released bool
}

func (l *postgresMigrationLock) Metadata() MigrationLockMetadata {
	return l.metadata
}

func (l *postgresMigrationLock) Release(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return nil
	}
	ctx = contextOrBackground(ctx)
	var releaseErr error
	for index := len(l.metadata.AdvisoryKeys) - 1; index >= 0; index-- {
		if err := l.session.Unlock(ctx, l.metadata.AdvisoryKeys[index]); err != nil && releaseErr == nil {
			releaseErr = err
		}
	}
	if err := l.session.Close(); err != nil && releaseErr == nil {
		releaseErr = err
	}
	l.released = true
	held := l.now().Sub(l.metadata.AcquiredAt)
	if releaseErr != nil {
		defaultMigrationLockMetrics.recordFailure()
		l.audit(ctx, migrationLockAuditEvent("migration.lock.release", AuditOutcomeFailure, l.metadata, held))
		return fmt.Errorf("release migration advisory lock: %w", releaseErr)
	}
	defaultMigrationLockMetrics.recordRelease(held)
	l.audit(ctx, migrationLockAuditEvent("migration.lock.release", AuditOutcomeSuccess, l.metadata, held))
	return nil
}

func ParseMigrationScope(value string) (MigrationScope, error) {
	scope := MigrationScope(strings.ToLower(strings.TrimSpace(value)))
	if !validMigrationScope(scope) {
		return "", ErrMigrationScope
	}
	return scope, nil
}

func validMigrationScope(scope MigrationScope) bool {
	return scope == MigrationScopePlatform || scope == MigrationScopeTenants || scope == MigrationScopeAll
}

func migrationLockKeys(database string, scope MigrationScope) ([]int64, error) {
	database = strings.TrimSpace(database)
	if database == "" {
		return nil, errors.New("migration lock database identity is required")
	}
	if !validMigrationScope(scope) {
		return nil, ErrMigrationScope
	}
	scopes := []MigrationScope{scope}
	if scope == MigrationScopeAll {
		scopes = []MigrationScope{MigrationScopePlatform, MigrationScopeTenants}
	}
	keys := make([]int64, 0, len(scopes))
	for _, item := range scopes {
		hash := fnv.New64a()
		_, _ = hash.Write([]byte("goravel-mine:migration:" + database + ":" + string(item)))
		keys = append(keys, int64(hash.Sum64()))
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	return keys, nil
}

func waitForMigrationLock(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func migrationLockAuditEvent(action, outcome string, metadata MigrationLockMetadata, duration time.Duration) AuditEvent {
	return AuditEvent{
		Action: action, Outcome: outcome, Actor: "system:migration", Method: "CLI", Route: "migration:safe",
		Fields: map[string]any{
			"scope": metadata.Scope, "database": metadata.Database, "run_id": metadata.RunID,
			"hostname": metadata.Hostname, "pid": metadata.PID, "advisory_keys": metadata.AdvisoryKeys,
			"duration_ms": duration.Milliseconds(),
		},
	}
}

func migrationLockRunID() string {
	return fmt.Sprintf("migration-%d-%d", time.Now().UnixNano(), os.Getpid())
}

func migrationLockHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
