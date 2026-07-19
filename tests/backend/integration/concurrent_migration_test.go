package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/services"
	_ "goravel/tests/backend/testcase"
)

func TestConcurrentMigration(t *testing.T) {
	if os.Getenv("MIGRATION_LOCK_HELPER") == "holder" {
		runConcurrentMigrationHelper(t)
		return
	}
	if os.Getenv("MIGRATION_LOCK_HELPER") == "contender" {
		runConcurrentMigrationContender(t)
		return
	}

	readyFile := filepath.Join(t.TempDir(), "migration-lock-ready")
	first := exec.Command(os.Args[0], "-test.run=^TestConcurrentMigration$", "--")
	first.Env = append(os.Environ(), "MIGRATION_LOCK_HELPER=holder", "MIGRATION_LOCK_READY_FILE="+readyFile, "MIGRATION_LOCK_HOLD=750ms")
	require.NoError(t, first.Start())
	defer func() { _ = first.Process.Kill() }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(readyFile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			output, _ := first.CombinedOutput()
			if strings.Contains(string(output), "migration advisory lock") || strings.Contains(string(output), "connection") {
				t.Skipf("PostgreSQL integration database unavailable: %s", output)
			}
			t.Fatalf("first migration process did not acquire its lock: %s", output)
		}
		time.Sleep(20 * time.Millisecond)
	}

	contender := exec.Command(os.Args[0], "-test.run=^TestConcurrentMigration$", "--")
	contender.Env = append(os.Environ(), "MIGRATION_LOCK_HELPER=contender")
	output, err := contender.CombinedOutput()
	require.NoError(t, err, string(output))
	require.Contains(t, string(output), "MIGRATION_LOCK_CONTENDER_REJECTED")
	require.NoError(t, first.Wait())
}

func runConcurrentMigrationHelper(t *testing.T) {
	lock, err := services.NewMigrationLockService().Acquire(context.Background(), services.MigrationScopePlatform, 100*time.Millisecond)
	require.NoError(t, err)
	defer func() { require.NoError(t, lock.Release(context.Background())) }()

	readyFile := os.Getenv("MIGRATION_LOCK_READY_FILE")
	require.NoError(t, os.WriteFile(readyFile, []byte("ready"), 0600))
	fmt.Println("MIGRATION_LOCK_HELPER_RUNNING")
	hold, err := time.ParseDuration(os.Getenv("MIGRATION_LOCK_HOLD"))
	require.NoError(t, err)
	time.Sleep(hold)
}

func runConcurrentMigrationContender(t *testing.T) {
	_, err := services.NewMigrationLockService().Acquire(context.Background(), services.MigrationScopePlatform, 100*time.Millisecond)
	require.ErrorIs(t, err, services.ErrMigrationLockTimeout)
	fmt.Println("MIGRATION_LOCK_CONTENDER_REJECTED")
}
