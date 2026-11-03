package main

import (
	"sync/atomic"
	"testing"
	"time"
)

type flushCounts struct {
	saves, rebuilds atomic.Int32
}

func newCountingFlusher(delay time.Duration, saveTakes time.Duration) (*recentFlusher, *flushCounts) {
	c := &flushCounts{}
	f := newRecentFlusher(delay)
	f.save = func() {
		time.Sleep(saveTakes)
		c.saves.Add(1)
	}
	f.rebuild = func() { c.rebuilds.Add(1) }
	return f, c
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 2s")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRecentFlushMergesQuickOpensIntoOneSave(t *testing.T) {
	f, c := newCountingFlusher(30*time.Millisecond, 0)
	for range 5 {
		f.Record()
	}
	waitFor(t, func() bool { return c.rebuilds.Load() == 1 })
	time.Sleep(60 * time.Millisecond)
	if c.saves.Load() != 1 || c.rebuilds.Load() != 1 {
		t.Fatalf("saves=%d rebuilds=%d, want 1 and 1", c.saves.Load(), c.rebuilds.Load())
	}
}

func TestRecentFlushSavesABatchOnceHoweverSlowItsFiles(t *testing.T) {
	f, c := newCountingFlusher(10*time.Millisecond, 0)
	f.Hold()
	for range 3 {
		f.Record()
		time.Sleep(30 * time.Millisecond)
	}
	if c.saves.Load() != 0 {
		t.Fatalf("saved %d times during the batch, want 0", c.saves.Load())
	}
	f.Release()
	if c.saves.Load() != 1 || c.rebuilds.Load() != 1 {
		t.Fatalf("saves=%d rebuilds=%d after the batch, want 1 and 1", c.saves.Load(), c.rebuilds.Load())
	}
	time.Sleep(30 * time.Millisecond)
	if c.saves.Load() != 1 {
		t.Fatalf("saved %d times, want 1", c.saves.Load())
	}
}

func TestRecentFlushWaitsForTheOuterBatch(t *testing.T) {
	f, c := newCountingFlusher(time.Millisecond, 0)
	f.Hold()
	f.Hold()
	f.Record()
	f.Release()
	time.Sleep(20 * time.Millisecond)
	if c.saves.Load() != 0 {
		t.Fatalf("saved with a batch still running")
	}
	f.Release()
	if c.saves.Load() != 1 {
		t.Fatalf("saves=%d after the last batch ended, want 1", c.saves.Load())
	}
}

func TestRecentFlushCloseWaitsForASaveInProgress(t *testing.T) {
	f, c := newCountingFlusher(time.Millisecond, 50*time.Millisecond)
	f.Record()
	waitFor(t, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.saving
	})
	f.Close()
	if c.saves.Load() != 1 {
		t.Fatalf("Close returned with saves=%d, want the save in progress finished", c.saves.Load())
	}
	time.Sleep(10 * time.Millisecond)
	if c.rebuilds.Load() != 0 {
		t.Fatalf("menu rebuilt %d times after Close, want 0", c.rebuilds.Load())
	}
}

func TestRecentFlushCloseSavesAWaitingChangeAndStopsTheTimer(t *testing.T) {
	f, c := newCountingFlusher(20*time.Millisecond, 0)
	f.Record()
	f.Close()
	if c.saves.Load() != 1 {
		t.Fatalf("saves=%d after Close, want 1", c.saves.Load())
	}
	time.Sleep(50 * time.Millisecond)
	if c.saves.Load() != 1 || c.rebuilds.Load() != 0 {
		t.Fatalf("saves=%d rebuilds=%d after the delay, want 1 and 0", c.saves.Load(), c.rebuilds.Load())
	}
}

func TestRecentFlushSavesAnOpenRecordedDuringASave(t *testing.T) {
	f, c := newCountingFlusher(time.Millisecond, 30*time.Millisecond)
	f.Record()
	waitFor(t, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.saving
	})
	f.Record()
	waitFor(t, func() bool { return c.saves.Load() == 2 })
}
