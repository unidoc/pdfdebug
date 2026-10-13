package pdfcore

import "sync"

// lazyCache holds a value built on first use and kept for its lifetime. The mutex
// covers the build, so concurrent callers share one build. A failed build is
// not cached; the next get retries.
//
// Callers that build from pdfcpu state hold DocumentState.pdfMu around get,
// so the cache mutex is the inner lock, except for the image index, whose
// build takes pdfMu page by page itself (see the DocumentState lock order).
type lazyCache[T any] struct {
	mu    sync.Mutex
	value T
	built bool
}

// get returns the cached value, running build first when nothing is cached.
func (c *lazyCache[T]) get(build func() (T, error)) (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.built {
		return c.value, nil
	}
	v, err := build()
	if err != nil {
		var zero T
		return zero, err
	}
	c.value = v
	c.built = true
	return v, nil
}

// isBuilt reports whether a value is cached.
func (c *lazyCache[T]) isBuilt() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.built
}
