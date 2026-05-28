package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// slowOpener is a pdfOpener stub that blocks inside OpenFile for sleepFor.
// Used by Test_10_5_AC8_OpenFileAndEmitReturnsBeforeParseCompletes to
// verify openFileAndEmitWithWarning returns BEFORE the parse completes
// (AC8 50ms wallclock budget while the goroutine sleeps for 2s).
type slowOpener struct {
	sleepFor    time.Duration
	openCalled  atomic.Bool
	openReached chan struct{}
}

func (s *slowOpener) OpenFile(path string) (*pdfcore.DocumentInfo, error) {
	s.openCalled.Store(true)
	close(s.openReached)
	time.Sleep(s.sleepFor)
	// Return a sentinel error so the dispatched goroutine exits cleanly
	// after the sleep without trying to use a nil DocumentInfo.
	return nil, errors.New("slowOpener: sentinel")
}

func (s *slowOpener) GetTreeRoot(tabID string) (*pdfcore.TreeNode, error) {
	return nil, errors.New("slowOpener: GetTreeRoot not expected")
}

func (s *slowOpener) GetChildren(tabID string, nodeID string) ([]*pdfcore.TreeNode, error) {
	return nil, errors.New("slowOpener: GetChildren not expected")
}

func (s *slowOpener) CloseDocument(tabID string) error {
	return nil
}

// recordingEmitter is an eventEmitter stub that captures emitted events.
// Used to assert document:load-start fires synchronously before
// openFileAndEmitWithWarning returns, while document:opened/document:error
// fires only from the goroutine after the parse completes.
type recordingEmitter struct {
	mu     sync.Mutex
	events []string
}

func (r *recordingEmitter) Emit(name string, data ...any) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, name)
	return true
}

func (r *recordingEmitter) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.events))
	copy(out, r.events)
	return out
}

// Test_10_5_AC8_OpenFileAndEmitReturnsBeforeParseCompletes [P0] AC#8:
// openFileAndEmitWithWarning MUST return within 50ms while the parse is
// still in flight (svc.OpenFile sleeps for 2s in this test). The function
// dispatches the pdfcpu read to a goroutine so the Wails event-dispatch
// goroutine is freed to service window resize / menu clicks during the
// parse. The 50ms ceiling is wallclock budget accounting for race-detector
// overhead, GC pause, and synchronous event-emit dispatch.
//
// Verified at the unit layer via a slow-OpenFile seam (pdfOpener
// interface) and a recording emitter (eventEmitter interface). The
// production types *pdfservice.PDFService and *application.EventManager
// satisfy these interfaces implicitly; this test injects stubs.
func Test_10_5_AC8_OpenFileAndEmitReturnsBeforeParseCompletes(t *testing.T) {
	const parseDuration = 2 * time.Second
	const latencyBudget = 50 * time.Millisecond

	opener := &slowOpener{
		sleepFor:    parseDuration,
		openReached: make(chan struct{}),
	}
	emitter := &recordingEmitter{}

	var wg sync.WaitGroup
	wg.Add(1)

	start := time.Now()
	openFileAndEmitWithWarning(opener, emitter, "/fake/path.pdf", "", &wg)
	elapsed := time.Since(start)

	// AC8 contract: return-time-versus-parse-time. The function MUST
	// return well before the 2s parse completes.
	if elapsed > latencyBudget {
		t.Errorf("[P0] 10-5-AC8: openFileAndEmitWithWarning returned in %v, exceeds %v budget (AC8: returns within 50ms while parse is in flight). Per AC8 flake-handling note: if CI flakes above 50ms, raise to 200ms; never to 0.", elapsed, latencyBudget)
	}

	// The dispatched goroutine MUST be in flight (OpenFile entered) before
	// the function returns. Without this assertion, a buggy implementation
	// that synchronously skipped the OpenFile call entirely would also
	// pass the latency check.
	select {
	case <-opener.openReached:
		// goroutine reached OpenFile and is now sleeping.
	case <-time.After(latencyBudget):
		t.Fatalf("[P0] 10-5-AC8: openFileAndEmitWithWarning returned but the dispatched goroutine did not reach OpenFile within %v -- contract violated (parse should already be in flight)", latencyBudget)
	}

	// document:load-start MUST be emitted synchronously (before return) so
	// the frontend renders the loading indicator without waiting on the
	// goroutine scheduler. This is the spec's "Emit document:load-start
	// synchronously (already does)" contract from the Decision section.
	events := emitter.snapshot()
	if len(events) == 0 || events[0] != "document:load-start" {
		t.Errorf("[P0] 10-5-AC8: expected document:load-start to be emitted synchronously before return; got events=%v", events)
	}

	// document:opened / document:error MUST NOT yet be emitted -- the
	// goroutine is still sleeping inside OpenFile. If either fires before
	// the parse completes, the dispatch shape is broken.
	for _, e := range events[1:] {
		if e == "document:opened" || e == "document:error" {
			t.Errorf("[P0] 10-5-AC8: event %q fired before parse completed -- the pdfcpu read is not actually dispatched to a goroutine", e)
		}
	}

	// Wait for the goroutine to complete so the test does not leak it.
	// The slowOpener returns an error after the sleep, which routes to
	// document:error and then wg.Done(). Drain the full parseDuration plus
	// a small grace window.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// goroutine completed cleanly.
	case <-time.After(parseDuration + 500*time.Millisecond):
		t.Fatalf("[P0] 10-5-AC8: dispatched goroutine did not complete within %v -- wg.Done() not reached", parseDuration+500*time.Millisecond)
	}

	if !opener.openCalled.Load() {
		t.Errorf("[P0] 10-5-AC8: slowOpener.OpenFile was never called -- the dispatched goroutine did not run")
	}
}

