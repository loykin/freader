package file_tracker

import "sync"

type FileTracker struct {
	info  map[string]TrackedFile
	mutex sync.Mutex
}

func New() *FileTracker {
	info := make(map[string]TrackedFile)
	return &FileTracker{
		info: info,
	}
}

// Add adds a file to the tracker with the specified parameters
// For backward compatibility, the offset parameter is optional and defaults to 0
func (f *FileTracker) Add(fileId string, path string, fingerprintStrategy string, fingerprintSize int64, offset ...int64) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// Default offset is 0 if not provided
	var fileOffset int64
	if len(offset) > 0 {
		fileOffset = offset[0]
	}

	f.info[fileId] = TrackedFile{
		Path:                path,
		FingerprintStrategy: fingerprintStrategy,
		FingerprintSize:     fingerprintSize,
		Offset:              fileOffset,
	}
}

func (f *FileTracker) Get(fileId string) *TrackedFile {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if info, ok := f.info[fileId]; ok {
		return &info
	}

	return nil
}

func (f *FileTracker) Remove(fileId string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	delete(f.info, fileId)
}

func (f *FileTracker) UpdateOffset(fileId string, offset int64) bool {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if file, exists := f.info[fileId]; exists {
		file.Offset = offset
		f.info[fileId] = file
		return true
	}
	return false
}

func (f *FileTracker) GetAllFiles() map[string]TrackedFile {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	result := make(map[string]TrackedFile, len(f.info))
	for id, file := range f.info {
		result[id] = file
	}
	return result
}
