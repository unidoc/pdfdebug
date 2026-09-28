package pdfcore

import "sync"

// lazyCache holds a value built on first use and kept until reset. The mutex
// covers the build, so concurrent callers share one build. A failed build is
// not cached; the next get retries.
//
// Callers that build from pdfcpu state must hold DocumentState.pdfMu around
// get: the cache mutex is the inner lock.
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

// reset drops the cached value so the next get rebuilds.
func (c *lazyCache[T]) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	var zero T
	c.value = zero
	c.built = false
}

// isBuilt reports whether a value is cached.
func (c *lazyCache[T]) isBuilt() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.built
}
