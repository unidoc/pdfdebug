package no_silent_truncation_test

// Multi-stream /Contents must not show only the first stream.
//
// multi-content-stream.pdf has one page whose /Contents is an array of two
// stream refs. Stream 1 is `q ... cm` (opens a graphics state, NO matching Q);
// stream 2 is `BT /F1 24 Tf 0 0 Td (Hello) Tj ET Q` (the matching Q plus a text
// block). Per ISO 32000-1 7.8.2 the page's content is the CONCATENATION of
// both, so decoding only stream 1 and emitting no marker would present an
// unbalanced partial program as if it were the whole content stream.
//
// The GREEN target is path-dependent (Task 0's return-type decision), so each
// assertion accepts EITHER outcome and fails only the silent stream-1-only
// state that is wrong under both:
//   - preferred: the output covers BOTH streams (a stream-2 operator such as Q
//     or Tj is present); OR
//   - floor: a machine-visible truncation marker (streamCount / truncated) is
//     present so no consumer mistakes the partial for the whole.

import (
	"strings"
	"testing"
)

// stream2Operators are operators that appear ONLY in the second content stream;
// their presence proves the concatenation (preferred) path covered stream 2.
var stream2Operators = []string{"Q", "BT", "Tf", "Td", "Tj", "ET"}

// ---------------------------------------------------------------------------
// Fixture sanity: the multi-stream fixture parses through the existing open
// path (dump objects, exit 0). Passes TODAY.
// ---------------------------------------------------------------------------

func TestMultiStream_FixtureParsesThroughOpenPath(t *testing.T) {
	bin := buildCLI(t)
	_, stderr, ec := runCLI(t, bin, "dump", "objects", fixturePath(t, "multi-content-stream.pdf"))
	if ec != 0 {
		t.Fatalf("multi-content-stream.pdf rejected by the open path (exit %d): %s", ec, stderr)
	}
}

// ---------------------------------------------------------------------------
// `dump stream --page 1 --json` must NOT present a silent stream-1-only view.
// GREEN is either (preferred) operators from BOTH streams, or (floor) a
// truncation marker with the array length.
// ---------------------------------------------------------------------------

func TestMultiStream_JSONNotSilentStreamOne(t *testing.T) {
	bin := buildCLI(t)
	f := fixturePath(t, "multi-content-stream.pdf")

	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--page", "1", "--json", f)
	if ec != 0 {
		t.Fatalf("`dump stream --page 1 --json` must exit 0, got %d\nstderr: %s", ec, stderr)
	}
	res := parseObject(t, stdout)

	ops := formattedOperators(res)

	// Preferred path: the concatenated content carries a stream-2-only operator.
	coversStream2 := false
	for _, op := range stream2Operators {
		if contains(ops, op) {
			coversStream2 = true
			break
		}
	}

	// Floor path: a machine-visible marker names the array length / truncation.
	_, hasStreamCount := res["streamCount"]
	truncated, _ := res["truncated"].(bool)
	hasMarker := hasStreamCount || truncated

	if !coversStream2 && !hasMarker {
		t.Errorf("--json presents a silent stream-1-only view (operators %v, no stream-2 op, no streamCount/truncated marker); a multi-stream page must either concatenate all streams or carry a truncation marker", ops)
	}

	// When the floor path is taken, the marker must report the real array length.
	if hasMarker && !coversStream2 {
		if sc, ok := res["streamCount"]; !ok || jsonInt(sc) != 2 {
			t.Errorf("floor-path marker must report streamCount == 2 (the /Contents array length), got %v", res["streamCount"])
		}
	}
}

// ---------------------------------------------------------------------------
// `dump stream --page 1 --ops` must NOT silently emit only stream 1's
// operators. GREEN is either (preferred) NDJSON operator records from BOTH
// streams, or (floor) a DISTINCT trailing meta record carrying the truncation
// state (streamCount, no "op" key) that rides the NDJSON without breaching
// the one-object-per-operator contract.
// ---------------------------------------------------------------------------

func TestMultiStream_OpsNotSilentStreamOne(t *testing.T) {
	bin := buildCLI(t)
	f := fixturePath(t, "multi-content-stream.pdf")

	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--page", "1", "--ops", f)
	if ec != 0 {
		t.Fatalf("`dump stream --page 1 --ops` must exit 0, got %d\nstderr: %s", ec, stderr)
	}

	coversStream2 := false
	hasMetaMarker := false
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rec := parseObject(t, line)
		if op, ok := rec["op"].(string); ok && op != "" {
			for _, s2 := range stream2Operators {
				if op == s2 {
					coversStream2 = true
				}
			}
			continue
		}
		// A record with no "op" key is the floor meta marker; it must carry the
		// truncation state and NOT masquerade as an operator (14-1 contract).
		if _, ok := rec["streamCount"]; ok {
			hasMetaMarker = true
			if jsonInt(rec["streamCount"]) != 2 {
				t.Errorf("(--ops): meta marker streamCount = %v, want 2", rec["streamCount"])
			}
		}
	}

	if !coversStream2 && !hasMetaMarker {
		t.Errorf("--ops silently emits only stream 1's operators; a multi-stream page must emit all streams' operators or a distinct trailing truncation meta record:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// --raw surface: `dump stream --page 1 --raw` on a multi-stream page must dump
// the CONCATENATION of all streams' decoded bytes (ISO 32000-1 7.8.2), not just
// stream 1. GREEN: stdout carries content from BOTH stream 1 (`cm`) and stream
// 2 (`Hello`), exit 0, and no truncation note (nothing was dropped). Guards
// that --raw is not a silent partial.
// ---------------------------------------------------------------------------

func TestMultiStream_RawConcatenatesAllStreams(t *testing.T) {
	bin := buildCLI(t)
	f := fixturePath(t, "multi-content-stream.pdf")

	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--page", "1", "--raw", f)
	if ec != 0 {
		t.Fatalf("(--raw): must exit 0, got %d\nstderr: %s", ec, stderr)
	}

	// stdout must carry BOTH streams' decoded bytes: stream 1's `cm` and stream
	// 2's `Hello`/`Q`, joined per 7.8.2.
	if !strings.Contains(stdout, "cm") {
		t.Errorf("(--raw): stdout missing stream 1 content (expected `cm`): %q", stdout)
	}
	if !strings.Contains(stdout, "Hello") || !strings.Contains(stdout, "Q") {
		t.Errorf("(--raw): stdout missing stream 2 content; --raw must dump the concatenation of all streams (expected `Hello` and `Q`): %q", stdout)
	}

	// Nothing was truncated, so no truncation note should appear on either channel.
	if strings.Contains(stdout, "truncated") || strings.Contains(stderr, "truncated") {
		t.Errorf("(--raw): unexpected truncation note after full concatenation\nstdout: %q\nstderr: %q", stdout, stderr)
	}
}
