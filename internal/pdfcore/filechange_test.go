// Stale-read guard and on-disk status: reads of the opened file after Open
// refuse bytes that no longer belong to the parsed document, and DiskStatus
// reports how the file on disk compares with the Open-time fingerprint.
//
// Run: cd code && go test -race -run 'TestFileChange|TestDiskStatus|TestTabIDs' ./internal/pdfcore/

package pdfcore

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// copyFixtureToTemp copies testdata/<name> into a fresh temp dir and returns
// the copy's path.
func copyFixtureToTemp(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir(t), name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write temp copy: %v", err)
	}
	return path
}

// openTempCopy opens a temp copy of the named fixture under tabID.
func openTempCopy(t *testing.T, name, tabID string) (*Inspector, string) {
	t.Helper()
	path := copyFixtureToTemp(t, name)
	ins := NewInspector()
	if _, err := ins.Open(tabID, path); err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { _ = ins.Close(tabID) })
	return ins, path
}

// overwriteWithFixture replaces path's content with another fixture's bytes
// (a different size) and moves the mtime an hour ahead.
func overwriteWithFixture(t *testing.T, path, other string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir(t), other))
	if err != nil {
		t.Fatalf("read fixture %s: %v", other, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

// renameOverWithSameBytes writes the file's own bytes to a sibling path, gives
// the sibling the original mtime and renames it over path. Size and mtime
// match the Open fingerprint; only the file identity differs.
func renameOverWithSameBytes(t *testing.T, path string) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	sibling := path + ".new"
	if err := os.WriteFile(sibling, data, 0o644); err != nil {
		t.Fatalf("write sibling: %v", err)
	}
	if err := os.Chtimes(sibling, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatalf("chtimes sibling: %v", err)
	}
	if err := os.Rename(sibling, path); err != nil {
		t.Fatalf("rename over: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after rename: %v", err)
	}
	if after.Size() != fi.Size() || !after.ModTime().Equal(fi.ModTime()) {
		t.Fatalf("rename-over setup must keep size and mtime: size %d->%d mtime %v->%v", fi.Size(), after.Size(), fi.ModTime(), after.ModTime())
	}
}

func TestFileChangeSentinelsCarryFrontendMarkers(t *testing.T) {
	if !strings.Contains(ErrFileChanged.Error(), "file changed on disk") {
		t.Errorf("ErrFileChanged text %q must contain the marker %q", ErrFileChanged.Error(), "file changed on disk")
	}
	if !strings.Contains(ErrFileBusy.Error(), "file is busy") {
		t.Errorf("ErrFileBusy text %q must contain the marker %q", ErrFileBusy.Error(), "file is busy")
	}
}

func TestFileChangePlainTextRefusesOverwrittenFileAndCachesNothing(t *testing.T) {
	ins, path := openTempCopy(t, "minimal.pdf", "tab-pt-overwrite")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	overwriteWithFixture(t, path, "multipage.pdf")
	doc, err := ins.GetPlainText(t.Context(), "tab-pt-overwrite")
	if !errors.Is(err, ErrFileChanged) {
		t.Fatalf("GetPlainText after overwrite: err = %v, want ErrFileChanged", err)
	}
	if doc != nil {
		t.Errorf("GetPlainText after overwrite returned a document; no bytes may be returned")
	}
	if errors.Is(err, ErrMalformedPDF) {
		t.Errorf("ErrFileChanged must not be reclassified as ErrMalformedPDF: %v", err)
	}

	// Restoring the Open-time bytes and mtime makes the next call succeed,
	// which proves the refused read was not cached.
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	doc, err = ins.GetPlainText(t.Context(), "tab-pt-overwrite")
	if err != nil {
		t.Fatalf("GetPlainText after restoring the original file: %v", err)
	}
	if doc.TotalBytes != int64(len(original)) {
		t.Errorf("TotalBytes = %d, want %d (the original file)", doc.TotalBytes, len(original))
	}
}

func TestFileChangePlainTextRefusesDeletedFile(t *testing.T) {
	ins, path := openTempCopy(t, "minimal.pdf", "tab-pt-deleted")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	doc, err := ins.GetPlainText(t.Context(), "tab-pt-deleted")
	if !errors.Is(err, ErrFileChanged) {
		t.Errorf("err = %v, want ErrFileChanged", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want it to wrap fs.ErrNotExist too", err)
	}
	if doc != nil {
		t.Errorf("a deleted file must return no document")
	}
}

// An in-place rewrite that keeps the size, the mtime and the file identity is
// not detected: the guard is stat-only by design. This case renames a new file
// over the path instead, so only the file identity differs.
func TestFileChangePlainTextRefusesRenameOver(t *testing.T) {
	ins, path := openTempCopy(t, "minimal.pdf", "tab-pt-rename")
	renameOverWithSameBytes(t, path)
	_, err := ins.GetPlainText(t.Context(), "tab-pt-rename")
	if !errors.Is(err, ErrFileChanged) {
		t.Errorf("rename-over with equal size and mtime: err = %v, want ErrFileChanged (file identity differs)", err)
	}
}

func TestFileChangePlainTextReturnsCachedResultAfterLaterChange(t *testing.T) {
	ins, path := openTempCopy(t, "minimal.pdf", "tab-pt-cached")
	first, err := ins.GetPlainText(t.Context(), "tab-pt-cached")
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	overwriteWithFixture(t, path, "multipage.pdf")
	second, err := ins.GetPlainText(t.Context(), "tab-pt-cached")
	if err != nil {
		t.Fatalf("a read verified before the change is served from cache: %v", err)
	}
	if second != first {
		t.Errorf("cached result must be returned as is")
	}
}

func TestFileChangeSignaturesRefuseChangedFile(t *testing.T) {
	ins, path := openTempCopy(t, "signed.pdf", "tab-sig-changed")
	overwriteWithFixture(t, path, "minimal.pdf")
	list, err := ins.GetSignatures("tab-sig-changed")
	if !errors.Is(err, ErrFileChanged) {
		t.Fatalf("err = %v, want ErrFileChanged instead of a hole mismatch report", err)
	}
	if list != nil {
		t.Errorf("a changed file must return no signature list, got %+v", list)
	}
}

func TestFileChangeSignaturesRefuseDeletedFile(t *testing.T) {
	ins, path := openTempCopy(t, "signed.pdf", "tab-sig-deleted")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_, err := ins.GetSignatures("tab-sig-deleted")
	if !errors.Is(err, ErrFileChanged) || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want ErrFileChanged wrapping fs.ErrNotExist", err)
	}
}

// beforeFileSpanRead runs right before readFileSpan opens the file, after the
// pre-walk check passed; it rewrites the file to simulate a save landing
// during the walk.
func TestFileChangeSignaturesRefuseChangeDuringWalk(t *testing.T) {
	ins, path := openTempCopy(t, "signed.pdf", "tab-sig-midwalk")
	called := false
	beforeFileSpanRead = func(p string) {
		if called {
			return
		}
		called = true
		later := time.Now().Add(time.Hour)
		if err := os.Chtimes(p, later, later); err != nil {
			t.Errorf("chtimes in hook: %v", err)
		}
	}
	t.Cleanup(func() { beforeFileSpanRead = nil })

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	list, err := ins.GetSignatures("tab-sig-midwalk")
	if !called {
		t.Fatalf("the span-read hook never ran; signed.pdf must reach the hole read")
	}
	if !errors.Is(err, ErrFileChanged) {
		t.Fatalf("change during the walk: err = %v, want ErrFileChanged", err)
	}
	if list != nil {
		t.Errorf("a change during the walk must return no list")
	}

	// The refusal is per call: once the Open-time mtime is back, the next walk
	// reports the signatures again.
	if err := os.Chtimes(path, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	list, err = ins.GetSignatures("tab-sig-midwalk")
	if err != nil {
		t.Fatalf("GetSignatures after restoring the mtime: %v", err)
	}
	if len(list.Signatures) == 0 || !list.Signatures[0].HoleMatchesContents {
		t.Errorf("restored file must report a matching hole again: %+v", list.Signatures)
	}
}

// A document without signed fields never reads the file, so a change on disk
// does not refuse the walk: it reports the same empty list as before.
func TestFileChangeSignaturesIgnoreChangeOnUnsignedFile(t *testing.T) {
	ins, path := openTempCopy(t, "minimal.pdf", "tab-sig-unsigned")
	overwriteWithFixture(t, path, "multipage.pdf")
	list, err := ins.GetSignatures("tab-sig-unsigned")
	if err != nil {
		t.Fatalf("err = %v, want nil for a changed file without signatures", err)
	}
	if list == nil || len(list.Signatures) != 0 {
		t.Errorf("want an empty signature list, got %+v", list)
	}
}

// changeOnFirstErrCtx moves the file's mtime the first time Err is called.
// readPlainText calls Err before each chunk, after the handle passed its
// first check, so the change lands while the read is under way.
type changeOnFirstErrCtx struct {
	context.Context
	t    *testing.T
	path string
	done bool
}

func (c *changeOnFirstErrCtx) Err() error {
	if !c.done {
		c.done = true
		later := time.Now().Add(time.Hour)
		if err := os.Chtimes(c.path, later, later); err != nil {
			c.t.Errorf("chtimes during read: %v", err)
		}
	}
	return c.Context.Err()
}

func TestFileChangePlainTextRefusesChangeDuringRead(t *testing.T) {
	path := copyFixtureToTemp(t, "minimal.pdf")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &changeOnFirstErrCtx{Context: t.Context(), t: t, path: path}
	doc, err := readPlainText(ctx, path, fi.Size(), newFileFingerprint(fi), "tab-pt-during")
	if !ctx.done {
		t.Fatalf("the read never checked its context; the change was not injected")
	}
	if !errors.Is(err, ErrFileChanged) {
		t.Fatalf("change during the read: err = %v, want ErrFileChanged", err)
	}
	if doc != nil {
		t.Errorf("a change during the read must return no document")
	}
}

func TestFileChangeSignaturesUnchangedFileReportsAsBefore(t *testing.T) {
	ins, _ := openTempCopy(t, "signed.pdf", "tab-sig-same")
	got, err := ins.GetSignatures("tab-sig-same")
	if err != nil {
		t.Fatalf("unchanged file: %v", err)
	}
	ref := NewInspector()
	if _, err := ref.Open("ref", filepath.Join(testdataDir(t), "signed.pdf")); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ref.Close("ref") }()
	want, err := ref.GetSignatures("ref")
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("unchanged copy reports differently from the fixture:\n got %s\nwant %s", gotJSON, wantJSON)
	}
	if len(got.Signatures) == 0 || !got.Signatures[0].HoleMatchesContents {
		t.Errorf("signed.pdf must still report a matching hole: %+v", got.Signatures)
	}
}

