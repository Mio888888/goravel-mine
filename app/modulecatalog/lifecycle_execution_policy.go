package modulecatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

func lifecycleErrorStatus(err error) string {
	var runningErr lifecycleCommandStillRunningError
	if errors.As(err, &runningErr) {
		return LifecycleStatusReconciliationRequired
	}
	var manualErr manualLifecycleCommandError
	if errors.As(err, &manualErr) {
		return LifecycleStatusManualRequired
	}
	return LifecycleStatusFailed
}

func (e lifecycleExecutor) lockTTLValue() time.Duration {
	if e.lockTTL <= 0 {
		return 5 * time.Minute
	}
	return e.lockTTL
}

func (e lifecycleExecutor) lockRenewIntervalValue() time.Duration {
	if e.lockRenewInterval > 0 {
		return e.lockRenewInterval
	}
	if ttl := e.lockTTLValue(); ttl <= time.Second {
		return ttl / 2
	}
	return time.Minute
}

func (e lifecycleExecutor) lockRenewTimeout(lock LifecycleLock) time.Duration {
	interval := e.lockRenewIntervalValue()
	if lock.ExpiresAt.IsZero() {
		return interval
	}
	remaining := lock.ExpiresAt.Sub(e.clock.Now())
	if remaining <= 0 {
		return 0
	}
	if interval <= 0 || interval > remaining {
		return remaining
	}
	return interval
}

func (e lifecycleExecutor) commandTimeoutValue() time.Duration {
	if e.commandTimeout <= 0 {
		return 10 * time.Minute
	}
	return e.commandTimeout
}

func (e lifecycleExecutor) runnerCancelGraceValue() time.Duration {
	if e.runnerCancelGrace < 0 {
		return 0
	}
	if e.runnerCancelGrace == 0 {
		return defaultLifecycleRunnerCancelGrace
	}
	return e.runnerCancelGrace
}

func lifecycleCommandPolicyHash(command string) string {
	canonical := strings.Join(normalizeLifecycleCommands(command), " && ")
	digest := sha256.Sum256([]byte(canonical))
	return "sha256:" + hex.EncodeToString(digest[:])
}
