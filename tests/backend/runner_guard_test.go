package main

import (
	"os"
	"testing"
)

func TestBackendTestsUseCentralRunner(t *testing.T) {
	if os.Getenv("GORAVEL_TEST_OVERLAY") != "1" {
		t.Fatal("集中测试未加载，请使用 tests/backend/test.sh 代替 go test")
	}
}
