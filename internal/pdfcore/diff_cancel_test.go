package pdfcore

import (
	"context"
	"errors"
	"testing"
	"time"
)

// countDiffChecks runs one uncancelled diff and returns how many cancellation
// checks the walk made.
func countDiffChecks(t *testing.T, ins *Inspector, l, r string) int {
	t.Helper()
	n := 0
	diffVisitHook = func() { n++ }
	defer func() { diffVisitHook = nil }()
	if _, err := ins.DiffDocuments(l, r); err != nil {
		t.Fatalf("uncancelled DiffDocuments: %v", err)
	}
	return n
}

// Closing the right document from another goroutine while the walk is paused
// mid-way makes DiffDocuments return a context.Canceled error, stop visiting
// nodes, and release the left document's pdfMu.
func TestDiff_CloseMidWalkCancels(t *testing.T) {
	pdf := diffTwoPage()
	ins, l, r := openTwoForDiff(t, "a.pdf", pdf, "b.pdf", pdf)

	total := countDiffChecks(t, ins, l, r)
	if total < 6 {
		t.Fatalf("fixture walk makes only %d checks, too few to close mid-walk", total)
	}
	closeAt := total / 2

	paused := make(chan struct{})
	resume := make(chan struct{})
	checks := 0
	diffVisitHook = func() {
		checks++
		if checks == closeAt {
			close(paused)
			<-resume
		}
	}
	defer func() { diffVisitHook = nil }()

	type outcome struct {
		res *DiffResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := ins.DiffDocuments(l, r)
		done <- outcome{res, err}
	}()

	select {
	case <-paused:
	case <-time.After(5 * time.Second):
		t.Fatal("walk never reached the close point")
	}
	if err := ins.Close(r); err != nil {
		t.Fatalf("close right: %v", err)
	}
	close(resume)

	var got outcome
	select {
	case got = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("DiffDocuments did not return after the right document closed")
	}
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("DiffDocuments error = %v, want one satisfying errors.Is(err, context.Canceled)", got.err)
	}
	if got.res != nil {
		t.Errorf("DiffDocuments returned a result alongside the cancel error")
	}
	if checks >= total {
		t.Errorf("walk made %d checks after a close at %d; an uncancelled walk makes %d", checks, closeAt, total)
	}

	freed := make(chan error, 1)
	go func() {
		_, err := ins.GetChildren(l, "root")
		freed <- err
	}()
	select {
	case err := <-freed:
		if err != nil {
			t.Errorf("GetChildren on the left document after cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("left document's pdfMu still held after the cancelled diff returned")
	}
}

// A document closed before the walk takes its locks is never walked.
func TestDiff_ClosedBeforeWalkCancels(t *testing.T) {
	pdf := diffOnePage()
	ins, l, r := openTwoForDiff(t, "a.pdf", pdf, "b.pdf", pdf)
	doc, err := ins.GetDocument(l)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	doc.closeCancel()

	if _, err := ins.DiffDocuments(l, r); !errors.Is(err, context.Canceled) {
		t.Fatalf("DiffDocuments error = %v, want one satisfying errors.Is(err, context.Canceled)", err)
	}
}
