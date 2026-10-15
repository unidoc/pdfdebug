// Watcher: stat-based change detection for every open document, driven by an
// injected clock, source and emitter.
//
// Contract the tests rely on:
//
//	New(src Source, emit Emitter, clk Clock) *Watcher
//	Source:  TabIDs() []string; DiskStatus(tabID string) (pdfcore.DiskStatus, error)
//	Emitter: Emit(Event)   Event carries TabID, Seq, State, LoadedModTime,
//	                       DiskModTime, DiskSize (flat or through an embedded
//	                       pdfcore.DiskStatus) and marshals to
//	                       {tabId, seq, state, loadedModTime, diskModTime, diskSize}
//	Clock:   After(d time.Duration) <-chan time.Time; the scheduler asks for
//	         the next tick with the current interval
//	(*Watcher) Start(), Stop(), SetFocused(bool), RecordReported(tabID, pdfcore.DiskStatus)
//
// One "check" fires a tick and waits until the source has been asked for the
// tab once more. A tick that lands while the previous result for the tab is
// still unapplied starts no stat (in-flight rule), so check fires again until
// a new call starts.
//
// Run: cd code && go test -race ./internal/filewatch/

package filewatch

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"unidoc-pdf-debugger/internal/pdfcore"
)

const waitLimit = 3 * time.Second

// --- fake clock ---

type fakeClock struct {
	mu    sync.Mutex
	reqs  []time.Duration
	chans []chan time.Time
	fired int // index of the next unfired channel
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time)
	c.reqs = append(c.reqs, d)
	c.chans = append(c.chans, ch)
	return ch
}

// lastInterval returns the most recently requested interval, or 0.
func (c *fakeClock) lastInterval() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.reqs) == 0 {
		return 0
	}
	return c.reqs[len(c.reqs)-1]
}

// fire delivers one tick on the latest armed channel and returns once the
// scheduler received it.
func (c *fakeClock) fire(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(waitLimit)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		var ch chan time.Time
		if n := len(c.chans); n > 0 && c.fired < n {
			ch = c.chans[n-1]
			c.fired = n
		}
		c.mu.Unlock()
		if ch != nil {
			select {
			case ch <- time.Now():
				return
			case <-time.After(waitLimit):
				t.Fatalf("scheduler did not take the tick")
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("scheduler never armed the clock")
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitLimit)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// --- fake source ---

type answer struct {
	st  pdfcore.DiskStatus
	err error
}

type fakeSource struct {
	mu       sync.Mutex
	tabs     []string
	answers  map[string]answer
	calls    map[string]int
	gates    map[string]chan struct{}
	inflight map[string]int
	maxIn    map[string]int
}

func newFakeSource(tabs ...string) *fakeSource {
	s := &fakeSource{
		tabs:     tabs,
		answers:  map[string]answer{},
		calls:    map[string]int{},
		gates:    map[string]chan struct{}{},
		inflight: map[string]int{},
		maxIn:    map[string]int{},
	}
	for _, id := range tabs {
		s.answers[id] = answer{st: current()}
	}
	return s
}

func (s *fakeSource) TabIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.tabs...)
}

func (s *fakeSource) DiskStatus(tabID string) (pdfcore.DiskStatus, error) {
	s.mu.Lock()
	s.calls[tabID]++
	s.inflight[tabID]++
	if s.inflight[tabID] > s.maxIn[tabID] {
		s.maxIn[tabID] = s.inflight[tabID]
	}
	gate := s.gates[tabID]
	// The answer is taken when the call starts, so changing it after check
	// returned never leaks into a call that already began.
	a := s.answers[tabID]
	s.mu.Unlock()
	if gate != nil {
		<-gate
	}
	s.mu.Lock()
	s.inflight[tabID]--
	s.mu.Unlock()
	return a.st, a.err
}

func (s *fakeSource) set(tabID string, st pdfcore.DiskStatus, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answers[tabID] = answer{st: st, err: err}
}

func (s *fakeSource) setTabs(tabs ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tabs = tabs
}

// block makes the next DiskStatus calls for tabID wait until release.
func (s *fakeSource) block(tabID string) (release func()) {
	ch := make(chan struct{})
	s.mu.Lock()
	s.gates[tabID] = ch
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.gates, tabID)
			s.mu.Unlock()
			close(ch)
		})
	}
}

func (s *fakeSource) callCount(tabID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[tabID]
}

func (s *fakeSource) maxInflight(tabID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxIn[tabID]
}

// --- fake emitter ---

