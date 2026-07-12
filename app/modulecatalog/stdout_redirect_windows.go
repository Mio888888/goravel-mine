//go:build windows

package modulecatalog

func withStdoutRedirectedToStderr(run func() error) error {
	return run()
}
