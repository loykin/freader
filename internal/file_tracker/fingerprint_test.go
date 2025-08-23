package file_tracker

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileSizeTooSmallError(t *testing.T) {
	tests := []struct {
		name     string
		expected int64
		actual   int64
		want     string
	}{
		{
			name:     "Positive values",
			expected: 100,
			actual:   50,
			want:     "expected file size to be greater than 100 bytes, got 50 bytes",
		},
		{
			name:     "Zero actual",
			expected: 100,
			actual:   0,
			want:     "expected file size to be greater than 100 bytes, got 0 bytes",
		},
		{
			name:     "Equal values",
			expected: 100,
			actual:   100,
			want:     "expected file size to be greater than 100 bytes, got 100 bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &FileSizeTooSmallError{
				Expected: tt.expected,
				Actual:   tt.actual,
			}
			assert.Equal(t, tt.want, err.Error())
		})
	}
}

func TestGetFileFingerprint(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		setupFunc   func() (*os.File, error)
		maxBytes    int64
		expectError bool
		errorType   string
	}{
		{
			name: "Large file with small maxBytes",
			setupFunc: func() (*os.File, error) {
				path := filepath.Join(tempDir, "large.txt")
				content := []byte("large content for testing fingerprint generation")
				if err := os.WriteFile(path, content, 0644); err != nil {
					return nil, err
				}
				return os.Open(path)
			},
			maxBytes:    10,
			expectError: false,
		},
		{
			name: "Small file with large maxBytes",
			setupFunc: func() (*os.File, error) {
				path := filepath.Join(tempDir, "small.txt")
				content := []byte("small")
				if err := os.WriteFile(path, content, 0644); err != nil {
					return nil, err
				}
				return os.Open(path)
			},
			maxBytes:    100,
			expectError: true,
			errorType:   "FileSizeTooSmall",
		},
		{
			name: "Empty file",
			setupFunc: func() (*os.File, error) {
				path := filepath.Join(tempDir, "empty.txt")
				if err := os.WriteFile(path, []byte{}, 0644); err != nil {
					return nil, err
				}
				return os.Open(path)
			},
			maxBytes:    1,
			expectError: true,
			errorType:   "FileSizeTooSmall",
		},
		{
			name: "Zero maxBytes",
			setupFunc: func() (*os.File, error) {
				path := filepath.Join(tempDir, "zero_max.txt")
				content := []byte("content for zero maxBytes test")
				if err := os.WriteFile(path, content, 0644); err != nil {
					return nil, err
				}
				return os.Open(path)
			},
			maxBytes:    0,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := tt.setupFunc()
			assert.NoError(t, err)
			defer func() { _ = file.Close() }()

			fingerprint, err := GetFileFingerprint(file, tt.maxBytes)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType == "FileSizeTooSmall" {
					assert.True(t, IsFileSizeTooSmall(err))
				}
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, fingerprint)

				// Verify fingerprint format (should be hex-encoded SHA-256)
				assert.Len(t, fingerprint, 64) // SHA-256 produces 32 bytes = 64 hex chars
			}
		})
	}
}

func TestGetFileFingerprintFromPath(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		setup       func() string
		maxBytes    int64
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid file",
			setup: func() string {
				path := filepath.Join(tempDir, "valid.txt")
				_ = os.WriteFile(path, []byte("test content"), 0644)
				return path
			},
			maxBytes:    5,
			expectError: false,
		},
		{
			name: "Non-existent file",
			setup: func() string {
				return filepath.Join(tempDir, "nonexistent.txt")
			},
			maxBytes:    5,
			expectError: true,
			errorMsg:    "cannot open file",
		},
		{
			name: "Directory instead of file",
			setup: func() string {
				path := filepath.Join(tempDir, "dir")
				_ = os.Mkdir(path, 0755)
				return path
			},
			maxBytes:    5,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup()
			fingerprint, err := GetFileFingerprintFromPath(path, tt.maxBytes)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, fingerprint)
			}
		})
	}
}

func TestFingerprint_Consistency(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.txt")
	content := []byte("test content for fingerprint consistency")

	assert.NoError(t, os.WriteFile(filePath, content, 0644))

	// Get multiple fingerprints of the same file
	fingerprints := make([]string, 3)
	for i := range fingerprints {
		fp, err := GetFileFingerprintFromPath(filePath, 10)
		assert.NoError(t, err)
		fingerprints[i] = fp
	}

	// All fingerprints should be identical
	for i := 1; i < len(fingerprints); i++ {
		assert.Equal(t, fingerprints[0], fingerprints[i],
			"Fingerprints should be consistent for the same file and maxBytes")
	}
}

func TestGetFileFingerprintUntilNSeparators(t *testing.T) {
	tempDir := t.TempDir()
	p := filepath.Join(tempDir, "sep.txt")
	content := "a<END>b<END>c<END>"
	assert.NoError(t, os.WriteFile(p, []byte(content), 0644))

	// Expected: hash of bytes up to and including second <END>
	secondPos := len("a<END>b<END>")
	h := sha256.Sum256([]byte(content[:secondPos]))
	expected := hex.EncodeToString(h[:])

	fp, err := GetFileFingerprintUntilNSeparatorsFromPath(p, "<END>", 2)
	assert.NoError(t, err)
	assert.Equal(t, expected, fp)

	// n=3 should hash full content
	fp3, err := GetFileFingerprintUntilNSeparatorsFromPath(p, "<END>", 3)
	assert.NoError(t, err)
	h3 := sha256.Sum256([]byte(content))
	assert.Equal(t, hex.EncodeToString(h3[:]), fp3)
}

func TestGetFileFingerprintUntilNSeparators_NotEnough(t *testing.T) {
	tempDir := t.TempDir()
	p := filepath.Join(tempDir, "not_enough.txt")
	content := "x<END>y" // only one separator present
	assert.NoError(t, os.WriteFile(p, []byte(content), 0644))

	_, err := GetFileFingerprintUntilNSeparatorsFromPath(p, "<END>", 2)
	assert.Error(t, err)
	assert.True(t, IsNotEnoughSeparators(err))
}

func TestGetFileFingerprintUntilNSeparators_CRLF(t *testing.T) {
	tempDir := t.TempDir()
	p := filepath.Join(tempDir, "crlf.txt")
	content := "a\r\nb\r\n"
	assert.NoError(t, os.WriteFile(p, []byte(content), 0644))

	fp, err := GetFileFingerprintUntilNSeparatorsFromPath(p, "\r\n", 2)
	assert.NoError(t, err)
	h := sha256.Sum256([]byte(content))
	assert.Equal(t, hex.EncodeToString(h[:]), fp)
}
