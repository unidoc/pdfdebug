//go:build mcp

package pdfservice

import (
	"reflect"
	"strings"
	"sync"
	"testing"
)

// drainDialogQueue empties the package-level queue so one test cannot leak
// queued paths into the next.
func drainDialogQueue(t *testing.T) {
	t.Helper()
	_ = takeQueuedDialogPaths()
	t.Cleanup(func() { _ = takeQueuedDialogPaths() })
}

func TestDialogQueueOpenFileDialogReturnsQueuedPaths(t *testing.T) {
	drainDialogQueue(t)
	s := &PDFService{}
	want := []string{"/fixtures/a.pdf", "/fixtures/b.pdf"}
	s.QueueDialogPaths(want)

	got, err := s.OpenFileDialog()
	if err != nil {
		t.Fatalf("OpenFileDialog with a queued path returned an error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("OpenFileDialog = %v, want %v", got, want)
	}
}

func TestDialogQueueIsClearedAfterOneDialog(t *testing.T) {
	drainDialogQueue(t)
	s := &PDFService{}
	s.QueueDialogPaths([]string{"/fixtures/a.pdf"})
	if _, err := s.OpenFileDialog(); err != nil {
		t.Fatalf("first OpenFileDialog: %v", err)
	}

	got, err := s.OpenFileDialog()
	if err == nil {
		t.Fatalf("second OpenFileDialog returned %v and no error; an empty queue must fall through to the real dialog", got)
	}
	if !strings.Contains(err.Error(), "app not initialized") {
		t.Errorf("second OpenFileDialog error = %q, want the real dialog path's %q", err, "app not initialized")
	}
}

func TestDialogQueueEmptyQueueFallsThroughToTheRealDialog(t *testing.T) {
	drainDialogQueue(t)
	s := &PDFService{}
	_, err := s.OpenFileDialog()
	if err == nil || !strings.Contains(err.Error(), "app not initialized") {
		t.Errorf("OpenFileDialog with nothing queued: err = %v, want %q", err, "app not initialized")
	}
}

func TestDialogQueueEmptyListQueuesNothing(t *testing.T) {
	drainDialogQueue(t)
	s := &PDFService{}
	s.QueueDialogPaths(nil)
	s.QueueDialogPaths([]string{})
	_, err := s.OpenFileDialog()
	if err == nil || !strings.Contains(err.Error(), "app not initialized") {
		t.Errorf("OpenFileDialog after queueing an empty list: err = %v, want %q", err, "app not initialized")
	}
}

func TestDialogQueueKeepsItsOwnCopyOfThePaths(t *testing.T) {
	drainDialogQueue(t)
	s := &PDFService{}
	paths := []string{"/fixtures/a.pdf"}
	s.QueueDialogPaths(paths)
	paths[0] = "/fixtures/changed.pdf"

	got, err := s.OpenFileDialog()
	if err != nil {
		t.Fatalf("OpenFileDialog: %v", err)
	}
	if len(got) != 1 || got[0] != "/fixtures/a.pdf" {
		t.Errorf("OpenFileDialog = %v, want the paths as they were when queued", got)
	}
}

func TestDialogQueueReplacesPathsNoDialogUsed(t *testing.T) {
	drainDialogQueue(t)
	s := &PDFService{}
	s.QueueDialogPaths([]string{"/fixtures/a.pdf"})
	s.QueueDialogPaths([]string{"/fixtures/b.pdf"})

	got, err := s.OpenFileDialog()
	if err != nil {
		t.Fatalf("OpenFileDialog: %v", err)
	}
	if want := []string{"/fixtures/b.pdf"}; !reflect.DeepEqual(got, want) {
		t.Errorf("OpenFileDialog = %v, want only the latest queued %v", got, want)
	}
}

func TestDialogQueueWorksThroughAValueCopyOfTheService(t *testing.T) {
	drainDialogQueue(t)
	svc := NewPDFService(nil)
	svc.QueueDialogPaths([]string{"/fixtures/a.pdf"})
	other := svc
	got, err := other.OpenFileDialog()
	if err != nil || len(got) != 1 {
		t.Errorf("OpenFileDialog on a copy of the service = %v, %v; want the path queued on the original", got, err)
	}
}

func TestDialogQueueIsSafeForConcurrentUse(t *testing.T) {
	drainDialogQueue(t)
	s := &PDFService{}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.QueueDialogPaths([]string{"/fixtures/a.pdf"})
		}()
		go func() {
			defer wg.Done()
			_, _ = s.OpenFileDialog()
		}()
	}
	wg.Wait()
}

func TestDialogQueueIsABoundServiceMethod(t *testing.T) {
	m, ok := reflect.TypeOf(&PDFService{}).MethodByName("QueueDialogPaths")
	if !ok {
		t.Fatal("*PDFService has no exported QueueDialogPaths method, so Wails cannot bind it")
	}
	if m.Type.NumIn() != 2 || m.Type.In(1) != reflect.TypeOf([]string(nil)) {
		t.Errorf("QueueDialogPaths signature = %v, want a single []string argument", m.Type)
	}
}
