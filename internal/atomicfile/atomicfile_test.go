package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestWriteCreatesTheDirectoryAndReplacesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "list.json")
	for _, body := range []string{"first", "second"} {
		if err := Write(path, []byte(body), "list-*.tmp", 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != body {
			t.Fatalf("file = %q, %v; want %q", got, err, body)
		}
	}
	if got := names(t, filepath.Dir(path)); !slices.Equal(got, []string{"list.json"}) {
		t.Errorf("directory holds %v, want only list.json", got)
	}
}

func TestWriteRetriesAFailedRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "list.json")
	calls := 0
	w := Writer{Attempts: 5, Rename: func(oldpath, newpath string) error {
		calls++
		if calls < 3 {
			return errors.New("sharing violation")
		}
		return os.Rename(oldpath, newpath)
	}}
	if err := w.Write(path, []byte("x"), "list-*.tmp", 0o755); err != nil {
		t.Fatalf("Write after two failed renames: %v", err)
	}
	if calls != 3 {
		t.Errorf("rename called %d times, want 3", calls)
	}
}

func TestWriteGivesUpAfterTheLastAttemptAndRemovesTheTempFile(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	w := Writer{Attempts: 3, Rename: func(string, string) error {
		calls++
		return errors.New("sharing violation")
	}}
	if err := w.Write(filepath.Join(dir, "list.json"), []byte("x"), "list-*.tmp", 0o755); err == nil {
		t.Fatal("Write reported success with every rename failing")
	}
	if calls != 3 {
		t.Errorf("rename called %d times, want 3", calls)
	}
	if got := names(t, dir); len(got) != 0 {
		t.Errorf("directory holds %v after a failed write, want nothing", got)
	}
}

func TestWriteRemovesOnlyStaleTempFilesOfItsOwnPattern(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-2 * StaleTempAge)
	files := map[string]time.Time{
		"list-111.tmp":  old,
		"list-222.tmp":  time.Now(),
		"other-333.tmp": old,
	}
	for name, mtime := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	if err := Write(filepath.Join(dir, "list.json"), []byte("x"), "list-*.tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	want := []string{"list-222.tmp", "list.json", "other-333.tmp"}
	if got := names(t, dir); !slices.Equal(got, want) {
		t.Errorf("directory holds %v, want %v", got, want)
	}
}

func TestRenameIsRetriedOnlyOnWindows(t *testing.T) {
	want := 1
	if runtime.GOOS == "windows" {
		want = 5
	}
	if DefaultAttempts != want {
		t.Errorf("DefaultAttempts = %d on %s, want %d", DefaultAttempts, runtime.GOOS, want)
	}
}