type emitted struct {
	ev          Event
	callsAtEmit int
}

type fakeEmitter struct {
	mu     sync.Mutex
	src    *fakeSource
	events []emitted
}

func (e *fakeEmitter) Emit(ev Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, emitted{ev: ev, callsAtEmit: e.src.callCount(ev.TabID)})
}

func (e *fakeEmitter) all() []emitted {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]emitted(nil), e.events...)
}

func (e *fakeEmitter) forTab(tabID string) []emitted {
	var out []emitted
	for _, x := range e.all() {
		if x.ev.TabID == tabID {
			out = append(out, x)
		}
	}
	return out
}

// --- statuses ---

const loadedMs = int64(1_700_000_000_000)

func current() pdfcore.DiskStatus {
	return pdfcore.DiskStatus{State: "current", LoadedModTime: loadedMs, DiskModTime: loadedMs, DiskSize: 100}
}

func changed(size, mtimeMs int64) pdfcore.DiskStatus {
	return pdfcore.DiskStatus{State: "changed", LoadedModTime: loadedMs, DiskModTime: mtimeMs, DiskSize: size}
}

func missing() pdfcore.DiskStatus {
	return pdfcore.DiskStatus{State: "missing", LoadedModTime: loadedMs}
}

// --- harness ---

type harness struct {
	t   *testing.T
	w   *Watcher
	clk *fakeClock
	src *fakeSource
	em  *fakeEmitter
}

func start(t *testing.T, tabs ...string) *harness {
	t.Helper()
	src := newFakeSource(tabs...)
	h := &harness{t: t, clk: &fakeClock{}, src: src, em: &fakeEmitter{src: src}}
	h.w = New(src, h.em, h.clk)
	h.w.Start()
	t.Cleanup(h.w.Stop)
	return h
}

// check makes one more stat of tabID happen and returns the new call count.
func (h *harness) check(tabID string) int {
	h.t.Helper()
	n := h.src.callCount(tabID)
	deadline := time.Now().Add(waitLimit)
	for time.Now().Before(deadline) {
		h.clk.fire(h.t)
		inner := time.Now().Add(20 * time.Millisecond)
		for time.Now().Before(inner) {
			if c := h.src.callCount(tabID); c > n {
				return c
			}
			time.Sleep(200 * time.Microsecond)
		}
	}
	h.t.Fatalf("no new stat of %s after ticks", tabID)
	return 0
}

// warm makes sure tabID has been observed at its baseline at least once, so
// later call counts start from a known point.
func (h *harness) warm(tabID string) int {
	h.t.Helper()
	h.check(tabID)
	return h.check(tabID)
}

func (h *harness) waitEvents(tabID string, n int) []emitted {
	h.t.Helper()
	waitFor(h.t, "events", func() bool { return len(h.em.forTab(tabID)) >= n })
	return h.em.forTab(tabID)
}

// --- settle ---

func TestWatcherSettleReportsChangeAfterTwoIdenticalChecks(t *testing.T) {
	h := start(t, "a")
	base := h.warm("a")

	h.src.set("a", changed(200, loadedMs+5000), nil)
	h.check("a")
	h.check("a")
	evs := h.waitEvents("a", 1)
	if evs[0].ev.State != "changed" {
		t.Fatalf("first event state %q, want changed", evs[0].ev.State)
	}
	if evs[0].callsAtEmit != base+2 {
		t.Errorf("changed emitted after check %d, want after the second identical check (%d)", evs[0].callsAtEmit-base, 2)
	}
	if evs[0].ev.DiskSize != 200 || evs[0].ev.DiskModTime != loadedMs+5000 || evs[0].ev.LoadedModTime != loadedMs {
		t.Errorf("event payload %+v does not carry the observed status", evs[0].ev)
	}

	// Further identical checks emit nothing; going back to the Open
	// fingerprint emits current after two checks.
	h.check("a")
	h.check("a")
	h.src.set("a", current(), nil)
	h.check("a")
	back := h.check("a")
	evs = h.waitEvents("a", 2)
	if len(evs) != 2 {
		t.Fatalf("events %+v, want exactly changed then current", evs)
	}
	if evs[1].ev.State != "current" || evs[1].callsAtEmit != back {
		t.Errorf("second event %+v at call %d, want current after call %d", evs[1].ev, evs[1].callsAtEmit, back)
	}
	if evs[1].ev.Seq <= evs[0].ev.Seq {
		t.Errorf("seq must increase across emits: %d then %d", evs[0].ev.Seq, evs[1].ev.Seq)
	}
}

