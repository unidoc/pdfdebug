package pdfcore

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLazyCacheBuildsOnceAndReturnsTheCachedValue(t *testing.T) {
	var c lazyCache[[]int]
	builds := 0
	build := func() ([]int, error) { builds++; return []int{1, 2}, nil }
	a, err := c.get(build)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := c.get(build)
	if builds != 1 || &a[0] != &b[0] {
		t.Errorf("builds = %d, want 1 and the same slice back", builds)
	}
}

func TestLazyCacheDoesNotCacheErrors(t *testing.T) {
	var c lazyCache[int]
	boom := errors.New("boom")
	if _, err := c.get(func() (int, error) { return 7, boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if c.isBuilt() {
		t.Fatal("a failed build was cached")
	}
	v, err := c.get(func() (int, error) { return 3, nil })
	if err != nil || v != 3 {
		t.Errorf("retry after a failure: %d, %v; want 3, nil", v, err)
	}
}

func TestLazyCacheConcurrentGetsShareOneBuild(t *testing.T) {
	var c lazyCache[int]
	var builds atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.get(func() (int, error) { builds.Add(1); return 1, nil })
		}()
	}
	wg.Wait()
	if builds.Load() != 1 {
		t.Errorf("builds = %d, want 1", builds.Load())
	}
}
