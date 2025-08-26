package watcher

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHasMeta(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"abc", false},
		{"a*b", true},
		{"file?.txt", true},
		{"[abc]", true},
	}
	for _, c := range cases {
		if got := hasMeta(c.in); got != c.want {
			t.Fatalf("hasMeta(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestIsSubPath(t *testing.T) {
	base := t.TempDir()
	a := filepath.Join(base, "a")
	b := filepath.Join(base, "a", "b")
	c := filepath.Join(base, "c")
	// create dirs
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(c, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// subpath: b is under a
	if !isSubPath(b, a) {
		t.Fatalf("expected %q to be subpath of %q", b, a)
	}
	// same path is not considered subpath (rel == ".")
	if isSubPath(a, a) {
		t.Fatalf("did not expect same path to be subpath")
	}
	// unrelated dir is not subpath
	if isSubPath(c, a) {
		t.Fatalf("did not expect %q to be subpath of %q", c, a)
	}
}

func TestDeriveGlobRoot(t *testing.T) {
	base := t.TempDir()
	logs := filepath.Join(base, "logs")
	nested := filepath.Join(logs, "deep", "inner")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// pattern with meta -> deepest dir before first meta
	pat1 := filepath.Join(logs, "*.log")
	if got := deriveGlobRoot(pat1); filepath.Clean(got) != filepath.Clean(logs) {
		t.Fatalf("deriveGlobRoot(%q)=%q want %q", pat1, got, logs)
	}
	// recursive like logs/**/*.txt -> "logs"
	pat2 := filepath.Join("logs", "**", "*.txt")
	if got := deriveGlobRoot(pat2); filepath.Clean(got) != filepath.Clean("logs") {
		t.Fatalf("deriveGlobRoot(%q)=%q want %q", pat2, got, "logs")
	}
	// pure basename glob -> "."
	if got := deriveGlobRoot("*.log"); got != "." {
		t.Fatalf("deriveGlobRoot(%q)=%q want %q", "*.log", got, ".")
	}
	// no meta -> cleaned path itself
	noMeta := filepath.Join(base, "logs", "deep")
	if got := deriveGlobRoot(noMeta); filepath.Clean(got) != filepath.Clean(noMeta) {
		t.Fatalf("deriveGlobRoot(%q)=%q want %q", noMeta, got, noMeta)
	}
	// empty -> ""
	if got := deriveGlobRoot(""); got != "" {
		t.Fatalf("deriveGlobRoot empty got %q want empty", got)
	}
}

func TestDeriveScanRoots(t *testing.T) {
	base := t.TempDir()
	dir1 := filepath.Join(base, "dir1")
	if err := os.MkdirAll(dir1, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	file1 := filepath.Join(dir1, "a.log")
	if err := os.WriteFile(file1, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// 1) existing directory -> itself
	roots := deriveScanRoots([]string{dir1})
	if len(roots) != 1 || filepath.Clean(roots[0]) != filepath.Clean(dir1) {
		t.Fatalf("roots for dir: %+v, want [%q]", roots, dir1)
	}

	// 2) existing file -> its directory
	roots = deriveScanRoots([]string{file1})
	if len(roots) != 1 || filepath.Clean(roots[0]) != filepath.Dir(file1) {
		t.Fatalf("roots for file: %+v, want [%q]", roots, filepath.Dir(file1))
	}

	// 3) non-existent path -> parent directory (or "." if parent empty)
	nonexist := filepath.Join(dir1, "missing", "x.log")
	wantParent := filepath.Dir(nonexist)
	roots = deriveScanRoots([]string{nonexist})
	if len(roots) != 1 || filepath.Clean(roots[0]) != filepath.Clean(wantParent) {
		t.Fatalf("roots for nonexist: %+v, want [%q]", roots, wantParent)
	}

	// 4) glob under an existing directory -> that directory as root
	glob := filepath.Join(dir1, "*.log")
	roots = deriveScanRoots([]string{glob})
	if len(roots) != 1 || filepath.Clean(roots[0]) != filepath.Clean(dir1) {
		t.Fatalf("roots for glob: %+v, want [%q]", roots, dir1)
	}

	// 5) mix of items and deduplication
	glob2 := filepath.Join(dir1, "a*.log")
	roots = deriveScanRoots([]string{dir1, file1, glob, glob2})
	if len(roots) != 1 || filepath.Clean(roots[0]) != filepath.Clean(dir1) {
		t.Fatalf("dedup roots: %+v, want single %q", roots, dir1)
	}

	// 6) when includes empty -> fallback to ["."]
	roots = deriveScanRoots(nil)
	if len(roots) != 1 || roots[0] != "." {
		t.Fatalf("empty includes fallback got %+v want [.]", roots)
	}
}

func TestDeriveScanRoots_RelativeNonExisting(t *testing.T) {
	// Pattern like "logs/app/*.log" should return "logs/app" root even if dirs don't exist yet.
	pat := filepath.Join("logs", "app", "*.log")
	roots := deriveScanRoots([]string{pat})
	want := filepath.Join("logs", "app")
	if runtime.GOOS == "windows" {
		// On Windows, Clean will normalize separators; just compare Cleaned values
		if filepath.Clean(roots[0]) != filepath.Clean(want) {
			t.Fatalf("got %q want %q", roots[0], want)
		}
		return
	}
	if len(roots) != 1 || roots[0] != want {
		t.Fatalf("got %+v want [%q]", roots, want)
	}
}
