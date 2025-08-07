package watcher

import (
	"path/filepath"
	"strings"
)

func isSubPath(a, b string) bool {
	rel, err := filepath.Rel(b, a)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..")
}
