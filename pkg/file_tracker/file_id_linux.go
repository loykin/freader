//go:build linux

package file_tracker

import (
	"fmt"
	"os"
	"syscall"
)

func GetFileID(info os.FileInfo) (string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("failed to get raw stat_t data")
	}
	// Linux는 dev + ino만 사용
	return fmt.Sprintf("dev:%d-ino:%d", stat.Dev, stat.Ino), nil
}
