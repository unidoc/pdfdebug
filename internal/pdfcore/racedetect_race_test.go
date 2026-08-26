//go:build race

package pdfcore

// raceEnabled reports whether the race detector is instrumenting this build.
// Allocation-magnitude assertions are meaningless under it: instrumentation
// forces heap escapes in the inner loops being measured, roughly doubling the
// figure, which pushes a legitimate result past any threshold tight enough to
// be worth asserting.
const raceEnabled = true
