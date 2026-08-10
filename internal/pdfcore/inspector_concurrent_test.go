package pdfcore

// Red-phase acceptance test for Story 10-5 AC2:
// Inspector concurrent-soak under -race detector.
//
// Spec contract (story 10-5 AC2 verbatim):
//   50 goroutines, 1 second, each goroutine randomly picks ONE of the
//   following nine Inspector methods on every iteration and invokes it
//   against the same tabID:
//     GetTreeRoot, GetChildren(tabID,"root"), GetReverseRefs(tabID,"obj:0:2"),
//     GetObjectDetail(tabID,"obj:0:2"), GetXRefTable,
//     GetContentStream(tabID, pageContentNodeID), GetObjectIndex,
//     GetAncestorPath(tabID,"obj:0:2"), GetPageContentStreamNodeID(tabID,1)
//
// Expected behaviour AFTER Story 10-5:
//   - Per-document pdfMu serializes pdfcpu access; the race detector reports
//     no data races across all 10 -count repetitions.
//
// Expected behaviour BEFORE Story 10-5 (red phase):
//   - pdfcpu's XRefTable.Dereference mutates internal state; concurrent calls
//     interleave reads and writes against shared object-stream caches.
//   - `go test -race -count=10 -run TestInspectorConcurrentSoak ./internal/pdfcore/...`
//     reports a WARNING: DATA RACE and the test fails. That failure is the
//     red signal this test is asserting.
//
// The test drives Inspector directly (NOT pdfservice). It bypasses any
// pdfservice-layer recover by design -- pdfcore's safeCall re-panic for
// runtime.Error stays intact (AC6), so a genuine runtime.Error inside pdfcpu
// would crash this test binary. multipage.pdf is a known-clean fixture and
// is not expected to trigger pdfcpu's runtime.Error surface.

import (
	"context"
	"math/rand/v2"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestInspectorConcurrentSoak drives 50 goroutines against the nine
// pdfcpu-touching Inspector methods on a single tabID for one second. Under
// -race it fails unless per-document pdfMu serializes pdfcpu access.
func TestInspectorConcurrentSoak(t *testing.T) {
	if testing.Short() {
		t.Skip("[P0] 10-5-AC2: skipping concurrent soak under -short")
	}

	ins := NewInspector()
	tabID := "10-5-ac2-soak"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("[P0] 10-5-AC2: failed to open multipage.pdf: %v", err)
	}
	t.Cleanup(func() { _ = ins.Close(tabID) })

	// Resolve a content-stream node ID for the AC2 GetContentStream call.
	// multipage.pdf's page-1 has no /Contents entry (see testdata generator
	// for `multipagePDFContent`); we substitute an obj reference that
	// resolves but isn't a stream, so GetContentStream exercises the
	// resolveNodeObject + cache-write path without requiring stream decode.
	// This still drives the streamMu critical section that AC3 hardens and
	// keeps the pdfcpu Dereference call inside the method pool for the
	// race detector to observe.
	pageNodeID, err := ins.GetPageContentStreamNodeID(tabID, 1)
	if err != nil {
		t.Fatalf("[P0] 10-5-AC2: GetPageContentStreamNodeID(1) failed: %v", err)
	}
	if pageNodeID == "" {
		// multipage.pdf has no /Contents on page 1 -- use a known indirect
		// reference (the catalog page 1 itself) so the call still
		// exercises resolveNodeObject + streamMu. GetContentStream returns
		// "node is not a stream object" with a cached entry, which is
		// fine for race-detection purposes.
		pageNodeID = "obj:0:3" // multipage page 1 object
	}

	// The nine method bodies from AC2. Each closure invokes one Inspector
	// method and ignores the result -- the assertion is solely about the
	// race detector and the absence of panics.
	methods := []func(){
		func() { _, _ = ins.GetTreeRoot(tabID) },
		func() { _, _ = ins.GetChildren(tabID, "root") },
		func() { _, _ = ins.GetReverseRefs(tabID, "obj:0:2") },
		func() { _, _ = ins.GetObjectDetail(tabID, "obj:0:2") },
		func() { _, _ = ins.GetXRefTable(tabID) },
		func() { _, _ = ins.GetContentStream(tabID, pageNodeID) },
		func() { _, _ = ins.GetObjectIndex(tabID) },
		func() { _, _ = ins.GetAncestorPath(tabID, "obj:0:2") },
		func() { _, _ = ins.GetPageContentStreamNodeID(tabID, 1) },
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	const goroutineCount = 50
	var wg sync.WaitGroup
	var iterCount atomic.Int64
	wg.Add(goroutineCount)
	for i := 0; i < goroutineCount; i++ {
		go func(seed uint64) {
			defer wg.Done()
			// Each goroutine has its own PRNG so the random pick is itself
			// race-free (math/rand/v2's package-level funcs are race-safe
			// post Go 1.22 but per-goroutine state is cleaner).
			rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
			for ctx.Err() == nil {
				methods[rng.IntN(len(methods))]()
				iterCount.Add(1)
			}
		}(uint64(i) + 1)
	}
	wg.Wait()

	// Sanity floor: the goroutines should have produced at least one
	// iteration each in a 1-second window. A near-zero count means the test
	// harness itself stalled (e.g. ALL methods returned ErrDocumentNotFound
	// before the goroutines started) and the race detector had nothing to
	// observe. Treat that as a fixture/setup failure, NOT a race.
	if got := iterCount.Load(); got < int64(goroutineCount) {
		t.Errorf("[P0] 10-5-AC2: only %d iterations across %d goroutines in 1s -- expected >= %d (one per goroutine). Fixture/setup may be broken.", got, goroutineCount, goroutineCount)
	}
}
