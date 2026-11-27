package pdfcore

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"
)

// Disk states reported by DiskStatus.
const (
	// DiskStateCurrent means the file on disk matches the Open fingerprint.
	DiskStateCurrent = "current"
	// DiskStateChanged means the file on disk differs from the Open fingerprint.
	DiskStateChanged = "changed"
	// DiskStateMissing means no file exists at the opened path any more.
	DiskStateMissing = "missing"
)

// FileFingerprint is the stat-based identity of the opened file, captured at
// Open: size, modification time and file identity.
type FileFingerprint struct {
	Size    int64
	ModTime time.Time
	info    os.FileInfo
}

// newFileFingerprint captures fi. os.SameFile(fi, fi) loads the file ID now:
// on Windows os.Stat stores only the path and loads the ID on the first
// SameFile, which after a rename-over would read the new file's ID.
func newFileFingerprint(fi os.FileInfo) FileFingerprint {
	os.SameFile(fi, fi)
	return FileFingerprint{Size: fi.Size(), ModTime: fi.ModTime(), info: fi}
}

// Matches reports whether fi has the fingerprint's size and mtime and is the
// same file. The mtime must be equal, not newer: restores move it backwards.
func (fp FileFingerprint) Matches(fi os.FileInfo) bool {
	if fp.info == nil || fi == nil {
		return false
	}
	return fi.Size() == fp.Size && fi.ModTime().Equal(fp.ModTime) && os.SameFile(fp.info, fi)
}

// DiskStatus reports how the file on disk compares with the Open fingerprint.
// Times are Unix milliseconds; DiskModTime and DiskSize are 0 when missing.
type DiskStatus struct {
	State         string `json:"state"`
	LoadedModTime int64  `json:"loadedModTime"`
	DiskModTime   int64  `json:"diskModTime"`
	DiskSize      int64  `json:"diskSize"`
}

// openVerified opens path for reading and checks the handle against fp. A
// missing file or a mismatch returns ErrFileChanged (wrapping fs.ErrNotExist
// for a missing file), a file another process holds without read sharing
// returns ErrFileBusy, and other open errors are returned as is. The caller
// closes the returned file.
func openVerified(path string, fp FileFingerprint) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %w", ErrFileChanged, err)
		}
		if isFileBusy(err) {
			return nil, fmt.Errorf("%w: %w", ErrFileBusy, err)
		}
		return nil, err
	}
	if err := verifyHandle(f, fp); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// verifyHandle returns ErrFileChanged when the open handle no longer matches fp.
func verifyHandle(f *os.File, fp FileFingerprint) error {
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if !fp.Matches(fi) {
		return ErrFileChanged
	}
	return nil
}

// TabIDs returns the tab IDs of every open document, in no particular order.
func (ins *Inspector) TabIDs() []string {
	ins.mu.Lock()
	defer ins.mu.Unlock()
	ids := make([]string, 0, len(ins.documents))
	for id := range ins.documents {
		ids = append(ids, id)
	}
	return ids
}

// DiskStatus stats the file behind tabID and compares it with the Open
// fingerprint. ins.mu is held only for the lookup; the stat runs outside every
// lock. A missing file is a status, not an error. Unknown tabs return
// ErrDocumentNotFound; other stat errors are returned as is.
func (ins *Inspector) DiskStatus(tabID string) (DiskStatus, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return DiskStatus{}, err
	}
	st := DiskStatus{LoadedModTime: doc.fingerprint.ModTime.UnixMilli()}
	fi, err := statWithIdentity(doc.FilePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			st.State = DiskStateMissing
			return st, nil
		}
		return DiskStatus{}, err
	}
	st.DiskModTime = fi.ModTime().UnixMilli()
	st.DiskSize = fi.Size()
	if doc.fingerprint.Matches(fi) {
		st.State = DiskStateCurrent
	} else {
		st.State = DiskStateChanged
	}
	return st, nil
}

// CopyDocumentFile writes the bytes of the file behind tabID to w. The handle
// is checked against the Open fingerprint before the first byte and again
// after the last: a file that changed or is gone returns ErrFileChanged
// (before anything is written when the change came first), and a read refused
// by another process's lock returns ErrFileBusy. Any other error is the copy's.
func (ins *Inspector) CopyDocumentFile(tabID string, w io.Writer) (int64, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return 0, err
	}
	f, err := openVerified(doc.FilePath, doc.fingerprint)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	n, err := io.Copy(w, busyReader{f})
	if err != nil {
		return n, err
	}
	return n, verifyHandle(f, doc.fingerprint)
}

// busyReader reports a read refused by another process's lock as ErrFileBusy.
type busyReader struct{ f *os.File }

func (r busyReader) Read(p []byte) (int, error) {
	n, err := r.f.Read(p)
	if err != nil && isFileBusy(err) {
		err = fmt.Errorf("%w: %w", ErrFileBusy, err)
	}
	return n, err
}
