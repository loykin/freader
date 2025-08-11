//go:build windows
// +build windows

package file_tracker

func GetFileID(info os.FileInfo) (string, error) {
	return "", errors.New("unsupported OS: windows")
}
