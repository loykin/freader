package file_tracker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

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
// If the file size is smaller than maxBytes, it returns an error to indicate insufficient content for fingerprinting.
// This is intentional to ensure consistent fingerprinting behavior (e.g., avoiding false matches on partial content).
func GetFileFingerprint(file *os.File, maxBytes int64) (string, error) {
	info, err := file.Stat()
	if err != nil {
		return "", err
	}

	// Return error only if file is smaller than required minimum
	// This allows the caller (watcher) to skip the file gracefully
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

func findNthSeparator(buf, sep []byte, n, found, searchStart int) (posAfter, newFound int, ok bool) {
	b := buf
	for found < n {
		idx := bytes.Index(b[searchStart:], sep)
		if idx < 0 {
			return 0, found, false
		}
		found++
		posAfter := searchStart + idx + len(sep)
		if found == n {
			return posAfter, found, true
		}
		searchStart = posAfter
	}
	return 0, found, false
}

func calcSearchStart(bufLen, sepLen int) int {
	if sepLen <= 1 {
		return bufLen
	}
	if bufLen >= sepLen-1 {
		return bufLen - (sepLen - 1)
	}
	return 0
}

func hashUntil(data []byte, end int) string {
	h := sha256.Sum256(data[:end])
	return hex.EncodeToString(h[:])
}

// GetFileFingerprintUntilNSeparators hashes from the start up to and including the N-th occurrence of sep.
// Simple approach: keep reading, accumulate into a buffer, search for the separator; if not found, continue.
// When the Nth separator is found, compute SHA-256 over bytes up to and including it and return.
// If EOF occurs before reaching N separators, returns NotEnoughSeparatorsError.
func GetFileFingerprintUntilNSeparators(file *os.File, sep string, n int) (string, error) {
	if sep == "" {
		return "", errors.New("separator must not be empty")
	}
	if n <= 0 {
		return "", errors.New("separator count must be > 0")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	sepB := []byte(sep)
	var acc bytes.Buffer
	found := 0
	searchStart := 0

	chunk := make([]byte, 32*1024)
	for {
		nr, err := file.Read(chunk)
		if nr > 0 {
			acc.Write(chunk[:nr])
			pos, updated, ok := findNthSeparator(acc.Bytes(), sepB, n, found, searchStart)
			found = updated
			if ok {
				return hashUntil(acc.Bytes(), pos), nil
			}
			// 못 찾았으면 다음 searchStart 준비
			searchStart = calcSearchStart(acc.Len(), len(sepB))
		}
		if err != nil {
			if err == io.EOF {
				return "", &NotEnoughSeparatorsError{Expected: n, Actual: found, Sep: sep}
			}
			return "", err
		}
	}
}

// GetFileFingerprintUntilNSeparatorsFromPath opens path and computes separator-based fingerprint.
func GetFileFingerprintUntilNSeparatorsFromPath(path, sep string, n int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot open file: %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return GetFileFingerprintUntilNSeparators(f, sep, n)
}
