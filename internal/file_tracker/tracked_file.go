package file_tracker

type TrackedFile struct {
	FingerprintStrategy string
	Path                string
	FingerprintSize     int64
	Offset              int64
}
