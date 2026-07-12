//go:build unix

package modulecatalog

import "syscall"

func withStdoutRedirectedToStderr(run func() error) (err error) {
	lifecycleStdoutRedirectMu.Lock()
	defer lifecycleStdoutRedirectMu.Unlock()

	original, err := syscall.Dup(syscall.Stdout)
	if err != nil {
		return err
	}
	restored := false
	defer func() {
		if !restored {
			if restoreErr := syscall.Dup2(original, syscall.Stdout); restoreErr != nil && err == nil {
				err = restoreErr
			}
		}
		_ = syscall.Close(original)
	}()
	if err := syscall.Dup2(syscall.Stderr, syscall.Stdout); err != nil {
		return err
	}
	runErr := run()
	restoreErr := syscall.Dup2(original, syscall.Stdout)
	if restoreErr != nil {
		return restoreErr
	}
	restored = true

	return runErr
}
