package modulecatalog

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type limitedLifecycleBuffer struct {
	buf   bytes.Buffer
	limit int
}

func newLimitedLifecycleBuffer(limit int) *limitedLifecycleBuffer {
	return &limitedLifecycleBuffer{limit: limit}
}

func (b *limitedLifecycleBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := min(b.limit-b.buf.Len(), len(p))
	if remaining > 0 {
		if _, err := b.buf.Write(p[:remaining]); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (b *limitedLifecycleBuffer) String() string {
	return b.buf.String()
}

func runLifecycleCommand(ctx context.Context, command string) (string, string, error) {
	args := append([]string{"artisan"}, strings.Fields(command)...)
	cmd := exec.CommandContext(contextOrBackground(ctx), os.Args[0], args...)
	stdout := newLimitedLifecycleBuffer(maxLifecycleOutput)
	stderr := newLimitedLifecycleBuffer(maxLifecycleOutput)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

var lifecycleStdoutRedirectMu sync.Mutex

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func randomLifecycleToken() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes[:])
}

func firstLifecycleNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func lifecycleLockKey(moduleID string) string {
	return "module-lifecycle:" + strings.TrimSpace(moduleID)
}

func lifecycleLockRunKey(idempotencyKey string) string {
	const tokenLength = 32
	prefix := strings.TrimSpace(idempotencyKey)
	maxPrefixLength := 220 - len(":lease:") - tokenLength
	if len(prefix) > maxPrefixLength {
		prefix = prefix[:maxPrefixLength]
	}
	return prefix + ":lease:" + randomLifecycleToken()
}
