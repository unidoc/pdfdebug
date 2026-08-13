// Story 12.1: the shared per-path routing decision in main.go.
//
// Both backend entry points (ApplicationOpenedWithFile, OnSecondInstanceLaunch)
// route every path through the queue first and only call openFileAndEmit +
// window.Focus() when Add returns true (ready/warm path). The shared decision
// lives in an in-package helper, e.g.
//
//	routeOpenPath(q *pendingopen.Queue, path string, open func(string)) bool
//
// Returning the ready verdict (caller decides Focus). Task 2.4 this helper is
// exercised by a main-package table test (the only sanctioned automated pin on
// the main.go wiring -- source-grep tests are forbidden by the strict guard,
// precedent: TestExtractPDFPaths). The behavioural assertion: before Drain the
// path is queued and the fake open func is NOT called; after Drain the open
// func IS called.
//
// This acceptance test DELEGATES to that main-package test rather than
// duplicating the wiring assertion, because the helper takes a func argument
// and lives in `package main` (not importable from any test module). The
// delegation runs `go test -run RouteOpenPath .` at the module root.
package story_12_1_test

import (
	"strings"
	"testing"
)

// TestRouteOpenPathDecision asserts a main-package test named with `RouteOpenPath`
// (matched by `-run RouteOpenPath`) exists and passes, pinning the
// queued-vs-ready verdict of the routing helper with a fake open func. When no
// such test exists, `go test -run RouteOpenPath .` reports "no tests to run",
// which this treats as a failure.
func TestRouteOpenPathDecision(t *testing.T) {
	out, err := runMainPackageTest(t, "RouteOpenPath")

	// A passing delegation requires BOTH: the command succeeded AND it
	// actually ran at least one matching test. `go test -run` exits 0 when
	// the regexp matches nothing, emitting the "no tests to run" warning, so
	// we must reject that case explicitly or a missing test would pass.
	if strings.Contains(out, "no tests to run") {
		t.Fatalf("no main-package test matching `RouteOpenPath` exists (expected a routeOpenPath helper plus a table test with a fake open func). Output:\n%s", out)
	}
	if err != nil {
		t.Fatalf("main-package RouteOpenPath test failed:\n%s", out)
	}
}