func TestTabIDsListsOpenDocuments(t *testing.T) {
	ins := NewInspector()
	for _, id := range []string{"tab-a", "tab-b"} {
		if _, err := ins.Open(id, filepath.Join(testdataDir(t), "minimal.pdf")); err != nil {
			t.Fatal(err)
		}
	}
	ids := ins.TabIDs()
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"tab-a", "tab-b"}) {
		t.Errorf("TabIDs = %v, want [tab-a tab-b]", ids)
	}
	_ = ins.Close("tab-a")
	if ids := ins.TabIDs(); !slices.Equal(ids, []string{"tab-b"}) {
		t.Errorf("TabIDs after closing tab-a = %v, want [tab-b]", ids)
	}
	_ = ins.Close("tab-b")
}

func TestDiskStatusCurrentChangedMissing(t *testing.T) {
	ins, path := openTempCopy(t, "minimal.pdf", "tab-ds")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	loadedMs := fi.ModTime().UnixMilli()

	st, err := ins.DiskStatus("tab-ds")
	if err != nil {
		t.Fatalf("DiskStatus: %v", err)
	}
	if st.State != "current" || st.LoadedModTime != loadedMs || st.DiskModTime != loadedMs || st.DiskSize != fi.Size() {
		t.Errorf("fresh open: %+v, want current with loaded=disk=%d size=%d", st, loadedMs, fi.Size())
	}

	overwriteWithFixture(t, path, "multipage.pdf")
	nfi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	st, err = ins.DiskStatus("tab-ds")
	if err != nil {
		t.Fatalf("DiskStatus after overwrite: %v", err)
	}
	if st.State != "changed" || st.LoadedModTime != loadedMs || st.DiskModTime != nfi.ModTime().UnixMilli() || st.DiskSize != nfi.Size() {
		t.Errorf("after overwrite: %+v, want changed loaded=%d disk=%d size=%d", st, loadedMs, nfi.ModTime().UnixMilli(), nfi.Size())
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	st, err = ins.DiskStatus("tab-ds")
	if err != nil {
		t.Fatalf("a missing file is a status, not an error: %v", err)
	}
	if st.State != "missing" || st.DiskModTime != 0 || st.DiskSize != 0 || st.LoadedModTime != loadedMs {
		t.Errorf("after delete: %+v, want missing with disk time and size 0", st)
	}
}

func TestDiskStatusDetectsRenameOver(t *testing.T) {
	ins, path := openTempCopy(t, "minimal.pdf", "tab-ds-rename")
	renameOverWithSameBytes(t, path)
	st, err := ins.DiskStatus("tab-ds-rename")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "changed" {
		t.Errorf("rename-over: state %q, want changed", st.State)
	}
}

// An in-place append whose mtime is set back to the Open value (a copy tool
// that preserves times, or a write inside the filesystem's mtime granularity)
// keeps the file identity and the mtime; only the size differs.
func TestFileChangeSizeOnlyChangeIsDetected(t *testing.T) {
	ins, path := openTempCopy(t, "minimal.pdf", "tab-size-only")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("%appended\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(fi, after) || !after.ModTime().Equal(fi.ModTime()) || after.Size() == fi.Size() {
		t.Fatalf("setup must keep identity and mtime and change only the size: size %d->%d mtime %v->%v", fi.Size(), after.Size(), fi.ModTime(), after.ModTime())
	}

	st, err := ins.DiskStatus("tab-size-only")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != DiskStateChanged || st.DiskSize != after.Size() {
		t.Errorf("DiskStatus after a size-only change: %+v, want changed with diskSize %d", st, after.Size())
	}
	if _, err := ins.GetPlainText(t.Context(), "tab-size-only"); !errors.Is(err, ErrFileChanged) {
		t.Errorf("GetPlainText after a size-only change: err = %v, want ErrFileChanged", err)
	}
}

// A stat that fails for a reason other than a missing file (here a directory
// the process cannot search) is an error, not "missing": the file may still be
// there, so the tab must not be marked as gone.
func TestDiskStatusUnreadableDirectoryIsAnErrorNotMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory search permission is a Unix mode bit")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	ins, path := openTempCopy(t, "minimal.pdf", "tab-ds-eacces")
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	st, err := ins.DiskStatus("tab-ds-eacces")
	if err == nil {
		t.Fatalf("DiskStatus with an unsearchable directory: %+v, want an error", st)
	}
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("err = %v, want a permission error, not not-exist or unknown tab", err)
	}
	if st.State == DiskStateMissing {
		t.Errorf("state = %q; a stat error must not report the file as missing", st.State)
	}
}

