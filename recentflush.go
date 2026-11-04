package main

import (
	"sync"
	"time"
)

// recentFlusher decides when Open Recent is saved and its menu rebuilt. A
// recorded open marks the list dirty, and the save runs delay after the last
// record with no open in flight, so a run of opens, from one batch or from
// files handed over one at a time, costs one save however long each takes. A
// save held that way still runs maxHold after the list first went dirty, so an
// open that never returns does not hold it for the rest of the session.
// SaveNow saves at once, for Clear Menu and a removed entry. Close stops
// further rebuilds and saves a change still waiting, after any save in
// progress.
type recentFlusher struct {
	delay   time.Duration
	maxHold time.Duration
	save    func()
	rebuild func()
	// onMain runs the rebuild on the UI thread. Close runs there too, so a
	// rebuild that gets there after Close sees closed and does nothing.
	onMain func(func())

	mu sync.Mutex
	// saved is signalled when a save in progress ends.
	saved      *sync.Cond
	timer      *time.Timer
	inFlight   int
	dirty      bool
	dirtySince time.Time
	saving     bool
	// started and finished count saves, so SaveNow can wait for one that
	// began after its change.
	started, finished int
	closed            bool
}

func newRecentFlusher(delay, maxHold time.Duration, save, rebuild func(), onMain func(func())) *recentFlusher {
	f := &recentFlusher{delay: delay, maxHold: maxHold, save: save, rebuild: rebuild, onMain: onMain}
	f.saved = sync.NewCond(&f.mu)
	return f
}

// Record marks the list dirty and restarts the delay.
func (f *recentFlusher) Record() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markDirty()
	f.arm()
}

// markDirty sets dirty, noting when the list went dirty. f.mu is held.
func (f *recentFlusher) markDirty() {
	if !f.dirty {
		f.dirty = true
		f.dirtySince = time.Now()
	}
}

// OpenStarted counts an open in flight; the save waits for it.
func (f *recentFlusher) OpenStarted() {
	f.mu.Lock()
	f.inFlight++
	f.mu.Unlock()
}

// OpenEnded ends an open in flight. With none left, a dirty list is saved
// delay later.
func (f *recentFlusher) OpenEnded() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inFlight--
	if f.dirty {
		f.arm()
	}
}

// arm restarts the timer: delay from now with no open in flight, otherwise at
// maxHold after the list went dirty. f.mu is held.
func (f *recentFlusher) arm() {
	if f.closed {
		return
	}
	d := f.delay
	if f.inFlight > 0 {
		d = max(time.Until(f.dirtySince.Add(f.maxHold)), 0)
	}
	if f.timer == nil {
		f.timer = time.AfterFunc(d, func() { f.flush(false) })
	} else {
		f.timer.Reset(d)
	}
}

// SaveNow saves the list, opens in flight or not, and returns once a save that
// began after the call has finished; the menu rebuild follows that save.
func (f *recentFlusher) SaveNow() {
	f.mu.Lock()
	f.markDirty()
	want := f.started + 1
	f.mu.Unlock()
	for {
		f.flush(true)
		f.mu.Lock()
		for f.saving && f.finished < want {
			f.saved.Wait()
		}
		done := f.finished >= want || f.closed
		f.mu.Unlock()
		if done {
			return
		}
	}
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

// flush saves a dirty list and rebuilds the menu. Outside force it waits while
// an open is in flight, up to maxHold after the list went dirty.
func (f *recentFlusher) flush(force bool) {
	f.mu.Lock()
	held := f.inFlight > 0 && time.Since(f.dirtySince) < f.maxHold
	if f.closed || !f.dirty || f.saving || (!force && held) {
		// A save held by an open in flight still runs at maxHold.
		if held && f.dirty && !f.saving && !f.closed {
			f.arm()
		}
		f.mu.Unlock()
		return
	}
	f.dirty = false
	f.saving = true
	f.started++
	f.mu.Unlock()

	f.save()

	f.mu.Lock()
	f.saving = false
	f.finished++
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
