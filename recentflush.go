package main

import (
	"sync"
	"time"
)

// recentFlusher decides when Open Recent is saved and its menu rebuilt. A
// recorded open marks the list dirty. Outside a batch the save runs delay after
// the last open, so opens arriving one by one in quick succession cost one
// save; inside a batch, Go's or the frontend's, it waits for the batch to end
// however long each file takes. SaveNow saves at once, for Clear Menu and a
// removed entry. Close stops further rebuilds and saves a change still
// waiting, after any save in progress.
type recentFlusher struct {
	delay   time.Duration
	save    func()
	rebuild func()
	// onMain runs the rebuild on the UI thread. Close runs there too, so a
	// rebuild that gets there after Close sees closed and does nothing.
	onMain func(func())

	mu sync.Mutex
	// saved is signalled when a save in progress ends.
	saved        *sync.Cond
	timer        *time.Timer
	held         int
	frontendHeld int
	dirty        bool
	saving       bool
	closed       bool
}

func newRecentFlusher(delay time.Duration, save, rebuild func(), onMain func(func())) *recentFlusher {
	f := &recentFlusher{delay: delay, save: save, rebuild: rebuild, onMain: onMain}
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
	if f.held > 0 || f.frontendHeld > 0 || f.closed {
		return
	}
	if f.timer == nil {
		f.timer = time.AfterFunc(f.delay, func() { f.flush(false) })
	} else {
		f.timer.Reset(f.delay)
	}
}

// Hold starts a Go batch; nothing is saved until every Hold has its Release.
func (f *recentFlusher) Hold() {
	f.mu.Lock()
	f.held++
	f.mu.Unlock()
}

// Release ends a Go batch and saves at once when no batch is left.
func (f *recentFlusher) Release() {
	f.mu.Lock()
	f.held--
	f.mu.Unlock()
	f.flush(false)
}

// HoldFrontend starts a frontend batch.
func (f *recentFlusher) HoldFrontend() {
	f.mu.Lock()
	f.frontendHeld++
	f.mu.Unlock()
}

// ReleaseFrontend ends a frontend batch and saves at once when no batch is
// left. A release with no batch open does nothing.
func (f *recentFlusher) ReleaseFrontend() {
	f.mu.Lock()
	if f.frontendHeld > 0 {
		f.frontendHeld--
	}
	f.mu.Unlock()
	f.flush(false)
}

// ResetFrontend drops the frontend's batches, for a page that reloaded before
// releasing them.
func (f *recentFlusher) ResetFrontend() {
	f.mu.Lock()
	f.frontendHeld = 0
	f.mu.Unlock()
	f.flush(false)
}

// SaveNow saves the list and rebuilds the menu at once, batch or not.
func (f *recentFlusher) SaveNow() {
	f.mu.Lock()
	f.dirty = true
	for f.saving {
		f.saved.Wait()
	}
	f.mu.Unlock()
	f.flush(true)
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

func (f *recentFlusher) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// flush saves a dirty list and rebuilds the menu; outside force, not while a
// batch is open.
func (f *recentFlusher) flush(force bool) {
	f.mu.Lock()
	if f.closed || !f.dirty || f.saving || (!force && (f.held > 0 || f.frontendHeld > 0)) {
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
	f.mu.Unlock()
	f.onMain(func() {
		if !f.isClosed() {
			f.rebuild()
		}
	})
}
