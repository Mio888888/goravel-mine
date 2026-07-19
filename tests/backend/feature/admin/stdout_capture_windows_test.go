//go:build windows

package admin

import "testing"

func captureStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	t.Skip("stdout fd capture is only validated on Unix-like CI runners")
	return "", run()
}
