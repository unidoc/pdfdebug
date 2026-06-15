package pendingopen

import (
	"strconv"
	"sync"
	"testing"
)

func TestColdAddQueuesReturnsFalse(t *testing.T) {
	var q Queue
	if got := q.Add("/a.pdf"); got != false {
		t.Fatalf("cold Add must return false (queued), got %v", got)
	}
	paths := q.Drain()
	if len(paths) != 1 || paths[0] != "/a.pdf" {
		t.Fatalf("Drain must deliver the queued path, got %#v", paths)
	}
}

func TestWarmAddReturnsTrueNoQueue(t *testing.T) {
	var q Queue
	q.Drain() // flip ready
	if got := q.Add("/warm.pdf"); got != true {
		t.Fatalf("warm Add must return true, got %v", got)
	}
	if paths := q.Drain(); len(paths) != 0 {
		t.Fatalf("ready-path Add must NOT queue, got %#v", paths)
	}
}

func TestDrainInsertionOrder(t *testing.T) {
	var q Queue
	q.Add("/1.pdf")
	q.Add("/2.pdf")
	q.Add("/3.pdf")
	want := []string{"/1.pdf", "/2.pdf", "/3.pdf"}
	got := q.Drain()
	if len(got) != len(want) {
		t.Fatalf("want %d paths, got %#v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order broken at %d: want %q got %q", i, want[i], got[i])
		}
	}
}

func TestSecondDrainEmptyStaysReady(t *testing.T) {
	var q Queue
	q.Add("/a.pdf")
	if first := q.Drain(); len(first) != 1 {
		t.Fatalf("first Drain should deliver 1, got %#v", first)
	}
	if second := q.Drain(); len(second) != 0 {
		t.Fatalf("second Drain must be empty, got %#v", second)
	}
	if got := q.Add("/after.pdf"); got != true {
		t.Fatalf("queue must stay ready, Add got %v", got)
	}
}

func TestQueuedDedupExactRawString(t *testing.T) {
	var q Queue
	q.Add("/dup.pdf")
	q.Add("/dup.pdf") // exact repeat -> recorded once
	q.Add("/Dup.pdf") // case variant -> distinct raw string
	want := []string{"/dup.pdf", "/Dup.pdf"}
	got := q.Drain()
	if len(got) != len(want) {
		t.Fatalf("want %#v, got %#v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("want %#v got %#v", want, got)
		}
	}
}

func TestReadyPathNeverDedups(t *testing.T) {
	var q Queue
	q.Drain() // ready
	if got := q.Add("/same.pdf"); got != true {
		t.Fatalf("ready Add #1 must return true, got %v", got)
	}
	if got := q.Add("/same.pdf"); got != true {
		t.Fatalf("ready Add #2 (repeat) must also return true, got %v", got)
	}
	if paths := q.Drain(); len(paths) != 0 {
		t.Fatalf("ready Adds must not queue, got %#v", paths)
	}
}

func TestExactlyOnceUnderRace(t *testing.T) {
	const n = 200
	var q Queue
	inputs := make([]string, n)
	for i := 0; i < n; i++ {
		inputs[i] = "/race/" + strconv.Itoa(i) + ".pdf"
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	openedImmediately := map[string]int{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			if q.Add(p) {
				mu.Lock()
				openedImmediately[p]++
				mu.Unlock()
			}
		}(inputs[i])
	}

	wg.Add(1)
	drained := map[string]int{}
	go func() {
		defer wg.Done()
		for _, p := range q.Drain() {
			drained[p]++
		}
	}()
	wg.Wait()

	for _, p := range q.Drain() {
		drained[p]++
	}

	for p, c := range drained {
		if c != 1 {
			t.Fatalf("path %q drained %d times (want 1)", p, c)
		}
	}
	for _, p := range inputs {
		if total := drained[p] + openedImmediately[p]; total != 1 {
			t.Fatalf("path %q delivered %d times (want 1)", p, total)
		}
	}
}
