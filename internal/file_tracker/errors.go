package file_tracker

import (
	"errors"
	"fmt"
)

type FileSizeTooSmallError struct {
	Expected int64
	Actual   int64
}

func (e *FileSizeTooSmallError) Error() string {
	return fmt.Sprintf("expected file size to be greater than %d bytes, got %d bytes", e.Expected, e.Actual)
}

// IsFileSizeTooSmall determines if the provided error is of type FileSizeTooSmallError.
func IsFileSizeTooSmall(err error) bool {
	var sizeErr *FileSizeTooSmallError
	return errors.As(err, &sizeErr)
}

// FirstSeparatorNotFoundError indicates that the file content does not yet contain the specified separator.
type FirstSeparatorNotFoundError struct {
	Separator string
}

func (e *FirstSeparatorNotFoundError) Error() string { return "first separator not found in file" }

// FileLinesTooFewError indicates that the file has fewer lines than required for line-based fingerprinting.
type FileLinesTooFewError struct {
	Expected int
	Actual   int
}

func (e *FileLinesTooFewError) Error() string {
	return fmt.Sprintf("expected file to have at least %d lines, got %d lines", e.Expected, e.Actual)
}

// NotEnoughSeparatorsError indicates fewer separators encountered than required.
type NotEnoughSeparatorsError struct {
	Expected int
	Actual   int
	Sep      string
}

func (e *NotEnoughSeparatorsError) Error() string {
	return fmt.Sprintf("expected at least %d occurrences of separator, got %d", e.Expected, e.Actual)
}

func IsNotEnoughSeparators(err error) bool {
	var e *NotEnoughSeparatorsError
	return errors.As(err, &e)
}
