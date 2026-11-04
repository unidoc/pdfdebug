package main

import (
	"sync/atomic"
	"testing"
	"time"
)

type flushCounts struct {
	saves, rebuilds atomic.Int32
}

func newCountingFlusher(delay, maxHold, saveTakes time.Duration) (*recentFlusher, *flushCounts) {
	c := &flushCounts{}
	save := func() {
		time.Sleep(saveTakes)
		c.saves.Add(1)
	}
	rebuild := func() { c.rebuilds.Add(1) }
	return newRecentFlusher(delay, maxHold, save, rebuild, func(fn func()) { fn() }), c
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

func waitSaving(t *testing.T, f *recentFlusher) {
	t.Helper()
	waitFor(t, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.saving
	})
}

func TestRecentFlushMergesQuickOpensIntoOneSave(t *testing.T) {
	f, c := newCountingFlusher(30*time.Millisecond, time.Hour, 0)
	for range 5 {
		f.Record()
	}
	waitFor(t, func() bool { return c.rebuilds.Load() == 1 })
	time.Sleep(60 * time.Millisecond)
	if c.saves.Load() != 1 || c.rebuilds.Load() != 1 {
		t.Fatalf("saves=%d rebuilds=%d, want 1 and 1", c.saves.Load(), c.rebuilds.Load())
	}
}

// Each open takes longer than the delay; the record lands after the open ends
// and before the next one starts, as on every open path.
func TestRecentFlushSavesSlowOpensInARowOnce(t *testing.T) {
	f, c := newCountingFlusher(10*time.Millisecond, time.Hour, 0)
	for range 3 {
		f.OpenStarted()
		time.Sleep(30 * time.Millisecond)
		f.OpenEnded()
		f.Record()
	}
	if c.saves.Load() != 0 {
		t.Fatalf("saved %d times during the run, want 0", c.saves.Load())
	}
	waitFor(t, func() bool { return c.saves.Load() == 1 })
	time.Sleep(30 * time.Millisecond)
	if c.saves.Load() != 1 {
		t.Fatalf("saves=%d, want 1", c.saves.Load())
	}
}

func TestRecentFlushWaitsForEveryOpenInFlight(t *testing.T) {
	f, c := newCountingFlusher(time.Millisecond, time.Hour, 0)
	f.OpenStarted()
	f.OpenStarted()
	f.Record()
	f.OpenEnded()
	time.Sleep(20 * time.Millisecond)
	if c.saves.Load() != 0 {
		t.Fatal("saved with an open still in flight")
	}
	f.OpenEnded()
	waitFor(t, func() bool { return c.saves.Load() == 1 })
}

func TestRecentFlushAnOpenThatNeverEndsHoldsTheSaveOnlyUntilMaxHold(t *testing.T) {
	f, c := newCountingFlusher(time.Millisecond, 40*time.Millisecond, 0)
	f.Record()
	f.OpenStarted()
	time.Sleep(10 * time.Millisecond)
	if c.saves.Load() != 0 {
		t.Fatal("saved with an open in flight before maxHold")
	}
	waitFor(t, func() bool { return c.saves.Load() == 1 })
	f.Record()
	waitFor(t, func() bool { return c.saves.Load() == 2 })
}

func TestRecentFlushCloseWaitsForASaveInProgress(t *testing.T) {
	f, c := newCountingFlusher(time.Millisecond, time.Hour, 50*time.Millisecond)
	f.Record()
	waitSaving(t, f)
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
	f, c := newCountingFlusher(20*time.Millisecond, time.Hour, 0)
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
	f, c := newCountingFlusher(time.Millisecond, time.Hour, 30*time.Millisecond)
	f.Record()
	waitSaving(t, f)
	f.Record()
	waitFor(t, func() bool { return c.saves.Load() == 2 })
}

func TestRecentFlushSaveNowIgnoresAnOpenInFlight(t *testing.T) {
	f, c := newCountingFlusher(time.Hour, time.Hour, 0)
	f.OpenStarted()
	f.SaveNow()
	if c.saves.Load() != 1 || c.rebuilds.Load() != 1 {
		t.Fatalf("saves=%d rebuilds=%d, want 1 and 1", c.saves.Load(), c.rebuilds.Load())
	}
}

func TestRecentFlushSaveNowReturnsOnlyAfterASaveThatStartedAfterIt(t *testing.T) {
	for range 20 {
		f, c := newCountingFlusher(time.Millisecond, time.Hour, 20*time.Millisecond)
		f.Record()
		waitSaving(t, f)
		f.SaveNow()
		if c.saves.Load() != 2 {
			t.Fatalf("saves=%d when SaveNow returned, want 2", c.saves.Load())
		}
	}
}

func TestRecentFlushRebuildReachingTheUIThreadAfterCloseDoesNothing(t *testing.T) {
	c := &flushCounts{}
	var queued func()
	f := newRecentFlusher(time.Hour, time.Hour, func() { c.saves.Add(1) }, func() { c.rebuilds.Add(1) }, func(fn func()) { queued = fn })
	f.SaveNow()
	f.Close()
	queued()
	if c.rebuilds.Load() != 0 {
		t.Fatalf("rebuilds=%d after Close, want 0", c.rebuilds.Load())
	}
}
