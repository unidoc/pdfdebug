package main

import (
	"sync"
	"time"
)

// recentFlusher decides when Open Recent is saved and its menu rebuilt. A
// recorded open marks the list dirty, and the save runs delay after the last
// record with no open in flight, so a batch, or opens in quick succession,
// costs one save however long each open takes. An open stops holding the save
// once it has run for maxHold, so one that never returns delays the save by
// maxHold once and not for the rest of the session.
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
	saved *sync.Cond
	timer *time.Timer
	// opens holds the start time of each open in flight, by id.
	opens    map[int]time.Time
	nextOpen int
	dirty    bool
	saving   bool
	// started and finished count saves, so SaveNow can wait for one that
	// began after its change.
	started, finished int
	closed            bool
}

func newRecentFlusher(delay, maxHold time.Duration, save, rebuild func(), onMain func(func())) *recentFlusher {
	f := &recentFlusher{delay: delay, maxHold: maxHold, save: save, rebuild: rebuild, onMain: onMain, opens: map[int]time.Time{}}
	f.saved = sync.NewCond(&f.mu)
	return f
}

// Record marks the list dirty and restarts the delay.
func (f *recentFlusher) Record() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dirty = true
	f.arm()
}

// OpenStarted counts an open in flight, which holds the save for up to
// maxHold, and returns the call that ends it.
func (f *recentFlusher) OpenStarted() (ended func()) {
	f.mu.Lock()
	id := f.nextOpen
	f.nextOpen++
	f.opens[id] = time.Now()
	f.mu.Unlock()
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		delete(f.opens, id)
		if f.dirty {
			f.arm()
		}
	}
}

// heldUntil reports whether an open in flight still holds the save, and until
// when: maxHold after the newest open started. f.mu is held.
func (f *recentFlusher) heldUntil() (time.Time, bool) {
	var newest time.Time
	for _, start := range f.opens {
		if start.After(newest) {
			newest = start
		}
	}
	until := newest.Add(f.maxHold)
	return until, len(f.opens) > 0 && time.Now().Before(until)
}

// arm restarts the timer: delay from now, or when the hold ends if an open in
// flight still holds the save. f.mu is held.
func (f *recentFlusher) arm() {
	if f.closed {
		return
	}
	d := f.delay
	if until, held := f.heldUntil(); held {
		d = time.Until(until)
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
	f.dirty = true
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
// an open in flight holds the save.
func (f *recentFlusher) flush(force bool) {
	f.mu.Lock()
	_, held := f.heldUntil()
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
