//go:build windows

package pdfcore

import (
	"os"
	"path/filepath"
	"testing"
)

// The identity is taken when statWithIdentity runs: a file renamed over the
// path afterwards is not the same file, where os.Stat's lazily loaded ID would
// read the new file and match it.
func TestStatWithIdentityCapturesTheFileIDAtStatTime(t *testing.T) {
	path := copyFixtureToTemp(t, "minimal.pdf")
	before, err := statWithIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(path), "other.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(other, path); err != nil {
		t.Fatal(err)
	}
	after, err := statWithIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Errorf("a file renamed over the path compares as the same file")
	}
}
