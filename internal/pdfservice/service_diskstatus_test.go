// GetDiskStatus: the on-demand disk check the frontend uses for Compare's size
// check, immediate marking after a stale read, and seeding a promoted copy.
//
// Run: cd code && go test -run TestGetDiskStatus ./internal/pdfservice/

package pdfservice

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"unidoc-pdf-debugger/internal/filewatch"
	"unidoc-pdf-debugger/internal/pdfcore"
)

func TestGetDiskStatusReportsCurrentThenChanged(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "minimal.pdf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewPDFService(nil)
	info, err := svc.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.CloseDocument(info.TabID) })

	st, err := svc.GetDiskStatus(info.TabID)
	if err != nil {
		t.Fatalf("GetDiskStatus: %v", err)
	}
	if st == nil || st.State != "current" {
		t.Fatalf("fresh open: %+v, want current", st)
	}

	other, err := os.ReadFile(filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, other, 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	st, err = svc.GetDiskStatus(info.TabID)
	if err != nil {
		t.Fatalf("GetDiskStatus after overwrite: %v", err)
	}
	if st.State != "changed" || st.DiskSize != int64(len(other)) {
		t.Errorf("after overwrite: %+v, want changed with diskSize %d", st, len(other))
	}
}

func TestGetDiskStatusUnknownTab(t *testing.T) {
	svc := NewPDFService(nil)
	if _, err := svc.GetDiskStatus("no-such-tab"); !errors.Is(err, pdfcore.ErrDocumentNotFound) {
		t.Errorf("err = %v, want ErrDocumentNotFound", err)
	}
}

// tickClock hands the watcher one shared tick channel the test drives.
type tickClock struct{ ticks chan time.Time }

func (c tickClock) After(time.Duration) <-chan time.Time { return c.ticks }

type chanEmitter chan filewatch.Event

func (e chanEmitter) Emit(ev filewatch.Event) { e <- ev }

// A change GetDiskStatus reported (the frontend marked the tab from it) is the
// watcher's last reported status, so a return to the Open fingerprint emits
// current although the watcher itself never reported the change.
func TestGetDiskStatusResultIsRecordedInTheWatcher(t *testing.T) {
	original, err := os.ReadFile(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "minimal.pdf")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	svc := NewPDFService(nil)
	info, err := svc.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.CloseDocument(info.TabID) })

	clk := tickClock{ticks: make(chan time.Time)}
	events := make(chanEmitter, 8)
	w := WatchDiskStatus(&svc, events, clk)
	w.Start()
	t.Cleanup(w.Stop)

	other, err := os.ReadFile(filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, other, 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := svc.GetDiskStatus(info.TabID)
	if err != nil || st.State != pdfcore.DiskStateChanged {
		t.Fatalf("GetDiskStatus after overwrite: %+v, %v; want changed", st, err)
	}

	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}

	// Tick until the watcher settles on current; a tick that finds the stat
	// still in flight starts nothing new.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev.TabID != info.TabID || ev.State != pdfcore.DiskStateCurrent {
				t.Fatalf("event %+v, want current for %s", ev, info.TabID)
			}
			return
		case clk.ticks <- time.Now():
		case <-deadline:
			t.Fatal("no current event: the GetDiskStatus result was not recorded as the last reported status")
		}
	}
}
