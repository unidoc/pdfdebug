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

// newBlockingFlusher returns a flusher whose first save signals started and
// then waits for release, so a test acts while that save is in progress.
func newBlockingFlusher(delay time.Duration) (f *recentFlusher, c *flushCounts, started, release chan struct{}) {
	c = &flushCounts{}
	started, release = make(chan struct{}), make(chan struct{})
	save := func() {
		if c.saves.Load() == 0 {
			close(started)
			<-release
		}
		c.saves.Add(1)
	}
	rebuild := func() { c.rebuilds.Add(1) }
	return newRecentFlusher(delay, time.Hour, save, rebuild, func(fn func()) { fn() }), c, started, release
}

// returnsWhileBlocked reports whether fn returns within 50ms.
func returnsWhileBlocked(fn func()) (done chan struct{}, returned bool) {
	done = make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
		return done, true
	case <-time.After(50 * time.Millisecond):
		return done, false
	}
}

func waitDone(t *testing.T, done chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("did not return within 2s")
	}
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
	f, c := newCountingFlusher(100*time.Millisecond, time.Hour, 0)
	for range 3 {
		ended := f.OpenStarted()
		time.Sleep(150 * time.Millisecond)
		ended()
		f.Record()
	}
	if c.saves.Load() != 0 {
		t.Fatalf("saved %d times during the run, want 0", c.saves.Load())
	}
	waitFor(t, func() bool { return c.saves.Load() == 1 })
	time.Sleep(150 * time.Millisecond)
	if c.saves.Load() != 1 {
		t.Fatalf("saves=%d, want 1", c.saves.Load())
	}
}

func TestRecentFlushARunLongerThanMaxHoldStillSavesOnce(t *testing.T) {
	f, c := newCountingFlusher(100*time.Millisecond, 300*time.Millisecond, 0)
	for range 6 {
		ended := f.OpenStarted()
		time.Sleep(150 * time.Millisecond)
		ended()
		f.Record()
	}
	if c.saves.Load() != 0 {
		t.Fatalf("saved %d times during a 900ms run of 150ms opens, want 0", c.saves.Load())
	}
	waitFor(t, func() bool { return c.saves.Load() == 1 })
}

func TestRecentFlushWaitsForEveryOpenInFlight(t *testing.T) {
	f, c := newCountingFlusher(time.Millisecond, time.Hour, 0)
	endA := f.OpenStarted()
	endB := f.OpenStarted()
	f.Record()
	endA()
	time.Sleep(20 * time.Millisecond)
	if c.saves.Load() != 0 {
		t.Fatal("saved with an open still in flight")
	}
	endB()
	waitFor(t, func() bool { return c.saves.Load() == 1 })
}

// startStuckOpen starts an open whose start lies past maxHold, as for an open
// that never returned.
func startStuckOpen(f *recentFlusher) (ended func()) {
	ended = f.OpenStarted()
	f.mu.Lock()
	f.opens[f.nextOpen-1] = time.Now().Add(-2 * f.maxHold)
	f.mu.Unlock()
	return ended
}

func TestRecentFlushAnOpenInFlightHoldsTheSaveUntilMaxHold(t *testing.T) {
	f, c := newCountingFlusher(time.Millisecond, 300*time.Millisecond, 0)
	f.OpenStarted()
	f.Record()
	time.Sleep(50 * time.Millisecond)
	if c.saves.Load() != 0 {
		t.Fatal("saved with an open in flight before maxHold")
	}
	waitFor(t, func() bool { return c.saves.Load() == 1 })
}

func TestRecentFlushAStuckOpenNoLongerHoldsTheSave(t *testing.T) {
	f, c := newCountingFlusher(time.Millisecond, time.Hour, 0)
	startStuckOpen(f)
	f.Record()
	waitFor(t, func() bool { return c.saves.Load() == 1 })
}

func TestRecentFlushAStuckOpenDoesNotReleaseTheHoldOfAFreshOne(t *testing.T) {
	f, c := newCountingFlusher(time.Millisecond, time.Hour, 0)
	startStuckOpen(f)
	ended := f.OpenStarted()
	f.Record()
	time.Sleep(50 * time.Millisecond)
	if c.saves.Load() != 0 {
		t.Fatal("saved while a fresh open was in flight")
	}
	ended()
	waitFor(t, func() bool { return c.saves.Load() == 1 })
}

func TestRecentFlushAnEndThatHeldNothingDoesNotDelayThePendingSave(t *testing.T) {
	f, c := newCountingFlusher(300*time.Millisecond, time.Hour, 0)
	endStuck := startStuckOpen(f)
	f.Record()
	time.Sleep(150 * time.Millisecond)
	endStuck()
	// Saved at 300ms; restarting the delay at this end would save at 450ms.
	time.Sleep(250 * time.Millisecond)
	if c.saves.Load() != 1 {
		t.Fatalf("saves=%d 400ms after the record, want 1", c.saves.Load())
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

func TestRecentFlushSaveNowIgnoresAnOpenInFlight(t *testing.T) {
	f, c := newCountingFlusher(time.Hour, time.Hour, 0)
	f.OpenStarted()
	f.SaveNow()
	if c.saves.Load() != 1 || c.rebuilds.Load() != 1 {
		t.Fatalf("saves=%d rebuilds=%d, want 1 and 1", c.saves.Load(), c.rebuilds.Load())
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

func TestRecentFlushCloseWaitsForASaveInProgress(t *testing.T) {
	f, c, started, release := newBlockingFlusher(time.Millisecond)
	f.Record()
	<-started
	done, returned := returnsWhileBlocked(f.Close)
	if returned {
		t.Fatal("Close returned with a save in progress")
	}
	close(release)
	waitDone(t, done)
	if c.saves.Load() != 1 {
		t.Fatalf("saves=%d after Close, want 1", c.saves.Load())
	}
	time.Sleep(10 * time.Millisecond)
	if c.rebuilds.Load() != 0 {
		t.Fatalf("menu rebuilt %d times after Close, want 0", c.rebuilds.Load())
	}
}

func TestRecentFlushSavesAnOpenRecordedDuringASave(t *testing.T) {
	f, c, started, release := newBlockingFlusher(time.Millisecond)
	f.Record()
	<-started
	f.Record()
	close(release)
	waitFor(t, func() bool { return c.saves.Load() == 2 })
}

func TestRecentFlushSaveNowReturnsOnlyAfterASaveThatStartedAfterIt(t *testing.T) {
	f, c, started, release := newBlockingFlusher(time.Millisecond)
	f.Record()
	<-started
	done, returned := returnsWhileBlocked(f.SaveNow)
	if returned {
		t.Fatal("SaveNow returned while the save that began before it was in progress")
	}
	close(release)
	waitDone(t, done)
	if c.saves.Load() != 2 {
		t.Fatalf("saves=%d when SaveNow returned, want 2", c.saves.Load())
	}
}
