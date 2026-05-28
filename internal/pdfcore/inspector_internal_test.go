package pdfcore

// Red-phase acceptance tests for Story 10-5 ACs that require package-private
// access to DocumentState's unexported fields:
//   - AC#4: re-Open under same tabID must invoke the prior DocumentState's
//     plainTextLoadCancel before the new entry is inserted.
//   - AC#7: reverse-refs build is deferred to first GetReverseRefs call;
//     `doc.reverseRefs == nil && !doc.revRefsBuildFailed` MUST hold
//     immediately after Open returns.
//
// `package pdfcore` (NOT `pdfcore_test`) is required because:
//   - AC4 writes a wrapping closure directly into prior.plainTextLoadCancel.
//   - AC7 reads doc.reverseRefs and doc.revRefsBuildFailed.

import (
	"path/filepath"
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// AC#4 -- tabID-collision lifecycle: re-Open invokes prior cancel
// ---------------------------------------------------------------------------

// Test_10_5_AC4_OpenSameTabIDReleasesPrior [P0] AC#4:
// When Open(tabID, newPath) is called against a tabID that already has a
// DocumentState with a registered plainTextLoadCancel, the prior cancel func
// is invoked exactly once BEFORE the new entry is inserted, and the new
// DocumentState pointer differs from the prior one.
//
// Failure shape today (baseline at story creation, inspector.go:158-163):
//   ins.mu.Lock()
//   delete(ins.documents, tabID)
//   ins.documents[tabID] = doc
//   ins.mu.Unlock()
// The cancel func is NEVER invoked; counter reads 0; test fails.
//
// Post-fix shape (Story 10-5 Task 6):
//   Open extracts the prior entry under ins.mu and calls closeDocLocked,
//   which acquires plainTextCancelMu and fires the cancel; counter reads 1.
//
// Verification deliberately does NOT depend on an in-flight GetPlainText
// returning context.Canceled (no read is in flight; we install only the
// cancel func) and does NOT use goroutine counts / lsof (flaky / unobservable
// because pdfcpu closes the file synchronously inside ReadContextFile).
func Test_10_5_AC4_OpenSameTabIDReleasesPrior(t *testing.T) {
	ins := NewInspector()
	tabID := "10-5-ac4-tab"

	path1 := filepath.Join("..", "..", "testdata", "minimal.pdf")
	path2 := filepath.Join("..", "..", "testdata", "multipage.pdf")

	if _, err := ins.Open(tabID, path1); err != nil {
		t.Fatalf("[P0] 10-5-AC4: first Open(%q) failed: %v", path1, err)
	}

	prior, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatalf("[P0] 10-5-AC4: GetDocument(prior) failed: %v", err)
	}

	// Install a wrapping cancel func that increments an atomic counter.
	// This stands in for the real cancel registered by an in-flight
	// GetPlainText -- AC4 asserts the LIFECYCLE contract (cancel invoked),
	// not a real plaintext read.
	var cancelCalls atomic.Int32
	prior.plainTextCancelMu.Lock()
	prior.plainTextLoadCancel = func() {
		cancelCalls.Add(1)
	}
	prior.plainTextCancelMu.Unlock()

	if _, err := ins.Open(tabID, path2); err != nil {
		t.Fatalf("[P0] 10-5-AC4: second Open(%q) failed: %v", path2, err)
	}

	got := cancelCalls.Load()
	if got != 1 {
		t.Errorf("[P0] 10-5-AC4: prior.plainTextLoadCancel invoked %d times after re-Open under same tabID -- want exactly 1 (AC4: re-Open must release the prior DocumentState's cancel before inserting the new entry)", got)
	}

	current, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatalf("[P0] 10-5-AC4: GetDocument(current) failed: %v", err)
	}
	if current == prior {
		t.Errorf("[P0] 10-5-AC4: new DocumentState pointer equals prior pointer -- expected re-Open to replace the entry with a fresh DocumentState")
	}
}

// ---------------------------------------------------------------------------
// AC#7 -- reverse-refs build deferred to first GetReverseRefs call
// ---------------------------------------------------------------------------