func TestWatcherSettleIgnoresFileStillBeingWritten(t *testing.T) {
	h := start(t, "a")
	base := h.warm("a")
	for size := int64(101); size <= 105; size++ {
		h.src.set("a", changed(size, loadedMs+size), nil)
		h.check("a")
	}
	h.check("a") // second identical observation of size 105
	evs := h.waitEvents("a", 1)
	if len(evs) != 1 || evs[0].ev.DiskSize != 105 {
		t.Fatalf("events %+v, want one changed event for the final size 105", evs)
	}
	if evs[0].callsAtEmit != base+6 {
		t.Errorf("emitted after check %d, want 6 (nothing while the observation kept changing)", evs[0].callsAtEmit-base)
	}
}

func TestWatcherSettleReportsMissingAfterTwoChecks(t *testing.T) {
	h := start(t, "a")
	base := h.warm("a")
	h.src.set("a", missing(), nil)
	h.check("a")
	h.check("a")
	evs := h.waitEvents("a", 1)
	if evs[0].ev.State != "missing" || evs[0].callsAtEmit != base+2 {
		t.Errorf("event %+v at check %d, want missing after check 2", evs[0].ev, evs[0].callsAtEmit-base)
	}
	if evs[0].ev.DiskSize != 0 || evs[0].ev.DiskModTime != 0 {
		t.Errorf("missing carries disk size and time 0, got %+v", evs[0].ev)
	}
}

func TestWatcherStatErrorResetsSettleStreak(t *testing.T) {
	h := start(t, "a")
	base := h.warm("a")
	h.src.set("a", changed(200, loadedMs+1), nil)
	h.check("a")
	h.src.set("a", pdfcore.DiskStatus{}, errors.New("permission denied"))
	h.check("a")
	h.src.set("a", changed(200, loadedMs+1), nil)
	h.check("a")
	h.check("a")
	evs := h.waitEvents("a", 1)
	if len(evs) != 1 || evs[0].callsAtEmit != base+4 {
		t.Errorf("events %+v, want one changed event after check 4 (the error resets the streak)", evs)
	}
}

func TestWatcherRecordedStatusLetsReturnToCurrentEmit(t *testing.T) {
	h := start(t, "a")
	h.warm("a")
	h.w.RecordReported("a", changed(200, loadedMs+1))
	h.check("a")
	h.check("a")
	evs := h.waitEvents("a", 1)
	if len(evs) != 1 || evs[0].ev.State != "current" {
		t.Errorf("events %+v, want one current event after a recorded change went back", evs)
	}
}

// A status recorded from GetDiskStatus restarts the settle streak: an older
// observation still counting toward a change cannot complete against it.
func TestWatcherRecordedStatusRestartsSettleStreak(t *testing.T) {
	h := start(t, "a")
	h.warm("a")
	h.src.set("a", changed(200, loadedMs+1), nil)
	h.check("a")
	h.w.RecordReported("a", current())
	h.check("a")
	h.src.set("a", current(), nil)
	h.check("a")
	h.check("a")
	if evs := h.em.forTab("a"); len(evs) != 0 {
		t.Errorf("events %+v, want none: the change never settled after the recorded current", evs)
	}
}

func TestWatcherEventJSONShape(t *testing.T) {
	h := start(t, "a")
	h.warm("a")
	h.src.set("a", changed(200, loadedMs+1), nil)
	h.check("a")
	h.check("a")
	evs := h.waitEvents("a", 1)
	raw, err := json.Marshal(evs[0].ev)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tabId", "seq", "state", "loadedModTime", "diskModTime", "diskSize"} {
		if _, ok := m[key]; !ok {
			t.Errorf("event JSON %s lacks %q", raw, key)
		}
	}
	if len(m) != 6 {
		t.Errorf("event JSON %s must be the flat six-field payload", raw)
	}
}

// --- cadence and focus ---

func TestWatcherCadenceFollowsFocus(t *testing.T) {
	h := start(t, "a", "b")
	waitFor(t, "first tick armed", func() bool { return h.clk.lastInterval() != 0 })
	if got := h.clk.lastInterval(); got != 2*time.Second {
		t.Errorf("starts focused: interval %v, want 2s", got)
	}

	h.w.SetFocused(false)
	waitFor(t, "unfocused interval", func() bool { return h.clk.lastInterval() == 10*time.Second })

	a, b := h.src.callCount("a"), h.src.callCount("b")
	h.w.SetFocused(true)
	waitFor(t, "focused interval", func() bool { return h.clk.lastInterval() == 2*time.Second })
	waitFor(t, "immediate check of every tab on focus", func() bool {
		return h.src.callCount("a") > a && h.src.callCount("b") > b
	})
}

