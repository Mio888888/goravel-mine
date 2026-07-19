package observability

import "time"

type TenantGovernanceMetrics struct {
	EvidenceExpired    int64
	VerificationFailed int64
	OldestRunAge       time.Duration
}

type TenantConnectionCapacityMetrics struct {
	Pools               int
	RequiredConnections int
	PostgreSQLBudget    int
	Safe                bool
}

type MigrationLockMetrics struct {
	AcquiredTotal            uint64
	ReleasedTotal            uint64
	TimeoutTotal             uint64
	FailureTotal             uint64
	WaitDurationSecondsTotal float64
	HoldDurationSecondsTotal float64
}
