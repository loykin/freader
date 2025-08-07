package file_tracker

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

type FileSizeTooSmallError struct {
	Expected int64
	Actual   int64
}

func (e *FileSizeTooSmallError) Error() string {
	return fmt.Sprintf("expected file size to be greater than %d bytes, got %d bytes", e.Expected, e.Actual)
}

// IsFileSizeTooSmall determines if the provided error is of type FileSizeTooSmallError.
func IsFileSizeTooSmall(err error) bool {
	var sizeErr *FileSizeTooSmallError
	return errors.As(err, &sizeErr)
}

func GetFileFingerprintFromPath(path string, maxBytes int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot open file: %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	return GetFileFingerprint(file, maxBytes)
}

// GetFileFingerprint computes SHA-256 hash of a file's content, up to a specified maximum number of bytes.
// It takes a file path and a maximum byte limit, returning the hash as a hexadecimal string or an error.
// Returns an error if the file size is smaller than the specified limit or other issues occur during processing.
func GetFileFingerprint(file *os.File, maxBytes int64) (string, error) {
	info, err := file.Stat()
	if err != nil {
		return "", err
	}

	if info.Size() < maxBytes {
		return "", &FileSizeTooSmallError{
			Expected: maxBytes,
			Actual:   info.Size(),
		}
	}

	var reader io.Reader = file
	if maxBytes > 0 {
		reader = io.LimitReader(file, maxBytes)
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", errors.New("failed to compute hash: " + err.Error())
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
