package file_tracker

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

func GetFileIDFromPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	return GetFileID(info)
}

func GetFileID(info os.FileInfo) (string, error) {
	switch runtime.GOOS {
	case "linux":
		a := info.Sys()
		fmt.Println(a)
		stat, ok := info.Sys().(*unix.Stat_t)
		if !ok {
			return "", fmt.Errorf("failed to get raw stat_t data")
		}
		return fmt.Sprintf("dev:%d-ino:%d-btime:%d",
			stat.Dev, stat.Ino, stat.Btim.Sec), nil
	case "darwin":
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return "", fmt.Errorf("failed to get raw stat_t data")
		}
		return fmt.Sprintf("dev:%d-ino:%d-btime:%d",
			stat.Dev, stat.Ino, stat.Birthtimespec.Sec), nil
	//case "windows":
	//	// currently not support
	//	hash := sha1.New()
	//	hash.Write([]byte(fmt.Sprintf("%s", info.Name())))
	//	return hex.EncodeToString(hash.Sum(nil)), nil
	default:
		return "", errors.New("unsupported OS: " + runtime.GOOS)
	}
}
