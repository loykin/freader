//go:build darwin

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
	return fmt.Sprintf("dev:%d-ino:%d-btime:%d",
		stat.Dev, stat.Ino, stat.Birthtimespec.Sec), nil
}
