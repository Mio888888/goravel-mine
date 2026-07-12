//go:build unix

package admin

import (
	"bytes"
	"io"
	"os"
	"syscall"
	"testing"
)

func captureStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}
	original, err := syscall.Dup(syscall.Stdout)
	if err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return "", err
	}
	defer func() {
		_ = syscall.Close(original)
	}()

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(&buf, reader)
		done <- copyErr
	}()

	if err := syscall.Dup2(int(writer.Fd()), syscall.Stdout); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return "", err
	}
	runErr := run()
	restoreErr := syscall.Dup2(original, syscall.Stdout)
	closeErr := writer.Close()
	copyErr := <-done
	readerErr := reader.Close()
	for _, err := range []error{runErr, restoreErr, closeErr, copyErr, readerErr} {
		if err != nil {
			return buf.String(), err
		}
	}

	return buf.String(), nil
}
