package no_silent_truncation_test

// Story 14.3 #5 -- multi-stream /Contents shows only the first stream (AC3, AC4).
//
// RED PHASE: multi-content-stream.pdf has one page whose /Contents is an array
// of two stream refs. Stream 1 is `q ... cm` (opens a graphics state, NO
// matching Q); stream 2 is `BT /F1 24 Tf 0 0 Td (Hello) Tj ET Q` (the matching
// Q plus a text block). Per ISO 32000-1 7.8.2 the page's content is the
// CONCATENATION of both. Today `dump stream --page 1` decodes ONLY stream 1 and
// emits no marker, presenting an unbalanced partial program as if it were the
// whole content stream.
//
// The GREEN target is path-dependent (Task 0's return-type decision), so each
// assertion accepts EITHER outcome and fails only the silent stream-1-only
// state that is wrong under both:
//   - preferred: the output covers BOTH streams (a stream-2 operator such as Q
//     or Tj is present); OR
//   - floor: a machine-visible truncation marker (streamCount / truncated) is
//     present so no consumer mistakes the partial for the whole.
// Today neither holds -> RED.

import (
	"strings"
	"testing"
)

// stream2Operators are operators that appear ONLY in the second content stream;
// their presence proves the concatenation (preferred) path covered stream 2.
var stream2Operators = []string{"Q", "BT", "Tf", "Td", "Tj", "ET"}

// ---------------------------------------------------------------------------
// 14.3-INTG-002 [P0] fixture sanity: the multi-stream fixture parses through
// the existing open path (dump objects, exit 0). Passes TODAY.
// ---------------------------------------------------------------------------

func TestMultiStream_FixtureParsesThroughOpenPath(t *testing.T) {
	bin := buildCLI(t)
	_, stderr, ec := runCLI(t, bin, "dump", "objects", fixturePath(t, "multi-content-stream.pdf"))
	if ec != 0 {
		t.Fatalf("[P0] 14.3-INTG-002: multi-content-stream.pdf rejected by the open path (exit %d): %s", ec, stderr)
	}
}

// ---------------------------------------------------------------------------
// 14.3-INTG-002 [P1] AC3/AC4: `dump stream --page 1 --json` must NOT present a
// silent stream-1-only view. GREEN is either (preferred) operators from BOTH
// streams, or (floor) a truncation marker with the array length. RED today:
// only stream 1's operators (q, cm), no marker.
// ---------------------------------------------------------------------------

func TestMultiStream_JSONNotSilentStreamOne(t *testing.T) {
	bin := buildCLI(t)
	f := fixturePath(t, "multi-content-stream.pdf")

	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--page", "1", "--json", f)
	if ec != 0 {
		t.Fatalf("[P1] 14.3-INTG-002: `dump stream --page 1 --json` must exit 0, got %d\nstderr: %s", ec, stderr)
	}
	res := parseObject(t, "14.3-INTG-002", stdout)

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
		t.Errorf("[P1] 14.3-INTG-002: --json presents a silent stream-1-only view (operators %v, no stream-2 op, no streamCount/truncated marker); a multi-stream page must either concatenate all streams or carry a truncation marker (AC3/AC4)", ops)
	}

	// When the floor path is taken, the marker must report the real array length.
	if hasMarker && !coversStream2 {
		if sc, ok := res["streamCount"]; !ok || jsonInt(sc) != 2 {
			t.Errorf("[P1] 14.3-INTG-002: floor-path marker must report streamCount == 2 (the /Contents array length), got %v", res["streamCount"])
		}
	}
}

// ---------------------------------------------------------------------------
// 14.3-INTG-002 [P1] AC4: `dump stream --page 1 --ops` must NOT silently emit
// only stream 1's operators. GREEN is either (preferred) NDJSON operator
// records from BOTH streams, or (floor) a DISTINCT trailing meta record
// carrying the truncation state (streamCount, no "op" key) that rides the
// NDJSON without breaching Story 14-1's one-object-per-operator contract. RED
// today: only q + cm records, no stream-2 op, no meta record.
// ---------------------------------------------------------------------------

func TestMultiStream_OpsNotSilentStreamOne(t *testing.T) {
	bin := buildCLI(t)
	f := fixturePath(t, "multi-content-stream.pdf")

	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--page", "1", "--ops", f)
	if ec != 0 {
		t.Fatalf("[P1] 14.3-INTG-002: `dump stream --page 1 --ops` must exit 0, got %d\nstderr: %s", ec, stderr)
	}

	coversStream2 := false
	hasMetaMarker := false
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rec := parseObject(t, "14.3-INTG-002", line)
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
				t.Errorf("[P1] 14.3-INTG-002 (--ops): meta marker streamCount = %v, want 2", rec["streamCount"])
			}
		}
	}

	if !coversStream2 && !hasMetaMarker {
		t.Errorf("[P1] 14.3-INTG-002: --ops silently emits only stream 1's operators; a multi-stream page must emit all streams' operators or a distinct trailing truncation meta record (AC4):\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 14.3-INTG-002 [P1] AC3/AC4 (--raw surface): `dump stream --page 1 --raw` on a
// multi-stream page must dump the CONCATENATION of all streams' decoded bytes
// (ISO 32000-1 7.8.2), not just stream 1. GREEN: stdout carries content from
// BOTH stream 1 (`cm`) and stream 2 (`Hello`), exit 0, and no truncation note
// (nothing was dropped). Guards that --raw is not a silent partial.
// ---------------------------------------------------------------------------

func TestMultiStream_RawConcatenatesAllStreams(t *testing.T) {
	bin := buildCLI(t)
	f := fixturePath(t, "multi-content-stream.pdf")

	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--page", "1", "--raw", f)
	if ec != 0 {
		t.Fatalf("[P1] 14.3-INTG-002 (--raw): must exit 0, got %d\nstderr: %s", ec, stderr)
	}

	// stdout must carry BOTH streams' decoded bytes: stream 1's `cm` and stream
	// 2's `Hello`/`Q`, joined per 7.8.2.
	if !strings.Contains(stdout, "cm") {
		t.Errorf("[P1] 14.3-INTG-002 (--raw): stdout missing stream 1 content (expected `cm`): %q", stdout)
	}
	if !strings.Contains(stdout, "Hello") || !strings.Contains(stdout, "Q") {
		t.Errorf("[P1] 14.3-INTG-002 (--raw): stdout missing stream 2 content; --raw must dump the concatenation of all streams (expected `Hello` and `Q`): %q", stdout)
	}

	// Nothing was truncated, so no truncation note should appear on either channel.
	if strings.Contains(stdout, "truncated") || strings.Contains(stderr, "truncated") {
		t.Errorf("[P1] 14.3-INTG-002 (--raw): unexpected truncation note after full concatenation\nstdout: %q\nstderr: %q", stdout, stderr)
	}
}