func TestWatcherFocusTakesCallerValueRegardlessOfOrder(t *testing.T) {
	h := start(t, "a")
	waitFor(t, "first tick armed", func() bool { return h.clk.lastInterval() != 0 })

	// A lost-focus handler that runs after a focus handler passes the window's
	// current IsFocused value; the last call wins.
	h.w.SetFocused(true)
	h.w.SetFocused(false)
	waitFor(t, "unfocused after the later call", func() bool { return h.clk.lastInterval() == 10*time.Second })

	h.w.SetFocused(false)
	h.w.SetFocused(true)
	waitFor(t, "focused after the later call", func() bool { return h.clk.lastInterval() == 2*time.Second })
}

func TestWatcherChecksBackgroundTabsAndNeverEmitsForUnchangedOnes(t *testing.T) {
	h := start(t, "active", "background")
	h.warm("background")
	h.src.set("background", changed(300, loadedMs+9), nil)
	h.check("background")
	h.check("background")
	evs := h.waitEvents("background", 1)
	if evs[0].ev.State != "changed" {
		t.Errorf("background tab event %+v, want changed", evs[0].ev)
	}
	if h.src.callCount("active") == 0 {
		t.Errorf("every open tab is checked")
	}
	if got := h.em.forTab("active"); len(got) != 0 {
		t.Errorf("a tab that never left current emits nothing, got %+v", got)
	}
}

func TestWatcherNewTabStartsAtBaselineWithoutEvent(t *testing.T) {
	h := start(t, "a")
	h.warm("a")
	h.src.setTabs("a", "late")
	h.src.set("late", current(), nil)
	h.check("late")
	h.check("late")
	h.src.set("a", changed(5, loadedMs+5), nil)
	h.check("a")
	h.check("a")
	h.waitEvents("a", 1)
	if got := h.em.forTab("late"); len(got) != 0 {
		t.Errorf("a newly opened tab emits nothing at its baseline, got %+v", got)
	}
}

// --- isolation ---

func TestWatcherBlockedTabDoesNotDelayOthers(t *testing.T) {
	h := start(t, "slow", "fast")
	h.warm("slow")
	h.warm("fast")
	release := h.src.block("slow")
	t.Cleanup(release)
	h.check("slow") // this call hangs

	h.src.set("fast", changed(7, loadedMs+7), nil)
	h.check("fast")
	h.check("fast")
	h.check("fast")
	evs := h.waitEvents("fast", 1)
	if evs[0].ev.State != "changed" {
		t.Errorf("fast tab event %+v, want changed while the slow tab is blocked", evs[0].ev)
	}
	slowCalls := h.src.callCount("slow")
	h.check("fast")
	if h.src.callCount("slow") != slowCalls {
		t.Errorf("a new stat started for a tab whose stat is still in flight")
	}
	if got := h.src.maxInflight("slow"); got != 1 {
		t.Errorf("max concurrent stats for the blocked tab = %d, want 1", got)
	}
}

func TestWatcherDiscardsLateResultForClosedTab(t *testing.T) {
	h := start(t, "gone", "keep")
	h.warm("gone")
	h.warm("keep")
	// One changed observation for "gone"; a second would settle it.
	h.src.set("gone", changed(9, loadedMs+9), nil)
	h.check("gone")
	release := h.src.block("gone")
	h.check("gone") // second observation is in flight

	h.src.setTabs("keep")
	h.check("keep") // reconcile removes "gone"
	release()

	h.src.set("keep", changed(4, loadedMs+4), nil)
	h.check("keep")
	h.check("keep")
	h.check("keep")
	h.waitEvents("keep", 1)
	if got := h.em.forTab("gone"); len(got) != 0 {
		t.Errorf("a result for a closed tab must be discarded, got %+v", got)
	}
}

func TestWatcherStopReturnsWhileStatIsBlocked(t *testing.T) {
	src := newFakeSource("a")
	clk := &fakeClock{}
	w := New(src, &fakeEmitter{src: src}, clk)
	w.Start()
	release := src.block("a")
	defer release()

	h := &harness{t: t, w: w, clk: clk, src: src}
	h.check("a") // blocked stat

	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(waitLimit):
		t.Fatalf("Stop must not wait for a blocked stat")
	}
}
