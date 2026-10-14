// Package filewatch reports when the file behind an open document changes on
// disk. It polls stat-based status for every open tab from one scheduler
// goroutine and emits an event once a new observation has settled.
package filewatch

import (
	"errors"
	"sync"
	"time"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// Poll intervals while the main window is focused and unfocused.
const (
	focusedInterval   = 2 * time.Second
	unfocusedInterval = 10 * time.Second
)

// Source lists open tabs and reports each tab's disk status. A tab whose
// DiskStatus returns pdfcore.ErrDocumentNotFound is dropped.
type Source interface {
	TabIDs() []string
	DiskStatus(tabID string) (pdfcore.DiskStatus, error)
}

// Emitter delivers settled status transitions.
type Emitter interface {
	Emit(Event)
}

// Clock arms the next tick.
type Clock interface {
	After(d time.Duration) <-chan time.Time
}

// SystemClock is the Clock backed by time.After.
type SystemClock struct{}

// After returns time.After(d).
func (SystemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Event is one settled disk-status transition for a tab. Seq increases with
// every emit so receivers can drop events that arrive out of order.
type Event struct {
	TabID         string `json:"tabId"`
	Seq           int64  `json:"seq"`
	State         string `json:"state"`
	LoadedModTime int64  `json:"loadedModTime"`
	DiskModTime   int64  `json:"diskModTime"`
	DiskSize      int64  `json:"diskSize"`
}

// tabState is the scheduler-owned settle state of one tab.
type tabState struct {
	lastReported pdfcore.DiskStatus
	candidate    pdfcore.DiskStatus
	streak       int
	inFlight     bool
}

type statResult struct {
	tabID string
	st    pdfcore.DiskStatus
	err   error
}

// Watcher polls every open tab's disk status. All settle state is owned by
// the scheduler goroutine; SetFocused and RecordReported only queue input for
// it and never block.
type Watcher struct {
	src  Source
	emit Emitter
	clk  Clock

	results chan statResult
	wake    chan struct{}
	stop    chan struct{}
	done    chan struct{}

	mu         sync.Mutex
	focused    bool
	focusDirty bool
	records    map[string]pdfcore.DiskStatus
	started    bool
	stopOnce   sync.Once
}

// New returns a Watcher that starts in focused mode. Call Start to run it.
func New(src Source, emit Emitter, clk Clock) *Watcher {
	return &Watcher{
		src:     src,
		emit:    emit,
		clk:     clk,
		results: make(chan statResult),
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		focused: true,
		records: map[string]pdfcore.DiskStatus{},
	}
}

// Start launches the scheduler goroutine. Calling it more than once has no
// further effect.
func (w *Watcher) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return
	}
	w.started = true
	go w.run()
}

// Stop ends the scheduler and waits for it to exit. It never waits for stat
// calls still in flight: a stat on a dead network share cannot be cancelled.
func (w *Watcher) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
	w.mu.Lock()
	started := w.started
	w.mu.Unlock()
	if started {
		<-w.done
	}
}

// SetFocused switches between the focused and unfocused interval. The last
// call wins, so callers pass the window's current focus state rather than the
// event that fired. Entering focused mode checks every tab immediately.
func (w *Watcher) SetFocused(focused bool) {
	w.mu.Lock()
	w.focused = focused
	w.focusDirty = true
	w.mu.Unlock()
	w.signal()
}

// RecordReported sets the status last reported for tabID, so a later return
// to the Open fingerprint emits current even when the watcher never reported
// the change itself.
func (w *Watcher) RecordReported(tabID string, st pdfcore.DiskStatus) {
	w.mu.Lock()
	w.records[tabID] = st
	w.mu.Unlock()
	w.signal()
}

func (w *Watcher) signal() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func intervalFor(focused bool) time.Duration {
	if focused {
		return focusedInterval
	}
	return unfocusedInterval
}

func (w *Watcher) run() {
	defer close(w.done)
	tabs := map[string]*tabState{}
	var seq int64
	focused := true
	tick := w.clk.After(intervalFor(focused))

	// applyPending takes queued focus changes and recorded statuses before any
	// tick or result is handled, so they apply in call order.
	applyPending := func() {
		w.mu.Lock()
		records := w.records
		w.records = map[string]pdfcore.DiskStatus{}
		newFocus, dirty := w.focused, w.focusDirty
		w.focusDirty = false
		w.mu.Unlock()

		for id, st := range records {
			ts := tabs[id]
			if ts == nil {
				ts = &tabState{}
				tabs[id] = ts
			}
			ts.lastReported = st
		}
		if dirty && newFocus != focused {
			entering := newFocus
			focused = newFocus
			tick = w.clk.After(intervalFor(focused))
			if entering {
				w.checkAll(tabs)
			}
		}
	}

	for {
		select {
		case <-w.stop:
			return
		case <-w.wake:
			applyPending()
		case <-tick:
			applyPending()
			w.checkAll(tabs)
			tick = w.clk.After(intervalFor(focused))
		case r := <-w.results:
			applyPending()
			ts := tabs[r.tabID]
			if ts == nil {
				continue
			}
			ts.inFlight = false
			if errors.Is(r.err, pdfcore.ErrDocumentNotFound) {
				delete(tabs, r.tabID)
				continue
			}
			if r.err != nil {
				ts.streak = 0
				ts.candidate = pdfcore.DiskStatus{}
				continue
			}
			if ev, ok := settle(ts, r.st); ok {
				seq++
				ev.TabID = r.tabID
				ev.Seq = seq
				w.emit.Emit(ev)
			}
		}
	}
}

// checkAll reconciles tabs against the source and starts one stat for every
// tab that has none in flight. New tabs start at their Open fingerprint.
func (w *Watcher) checkAll(tabs map[string]*tabState) {
	ids := w.src.TabIDs()
	live := make(map[string]bool, len(ids))
	for _, id := range ids {
		live[id] = true
		if tabs[id] == nil {
			tabs[id] = &tabState{lastReported: pdfcore.DiskStatus{State: pdfcore.DiskStateCurrent}}
		}
	}
	for id := range tabs {
		if !live[id] {
			delete(tabs, id)
		}
	}
	for id, ts := range tabs {
		if ts.inFlight {
			continue
		}
		ts.inFlight = true
		go w.stat(id)
	}
}

// stat runs one source call and hands the result to the scheduler, or drops
// it once the scheduler has stopped.
func (w *Watcher) stat(tabID string) {
	st, err := w.src.DiskStatus(tabID)
	select {
	case w.results <- statResult{tabID: tabID, st: st, err: err}:
	case <-w.stop:
	}
}

// settle records one observation and returns the event to emit when two
// consecutive identical observations differ from the last reported status.
// A tab that stays current never emits.
func settle(ts *tabState, obs pdfcore.DiskStatus) (Event, bool) {
	if obs == ts.candidate && ts.streak > 0 {
		ts.streak++
	} else {
		ts.candidate = obs
		ts.streak = 1
	}
	if ts.streak < 2 {
		return Event{}, false
	}
	last := ts.lastReported
	if obs.State == pdfcore.DiskStateCurrent && last.State == pdfcore.DiskStateCurrent {
		return Event{}, false
	}
	if obs == last {
		return Event{}, false
	}
	ts.lastReported = obs
	return Event{
		State:         obs.State,
		LoadedModTime: obs.LoadedModTime,
		DiskModTime:   obs.DiskModTime,
		DiskSize:      obs.DiskSize,
	}, true
}
