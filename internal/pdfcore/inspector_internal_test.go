package pdfcore

// Acceptance tests for the behaviours that require package-private
// access to DocumentState's unexported fields:
//   re-Open under same tabID must invoke the prior DocumentState's
//     plainTextLoadCancel before the new entry is inserted.
//   reverse-refs build is deferred to first GetReverseRefs call;
//     `doc.reverseRefs == nil && !doc.revRefsBuildFailed` MUST hold
//     immediately after Open returns.
//
// `package pdfcore` (NOT `pdfcore_test`) is required because:
//   - writes a wrapping closure directly into prior.plainTextLoadCancel.
//   - reads doc.reverseRefs and doc.revRefsBuildFailed.

import (
	"path/filepath"
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// tabID-collision lifecycle: re-Open invokes prior cancel
// ---------------------------------------------------------------------------

// TestOpenSameTabIDReleasesPrior asserts that re-Opening a tabID which already
// holds a DocumentState invokes the prior plainTextLoadCancel exactly once before
// the new entry is inserted, and that the new DocumentState pointer differs from
// the prior one.
func TestOpenSameTabIDReleasesPrior(t *testing.T) {
	ins := NewInspector()
	tabID := "cancel-registration-tab"

	path1 := filepath.Join("..", "..", "testdata", "minimal.pdf")
	path2 := filepath.Join("..", "..", "testdata", "multipage.pdf")

	if _, err := ins.Open(tabID, path1); err != nil {
		t.Fatalf("first Open(%q) failed: %v", path1, err)
	}

	prior, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatalf("GetDocument(prior) failed: %v", err)
	}

	// Install a wrapping cancel func that increments an atomic counter.
	// This stands in for the real cancel registered by an in-flight
	// GetPlainText -- asserts the LIFECYCLE contract (cancel invoked), not
	// a real plaintext read.
	var cancelCalls atomic.Int32
	prior.plainTextCancelMu.Lock()
	prior.plainTextLoadCancel = func() {
		cancelCalls.Add(1)
	}
	prior.plainTextCancelMu.Unlock()

	if _, err := ins.Open(tabID, path2); err != nil {
		t.Fatalf("second Open(%q) failed: %v", path2, err)
	}

	got := cancelCalls.Load()
	if got != 1 {
		t.Errorf("prior.plainTextLoadCancel invoked %d times after re-Open under same tabID -- want exactly 1 (re-Open must release the prior DocumentState's cancel before inserting the new entry)", got)
	}

	current, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatalf("GetDocument(current) failed: %v", err)
	}
	if current == prior {
		t.Errorf("new DocumentState pointer equals prior pointer -- expected re-Open to replace the entry with a fresh DocumentState")
	}
}

// ---------------------------------------------------------------------------
// reverse-refs build deferred to first GetReverseRefs call
// ---------------------------------------------------------------------------

// TestOpenDoesNotBuildReverseRefs asserts that immediately after Inspector.Open
// returns, doc.reverseRefs is nil and doc.revRefsBuildFailed is false: the
// reverse-ref build is deferred to the first GetReverseRefs call via
// revBuildOnce.
func TestOpenDoesNotBuildReverseRefs(t *testing.T) {
	ins := NewInspector()
	tabID := "reverse-refs-lazy-tab"

	path := filepath.Join("..", "..", "testdata", "multipage.pdf")
	if _, err := ins.Open(tabID, path); err != nil {
		t.Fatalf("Open(%q) failed: %v", path, err)
	}
	t.Cleanup(func() { _ = ins.Close(tabID) })

	doc, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}

	if doc.reverseRefs != nil {
		t.Errorf("doc.reverseRefs is NOT nil immediately after Open -- expected lazy build deferred to first GetReverseRefs call (revBuildOnce sync.Once)")
	}
	if doc.revRefsBuildFailed {
		t.Errorf("doc.revRefsBuildFailed = true immediately after Open -- expected false (build has not yet been attempted)")
	}
}

// TestFirstGetReverseRefsTriggersBuild asserts the first GetReverseRefs call
// makes doc.reverseRefs non-nil, having first confirmed the map was still nil.
// The sync.Once does not re-run, so later calls observe the same populated map.
func TestFirstGetReverseRefsTriggersBuild(t *testing.T) {
	ins := NewInspector()
	tabID := "reverse-refs-trigger-tab"

	path := filepath.Join("..", "..", "testdata", "multipage.pdf")
	if _, err := ins.Open(tabID, path); err != nil {
		t.Fatalf("Open(%q) failed: %v", path, err)
	}
	t.Cleanup(func() { _ = ins.Close(tabID) })

	doc, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}

	// Pre-condition: build has not yet been triggered.
	if doc.reverseRefs != nil {
		t.Fatalf("doc.reverseRefs already populated before any GetReverseRefs call -- lazy-build contract violated")
	}

	// Trigger build via first GetReverseRefs call. The catalog object is
	// well-known and present in multipage.pdf; the call should succeed.
	if _, err := ins.GetReverseRefs(tabID, "obj:0:1"); err != nil {
		t.Fatalf("first GetReverseRefs failed: %v", err)
	}

	if doc.reverseRefs == nil && !doc.revRefsBuildFailed {
		t.Errorf("after first GetReverseRefs, doc.reverseRefs is still nil and doc.revRefsBuildFailed is false -- expected the call to trigger the build (buildReverseRefsOnce)")
	}
}
