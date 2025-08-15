package common

import "strings"

// filter applies include/exclude substring filters.
type filter struct {
	includes []string
	excludes []string
}

func (f *filter) allow(line string) bool {
	if len(f.includes) > 0 {
		ok := false
		for _, inc := range f.includes {
			if inc == "" || strings.Contains(line, inc) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for _, exc := range f.excludes {
		if exc != "" && strings.Contains(line, exc) {
			return false
		}
	}
	return true
}
