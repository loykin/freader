package common

// Sink specifies the minimal interface for a line-forwarding backend.
type Sink interface {
	Enqueue(line string)
	Stop() error
}