// 4.4-UNIT-005 [P2]: extractPDFPaths extracts .pdf paths from args.
//
// AC#2: Given a second instance is launched with args containing PDF paths,
// When extractPDFPaths parses the args,
// Then it returns only the .pdf arguments (case-insensitive extension).
func TestExtractPDFPaths(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "single pdf arg",
			args: []string{"unidoc-pdf-debugger", "/path/to/file.pdf"},
			want: []string{"/path/to/file.pdf"},
		},
		{
			name: "empty args slice",
			args: []string{},
			want: nil,
		},
		{
			name: "nil args slice",
			args: nil,
			want: nil,
		},
		{
			name: "no args beyond binary name",
			args: []string{"unidoc-pdf-debugger"},
			want: nil,
		},
		{
			name: "non-pdf arg",
			args: []string{"unidoc-pdf-debugger", "/path/to/file.txt"},
			want: nil,
		},
		{
			name: "mixed extensions including uppercase PDF",
			args: []string{"unidoc-pdf-debugger", "/a.pdf", "/b.PDF", "/c.txt"},
			want: []string{"/a.pdf", "/b.PDF"},
		},
		{
			name: "multiple pdfs",
			args: []string{"unidoc-pdf-debugger", "/doc1.pdf", "/doc2.pdf"},
			want: []string{"/doc1.pdf", "/doc2.pdf"},
		},
		{
			name: "mixed case extension",
			args: []string{"unidoc-pdf-debugger", "/file.Pdf"},
			want: []string{"/file.Pdf"},
		},
		{
			name: "path with spaces",
			args: []string{"unidoc-pdf-debugger", "/my docs/my file.pdf"},
			want: []string{"/my docs/my file.pdf"},
		},
		{
			name: "arg with no extension",
			args: []string{"unidoc-pdf-debugger", "noext"},
			want: nil,
		},
		{
			name: "bare .pdf extension only",
			args: []string{"unidoc-pdf-debugger", ".pdf"},
			want: []string{".pdf"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPDFPaths(tt.args)
			if len(got) != len(tt.want) {
				t.Fatalf("extractPDFPaths(%v) = %v, want %v", tt.args, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractPDFPaths(%v)[%d] = %q, want %q", tt.args, i, got[i], tt.want[i])
				}
			}
		})
	}
}
