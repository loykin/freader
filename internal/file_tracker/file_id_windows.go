//go:build windows
// +build windows

package file_tracker

import (
	"errors"
	"os"
)

func GetFileID(info os.FileInfo) (string, error) {
	return "", errors.New("unsupported OS: windows")
}
