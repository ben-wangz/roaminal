//go:build !linux && !darwin

package report

func withDiagnosticFileLock(_ string, fn func() error) error {
	return fn()
}
