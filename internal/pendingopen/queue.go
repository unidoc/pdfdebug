// Package pendingopen holds the app-shell queue that buffers OS
// file-association open paths that arrive before the frontend is ready to
// receive document:opened events (cold start). It follows the same isolation
// playbook as internal/splash: pure Go, zero Wails dependencies, no PDF logic
// (it stores raw path strings only), so the Add/Drain contract is unit-testable
// without a Wails runtime. The Wails-side wiring (service binding, callback
// routing) lives in main.go and internal/pdfservice.
//
// The pull model closes the cold-start lost-event race deterministically: the
// frontend drains the queue immediately AFTER registering its document:opened
// listener (the one place subscribe-vs-emit ordering is knowable). The queue
// mutex is the single linearization point -- every path lands on exactly one
// side of the ready flag, and both sides have a working delivery path.
package pendingopen

import "sync"

// Queue is a mutex-guarded buffer of pending file-association open paths.
//
// The zero value is ready to use (not-ready, empty). Before the first Drain
// the queue is in the cold-start state: Add buffers paths and returns false.
// Drain flips the queue ready, returns the buffered paths in insertion order,
// and clears them. Once ready, Add returns true for every path (caller opens
// immediately, the warm path) and never buffers again -- "stays ready" by
// construction, which makes React StrictMode double-effects and dev-mode
// reloads safe (a second Drain is empty).
//
// Safe for concurrent use: the backend callbacks call Add from Wails
// dispatcher goroutines while the bound service method calls Drain from the
// request-handling goroutine.
type Queue struct {
	mu    sync.Mutex
	ready bool
	paths []string
}

// Add records a pending open path.
//
// It returns true when the queue is ready (the frontend has already drained at
// least once): the caller must open the path immediately -- this is the warm
// path. The ready path NEVER dedups and never buffers; repeated ready Adds of
// the same path all return true, because warm duplicate handling belongs to the
// existing frontend re-activation flow (deduping warm opens here would break
// the warm-start contract).
//
// It returns false when the queue is not yet ready (cold start): the path is
// buffered for the next Drain. While buffered, Add dedups on EXACT raw string
// equality so a double-fire of the same file during launch opens it once.
// Dedup is raw-string only -- no filepath.Clean, normalization, or case-folding;
// semantic duplicates (symlinks, case variants) stay the frontend reducer's job,
// exactly as warm opens behave today.
func (q *Queue) Add(path string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.ready {
		return true
	}
	for _, p := range q.paths {
		if p == path {
			// Exact-string duplicate already queued; record once.
			return false
		}
	}
	q.paths = append(q.paths, path)
	return false
}

// Drain marks the queue ready, returns the buffered paths in insertion order,
// and clears the buffer.
//
// It is drain-on-read idempotent: a second Drain returns an empty slice and the
// queue stays ready. Combined with the mutex this guarantees every queued path
// is delivered exactly once across any interleaving of Add/Drain.
func (q *Queue) Drain() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ready = true
	paths := q.paths
	q.paths = nil
	return paths
}
