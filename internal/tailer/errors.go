package tailer

import "fmt"

// FileFingerprintMismatchError indicates that a file's fingerprint has changed,
// usually due to file rotation, truncation, or overwrite.
type FileFingerprintMismatchError struct {
	Path                string
	ExpectedFingerprint string
	ActualFingerprint   string
}

func (e *FileFingerprintMismatchError) Error() string {
	return fmt.Sprintf("file fingerprint mismatch for %s: expected %s, got %s",
		e.Path, e.ExpectedFingerprint, e.ActualFingerprint)
}

// IsFileFingerprintMismatch checks if an error is a FileFingerprintMismatchError.
func IsFileFingerprintMismatch(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*FileFingerprintMismatchError)
	return ok
}
