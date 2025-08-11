package file_tracker

import "os"

func GetFileIDFromPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	return GetFileID(info)
}
