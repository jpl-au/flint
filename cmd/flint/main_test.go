package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// writeTree creates a directory tree of empty files under root.
func writeTree(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("package p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// basenames strips each path to its final element for order-independent
// comparison.
func basenames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	sort.Strings(out)
	return out
}

func TestFindGoFiles(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root,
		"a.go",
		"a_test.go",
		"sub/b.go",
		"sub/notgo.txt",
		".hidden/c.go",
		"testdata/d.go",
		"vendor/e.go",
	)

	t.Run("recursive skips hidden, testdata, vendor, tests", func(t *testing.T) {
		files, err := findGoFiles(root, true, false)
		if err != nil {
			t.Fatal(err)
		}
		got := basenames(files)
		want := []string{"a.go", "b.go"}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("include-tests keeps test files", func(t *testing.T) {
		files, err := findGoFiles(root, true, true)
		if err != nil {
			t.Fatal(err)
		}
		got := basenames(files)
		want := []string{"a.go", "a_test.go", "b.go"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("non-recursive stays in the root directory", func(t *testing.T) {
		files, err := findGoFiles(root, false, false)
		if err != nil {
			t.Fatal(err)
		}
		got := basenames(files)
		if len(got) != 1 || got[0] != "a.go" {
			t.Errorf("got %v, want [a.go]", got)
		}
	})

	// A walk root whose name starts with a dot must not be skipped as a
	// hidden directory: ".." as a pattern, or an explicitly named hidden
	// directory, are deliberate choices by the user.
	t.Run("dot-dot root is walked", func(t *testing.T) {
		t.Chdir(filepath.Join(root, "sub"))
		files, err := findGoFiles("..", true, false)
		if err != nil {
			t.Fatal(err)
		}
		got := basenames(files)
		want := []string{"a.go", "b.go"}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("explicit hidden root is walked", func(t *testing.T) {
		files, err := findGoFiles(filepath.Join(root, ".hidden"), true, false)
		if err != nil {
			t.Fatal(err)
		}
		got := basenames(files)
		if len(got) != 1 || got[0] != "c.go" {
			t.Errorf("got %v, want [c.go]", got)
		}
	})
}
