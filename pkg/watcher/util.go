package watcher

import (
	"os"
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

// deriveGlobRoot returns a root path to start walking for a given include pattern.
// Rules:
//   - If the pattern contains no glob meta, return the cleaned pattern itself (directory or file).
//   - If the pattern contains meta, return the deepest directory path before the first segment with meta.
//     Examples:
//     "/var/log/*.log" -> "/var/log"
//     "logs/**/*.txt" -> "logs"
//     "*.log" -> "."
func deriveGlobRoot(pattern string) string {
	if pattern == "" {
		return ""
	}
	clean := filepath.Clean(pattern)
	// No meta anywhere: walk the path itself (file or directory)
	if !strings.ContainsAny(clean, "*?[") {
		return clean
	}
	// Contains meta: peel segments from the right until segment with meta is removed
	p := clean
	for {
		dir, base := filepath.Split(p)
		if dir == "" && base == "" {
			break
		}
		if base == "" { // trailing separator case (unlikely after Clean, but keep safe)
			p = strings.TrimSuffix(dir, string(filepath.Separator))
			continue
		}
		if hasMeta(base) {
			p = strings.TrimSuffix(dir, string(filepath.Separator))
			continue
		}
		// base has no meta; return the current path 'p' which is the deepest non-meta directory
		return p
	}
	if p == "" {
		return "."
	}
	return p
}

// deriveScanRoots normalizes include patterns into distinct root directories to walk.
// Behavior mirrors logic used in watcher.NewWatcher/scan previously:
// - For globs: use deriveGlobRoot
// - For non-meta paths:
//   - if path exists and is dir -> use it
//   - if path exists and is file -> use its directory
//   - if path does not exist -> use its parent directory (or "." when empty)
//
// - Deduplicate roots; fallback to ["."] when result is empty
func deriveScanRoots(includes []string) []string {
	roots := make([]string, 0)
	if len(includes) > 0 {
		seen := map[string]struct{}{}
		for _, pat := range includes {
			p := filepath.Clean(pat)
			var root string
			if hasMeta(p) {
				root = deriveGlobRoot(p)
			} else {
				if fi, err := os.Stat(p); err == nil {
					if fi.IsDir() {
						root = p
					} else {
						dir := filepath.Dir(p)
						if dir == "" {
							root = "."
						} else {
							root = dir
						}
					}
				} else {
					d := filepath.Dir(p)
					if d == "." || d == "" {
						root = "."
					} else {
						root = d
					}
				}
			}
			if root == "" {
				continue
			}
			clean := filepath.Clean(root)
			if _, ok := seen[clean]; !ok {
				seen[clean] = struct{}{}
				roots = append(roots, clean)
			}
		}
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}
	return roots
}

func hasMeta(s string) bool {
	return strings.ContainsAny(s, "*?[")
}