// Test_10_5_AC7_OpenDoesNotBuildReverseRefs [P0] AC#7:
// Immediately after Inspector.Open returns, doc.reverseRefs MUST be nil and
// doc.revRefsBuildFailed MUST be false. The build is now triggered lazily by
// the first GetReverseRefs call via revBuildOnce.
//
// Failure shape today (baseline at story creation, inspector.go:140-156):
//   Open builds the reverse-ref index eagerly inside safeCall and writes
//   doc.reverseRefs = revMap before returning. The map is therefore NOT nil
//   after Open; the assertion below fires; test fails.
//
// Post-fix shape (Story 10-5 Task 5):
//   Open no longer calls buildReverseRefs; doc.reverseRefs stays nil and
//   doc.revRefsBuildFailed stays false until the first GetReverseRefs call
//   triggers buildReverseRefsOnce via revBuildOnce sync.Once.
func Test_10_5_AC7_OpenDoesNotBuildReverseRefs(t *testing.T) {
	ins := NewInspector()
	tabID := "10-5-ac7-tab"

	path := filepath.Join("..", "..", "testdata", "multipage.pdf")
	if _, err := ins.Open(tabID, path); err != nil {
		t.Fatalf("[P0] 10-5-AC7: Open(%q) failed: %v", path, err)
	}
	t.Cleanup(func() { _ = ins.Close(tabID) })

	doc, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatalf("[P0] 10-5-AC7: GetDocument failed: %v", err)
	}

	if doc.reverseRefs != nil {
		t.Errorf("[P0] 10-5-AC7: doc.reverseRefs is NOT nil immediately after Open -- expected lazy build deferred to first GetReverseRefs call (AC7: revBuildOnce sync.Once)")
	}
	if doc.revRefsBuildFailed {
		t.Errorf("[P0] 10-5-AC7: doc.revRefsBuildFailed = true immediately after Open -- expected false (build has not yet been attempted)")
	}
}

// Test_10_5_AC7_FirstGetReverseRefsTriggersBuild [P0] AC#7:
// The first GetReverseRefs call MUST cause doc.reverseRefs to become
// non-nil (assuming the BFS succeeds on a clean fixture). Subsequent calls
// MUST observe the same populated map (the sync.Once does not re-run).
//
// Pre-fix: doc.reverseRefs is already non-nil after Open, so the
// pre-condition "nil at this point" is violated -- the t.Fatalf at the
// pre-call check fires and the test fails before reaching the post-call
// assertion. That's still a red-phase signal: the lazy-build contract is
// not implemented.
//
// Post-fix: pre-call check passes (nil), post-call assertion passes
// (non-nil).
func Test_10_5_AC7_FirstGetReverseRefsTriggersBuild(t *testing.T) {
	ins := NewInspector()
	tabID := "10-5-ac7-trigger-tab"

	path := filepath.Join("..", "..", "testdata", "multipage.pdf")
	if _, err := ins.Open(tabID, path); err != nil {
		t.Fatalf("[P0] 10-5-AC7: Open(%q) failed: %v", path, err)
	}
	t.Cleanup(func() { _ = ins.Close(tabID) })

	doc, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatalf("[P0] 10-5-AC7: GetDocument failed: %v", err)
	}

	// Pre-condition: build has not yet been triggered.
	if doc.reverseRefs != nil {
		t.Fatalf("[P0] 10-5-AC7: doc.reverseRefs already populated before any GetReverseRefs call -- lazy-build contract violated (AC7)")
	}

	// Trigger build via first GetReverseRefs call. The catalog object is
	// well-known and present in multipage.pdf; the call should succeed.
	if _, err := ins.GetReverseRefs(tabID, "obj:0:1"); err != nil {
		t.Fatalf("[P0] 10-5-AC7: first GetReverseRefs failed: %v", err)
	}

	if doc.reverseRefs == nil && !doc.revRefsBuildFailed {
		t.Errorf("[P0] 10-5-AC7: after first GetReverseRefs, doc.reverseRefs is still nil and doc.revRefsBuildFailed is false -- expected the call to trigger the build (AC7: buildReverseRefsOnce)")
	}
}
