// Story 12.1: internal/pendingopen.Queue Add/Drain contract.
//
// These tests pin the queue's public API through a harness package compiled
// inside the main module (see helpers_test.go for why direct import is not
// possible from a standalone test module).
//
// Contract under test (from the story ACs):
//   - Add(path) bool: true == ready (caller opens immediately, warm path);
//     false == queued for later drain (cold path).
//   - Drain() []string: marks ready, returns queued paths in insertion order,
//     clears the queue. Drain-on-read idempotent: a second Drain returns empty
//     and stays ready.
//   - While queued, Add dedups on EXACT raw string equality (no Clean/fold).
//   - Once ready, Add returns true for EVERY path including repeats (no
//     ready-path dedup -- deduping warm opens here would break).
//   - Every queued path delivered exactly once across any Add/Drain
//     interleaving, verified under -race.
package story_12_1_test

import "testing"

// queueHarnessPreamble is the shared import + alias block for the queue
// harness package. The Queue zero value must be usable (Task 1.1 pins
// mu/ready/paths private fields with no constructor required).
const queueHarnessPreamble = `package atdd

import (
	"testing"

	"unidoc-pdf-debugger/internal/pendingopen"
)
`

// queueHarnessPreambleSync is the variant for the concurrency test, which also
// needs sync. Kept separate so the non-concurrent harnesses do not import sync
// unused (a build error once the production package exists).
const queueHarnessPreambleSync = `package atdd

import (
	"sync"
	"testing"

	"unidoc-pdf-debugger/internal/pendingopen"
)
`

// TestColdAddQueuesReturnsFalse asserts that before any Drain, Add returns false
// (path queued, not opened) and the path is delivered by the subsequent Drain.
func TestColdAddQueuesReturnsFalse(t *testing.T) {
	out, err := runHarness(t, map[string]string{
		"harness_test.go": queueHarnessPreamble + `
func TestColdAddQueues(t *testing.T) {
	var q pendingopen.Queue
	if got := q.Add("/a.pdf"); got != false {
		t.Fatalf("cold Add must return false (queued), got %v", got)
	}
	paths := q.Drain()
	if len(paths) != 1 || paths[0] != "/a.pdf" {
		t.Fatalf("Drain must deliver the queued path, got %#v", paths)
	}
}
`,
	})
	if err != nil {
		t.Fatalf("internal/pendingopen harness failed:\n%s", out)
	}
}

// TestWarmAddReturnsTrueNoQueue asserts that once Drain has marked the queue
// ready, Add returns true (caller opens immediately) and does NOT queue -- the
// next Drain stays empty.
func TestWarmAddReturnsTrueNoQueue(t *testing.T) {
	out, err := runHarness(t, map[string]string{
		"harness_test.go": queueHarnessPreamble + `
func TestWarmAddImmediate(t *testing.T) {
	var q pendingopen.Queue
	q.Drain() // flip to ready
	if got := q.Add("/warm.pdf"); got != true {
		t.Fatalf("warm Add must return true (open immediately), got %v", got)
	}
	if paths := q.Drain(); len(paths) != 0 {
		t.Fatalf("ready-path Add must NOT queue, Drain got %#v", paths)
	}
}
`,
	})
	if err != nil {
		t.Fatalf("internal/pendingopen harness failed:\n%s", out)
	}
}

// TestDrainInsertionOrder asserts Drain returns queued paths in insertion order.
func TestDrainInsertionOrder(t *testing.T) {
	out, err := runHarness(t, map[string]string{
		"harness_test.go": queueHarnessPreamble + `
func TestInsertionOrder(t *testing.T) {
	var q pendingopen.Queue
	q.Add("/1.pdf")
	q.Add("/2.pdf")
	q.Add("/3.pdf")
	paths := q.Drain()
	want := []string{"/1.pdf", "/2.pdf", "/3.pdf"}
	if len(paths) != len(want) {
		t.Fatalf("want %d paths, got %#v", len(want), paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("insertion order broken at %d: want %q got %q (%#v)", i, want[i], paths[i], paths)
		}
	}
}
`,
	})
	if err != nil {
		t.Fatalf("internal/pendingopen harness failed:\n%s", out)
	}
}

// TestSecondDrainEmptyStaysReady asserts a second Drain returns an empty slice and
// the queue stays ready (Add still returns true). That drain-on-read idempotency
// is what makes StrictMode double-effects and dev-mode reloads safe.
func TestSecondDrainEmptyStaysReady(t *testing.T) {
	out, err := runHarness(t, map[string]string{
		"harness_test.go": queueHarnessPreamble + `
func TestSecondDrainEmpty(t *testing.T) {
	var q pendingopen.Queue
	q.Add("/a.pdf")
	first := q.Drain()
	if len(first) != 1 {
		t.Fatalf("first Drain should deliver 1 path, got %#v", first)
	}
	second := q.Drain()
	if len(second) != 0 {
		t.Fatalf("second Drain must be empty (drain-on-read), got %#v", second)
	}
	if got := q.Add("/after.pdf"); got != true {
		t.Fatalf("queue must STAY ready after drain, Add got %v", got)
	}
}
`,
	})
	if err != nil {
		t.Fatalf("internal/pendingopen harness failed:\n%s", out)
	}
}

