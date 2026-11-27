// Package atomicfile replaces a file through a temp file in the same directory
// that is renamed over it, so a reader never sees a half-written file.
package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	// RetryDelay is the pause between rename attempts.
	RetryDelay = 20 * time.Millisecond
	// StaleTempAge is how old a leftover temp file must be before a write
	// removes it. A write in flight in another process is milliseconds old.
	StaleTempAge = time.Hour
)

// DefaultAttempts is how many times Write tries the final rename. On Windows
// a reader in another process (or Defender, or the indexer) holding the file
// open fails the rename until it closes the file, which for a small file is
// milliseconds; elsewhere a rename failure is not transient.
var DefaultAttempts = func() int {
	if runtime.GOOS == "windows" {
		return 5
	}
	return 1
}()

// Writer holds the rename step and its attempt count. The zero Writer uses
// os.Rename and DefaultAttempts; tests replace both.
type Writer struct {
	Rename   func(oldpath, newpath string) error
	Attempts int
}

// Write replaces path with data using the zero Writer.
func Write(path string, data []byte, pattern string, dirPerm os.FileMode) error {
	return Writer{}.Write(path, data, pattern, dirPerm)
}

// Write replaces path with data through a temp file named by pattern (the
// os.CreateTemp syntax) in path's directory, which is created with dirPerm
// when missing. Temp files matching pattern older than StaleTempAge, left by a
// process that died mid-write, are removed first, and the temp file is removed
// on every failure path.
func (w Writer) Write(path string, data []byte, pattern string, dirPerm os.FileMode) error {
	rename := w.Rename
	if rename == nil {
		rename = os.Rename
	}
	attempts := w.Attempts
	if attempts <= 0 {
		attempts = DefaultAttempts
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	removeStaleTemps(dir, pattern, time.Now())
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	for i := 1; ; i++ {
		err = rename(tmpPath, path)
		if err == nil || i >= attempts {
			break
		}
		time.Sleep(RetryDelay)
	}
	if err != nil {
		return err
	}
	renamed = true
	return nil
}

// removeStaleTemps deletes files in dir matching pattern whose modification
// time is at least StaleTempAge before now. Errors are ignored.
func removeStaleTemps(dir, pattern string, now time.Time) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if ok, _ := filepath.Match(pattern, e.Name()); !ok || e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) >= StaleTempAge {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
