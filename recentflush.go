package main

import (
	"sync"
	"time"
)

// recentFlusher decides when Open Recent is saved and its menu rebuilt. A
// recorded open marks the list dirty. Outside a batch the save runs delay after
// the last open, so opens arriving one by one in quick succession cost one
// save; inside a batch it waits for the batch to end however long each file
// takes. Close stops further rebuilds and saves a change still waiting, after
// any save in progress.
type recentFlusher struct {
	delay   time.Duration
	save    func()
	rebuild func()

	mu     sync.Mutex
	timer  *time.Timer
	held   int
	dirty  bool
	saving bool
	closed bool
	// saved is signalled when a save in progress ends.
	saved *sync.Cond
}

func newRecentFlusher(delay time.Duration) *recentFlusher {
	f := &recentFlusher{delay: delay}
	f.saved = sync.NewCond(&f.mu)
	return f
}

// Record marks the list dirty and, outside a batch, restarts the delay.
func (f *recentFlusher) Record() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dirty = true
	f.arm()
}

// arm restarts the delay unless a batch or Close holds the save. f.mu is held.
func (f *recentFlusher) arm() {
	if f.held > 0 || f.closed {
		return
	}
	if f.timer == nil {
		f.timer = time.AfterFunc(f.delay, f.flush)
	} else {
		f.timer.Reset(f.delay)
	}
}

// Hold starts a batch; nothing is saved until every Hold has its Release.
func (f *recentFlusher) Hold() {
	f.mu.Lock()
	f.held++
	if f.timer != nil {
		f.timer.Stop()
	}
	f.mu.Unlock()
}

// Release ends a batch and saves at once when it was the last one.
func (f *recentFlusher) Release() {
	f.mu.Lock()
	f.held--
	f.mu.Unlock()
	f.flush()
}

// Close stops the timer and later rebuilds, waits for a save in progress, and
// saves a change still waiting.
func (f *recentFlusher) Close() {
	f.mu.Lock()
	f.closed = true
	if f.timer != nil {
		f.timer.Stop()
	}
	for f.saving {
		f.saved.Wait()
	}
	dirty := f.dirty
	f.dirty = false
	f.mu.Unlock()
	if dirty {
		f.save()
	}
}

func (f *recentFlusher) flush() {
	f.mu.Lock()
	if f.closed || f.held > 0 || !f.dirty || f.saving {
		f.mu.Unlock()
		return
	}
	f.dirty = false
	f.saving = true
	f.mu.Unlock()

	f.save()

	f.mu.Lock()
	f.saving = false
	f.saved.Broadcast()
	// An open recorded during the save gets its own.
	if f.dirty {
		f.arm()
	}
	closed := f.closed
	f.mu.Unlock()
	if !closed {
		f.rebuild()
	}
}