// TestQueuedDedupExactRawString asserts that while queued, the same raw path is
// recorded once, so a double-fire of the same file during launch opens it once.
// Dedup is EXACT string equality -- semantic variants (a trailing slash, a
// distinct casing) are NOT deduped; that stays the frontend reducer's job.
func TestQueuedDedupExactRawString(t *testing.T) {
	out, err := runHarness(t, map[string]string{
		"harness_test.go": queueHarnessPreamble + `
func TestQueuedDedup(t *testing.T) {
	var q pendingopen.Queue
	q.Add("/dup.pdf")
	q.Add("/dup.pdf") // exact repeat while queued -> recorded once
	q.Add("/Dup.pdf") // case variant -> NOT a dedup, distinct raw string
	paths := q.Drain()
	want := []string{"/dup.pdf", "/Dup.pdf"}
	if len(paths) != len(want) {
		t.Fatalf("exact-string dedup expected %#v, got %#v", want, paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("want %#v got %#v", want, paths)
		}
	}
}
`,
	})
	if err != nil {
		t.Fatalf("internal/pendingopen harness failed:\n%s", out)
	}
}

// TestReadyPathNeverDedups asserts that once ready, Add returns true for EVERY
// path INCLUDING repeats. The ready path must never dedup: warm duplicate
// handling belongs to the frontend re-activation flow, and deduping warm opens in
// the queue would break the immediate-open contract. This is the warm-start
// regression guard at the queue layer.
func TestReadyPathNeverDedups(t *testing.T) {
	out, err := runHarness(t, map[string]string{
		"harness_test.go": queueHarnessPreamble + `
func TestReadyNoDedup(t *testing.T) {
	var q pendingopen.Queue
	q.Drain() // ready
	if got := q.Add("/same.pdf"); got != true {
		t.Fatalf("ready Add #1 must return true, got %v", got)
	}
	if got := q.Add("/same.pdf"); got != true {
		t.Fatalf("ready Add #2 (repeat) must ALSO return true (no ready-path dedup), got %v", got)
	}
	// Ready Adds must not accumulate in the queue.
	if paths := q.Drain(); len(paths) != 0 {
		t.Fatalf("ready Adds must not queue, Drain got %#v", paths)
	}
}
`,
	})
	if err != nil {
		t.Fatalf("internal/pendingopen harness failed:\n%s", out)
	}
}

// TestQueueDeliversEachPathExactlyOnceUnderRace asserts that across concurrent Add
// calls and a single Drain, every queued path is delivered exactly once -- no
// loss, no duplication. The harness runs under `go test -race` (runHarness passes
// -race), so a data race on the mutex-guarded queue fails the run.
//
// The harness fires N concurrent Adds, then a Drain, then more Adds (which may
// land on either side of ready). It asserts that the union of drained paths plus
// post-drain ready-true Adds covers every input exactly once, and that no path
// appears twice in the drained slice.
func TestQueueDeliversEachPathExactlyOnceUnderRace(t *testing.T) {
	out, err := runHarness(t, map[string]string{
		"harness_test.go": queueHarnessPreambleSync + `
func TestExactlyOnceUnderRace(t *testing.T) {
	const n = 200
	var q pendingopen.Queue

	inputs := make([]string, n)
	for i := 0; i < n; i++ {
		inputs[i] = "/race/" + string(rune('a'+(i%26))) + "-" + itoa(i) + ".pdf"
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	openedImmediately := map[string]int{} // Add returned true (ready path)

	// Concurrent Adds.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			if q.Add(p) {
				mu.Lock()
				openedImmediately[p]++
				mu.Unlock()
			}
		}(inputs[i])
	}

	// Concurrent Drain happening alongside the Adds. Drain may run before,
	// during, or after some Adds -- the linearization point is the mutex.
	wg.Add(1)
	drained := map[string]int{}
	go func() {
		defer wg.Done()
		for _, p := range q.Drain() {
			drained[p]++
		}
	}()

	wg.Wait()

	// A late drain mops up anything queued after the concurrent drain ran.
	for _, p := range q.Drain() {
		drained[p]++
	}

	// No path may appear twice in the drained set.
	for p, c := range drained {
		if c != 1 {
			t.Fatalf("path %q drained %d times (want exactly once)", p, c)
		}
	}
	// Every input is accounted for exactly once: either drained OR opened
	// immediately via a ready-path Add, never both, never neither.
	for _, p := range inputs {
		total := drained[p] + openedImmediately[p]
		if total != 1 {
			t.Fatalf("path %q delivered %d times across drain+immediate (want exactly once)", p, total)
		}
	}
}

// itoa avoids importing strconv in the harness preamble.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
`,
	})
	if err != nil {
		t.Fatalf("internal/pendingopen harness failed (a -race violation also lands here):\n%s", out)
	}
}