func TestDiskStatusUnknownTab(t *testing.T) {
	ins := NewInspector()
	if _, err := ins.DiskStatus("nope"); !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("err = %v, want ErrDocumentNotFound", err)
	}
}

func TestDiskStatusJSONShape(t *testing.T) {
	ins, _ := openTempCopy(t, "minimal.pdf", "tab-ds-json")
	st, err := ins.DiskStatus("tab-ds-json")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"state", "loadedModTime", "diskModTime", "diskSize"} {
		if _, ok := m[key]; !ok {
			t.Errorf("DiskStatus JSON %s lacks %q", raw, key)
		}
	}
	for _, key := range []string{"loadedModTime", "diskModTime", "diskSize"} {
		if _, ok := m[key].(float64); !ok {
			t.Errorf("%s must be a plain number (Unix ms / bytes), got %T in %s", key, m[key], raw)
		}
	}
}

// A save that lands while Open parses makes Open parse again, so the document
// and its fingerprint describe the same file and it opens as current.
func TestOpenParsesAgainWhenTheFileChangesDuringTheParse(t *testing.T) {
	path := copyFixtureToTemp(t, "minimal.pdf")
	calls := 0
	afterOpenParse = func(p string) {
		calls++
		if calls == 1 {
			overwriteWithFixture(t, p, "multipage.pdf")
		}
	}
	t.Cleanup(func() { afterOpenParse = nil })

	ins := NewInspector()
	info, err := ins.Open("tab-open-race", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ins.Close("tab-open-race") })
	if calls != 2 {
		t.Errorf("parses = %d, want 2 (one more after the change)", calls)
	}

	ref := NewInspector()
	want, err := ref.Open("tab-ref", filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ref.Close("tab-ref") })
	if info.PageCount != want.PageCount {
		t.Errorf("page count %d, want %d from the file saved during the parse", info.PageCount, want.PageCount)
	}
	st, err := ins.DiskStatus("tab-open-race")
	if err != nil || st.State != DiskStateCurrent {
		t.Errorf("DiskStatus after Open = %+v, %v; want current", st, err)
	}
}

// A file that keeps changing through every parse opens after openAttempts
// parses with the stat taken before the last one, so it reads as changed.
func TestOpenOfAFileStillBeingWrittenOpensAsChanged(t *testing.T) {
	path := copyFixtureToTemp(t, "minimal.pdf")
	calls := 0
	afterOpenParse = func(p string) {
		calls++
		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString("\n")
		_ = f.Close()
		if err != nil {
			t.Fatal(err)
		}
		later := time.Now().Add(time.Duration(calls) * time.Hour)
		if err := os.Chtimes(p, later, later); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { afterOpenParse = nil })

	ins := NewInspector()
	if _, err := ins.Open("tab-open-busy-write", path); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ins.Close("tab-open-busy-write") })
	if calls != openAttempts {
		t.Errorf("parses = %d, want %d", calls, openAttempts)
	}
	st, err := ins.DiskStatus("tab-open-busy-write")
	if err != nil || st.State != DiskStateChanged {
		t.Errorf("DiskStatus after Open = %+v, %v; want changed", st, err)
	}
}
