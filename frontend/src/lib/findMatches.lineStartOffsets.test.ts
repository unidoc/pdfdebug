/**
 * Story 10.7: Frontend Hook and Render-Path Correctness (finding #8) --
 * findMatches accepts a pre-built lineStartOffsets table.
 * (finding #20) -- findMatches accepts a pre-built haystack string.
 *
 * TDD RED PHASE: every test below is emitted as `test()`. They assert the
 * POST-FIX signature `findMatches(content, query, caseSensitive,
 * lineStartOffsets?, haystack?)`. The current signature
 * (findMatches.ts:86) takes only three params and unconditionally calls
 * buildLineStartOffsets(content) at line 93 and content.toLowerCase() at line
 * 91, so these will fail / not exercise the skip path until Task 4 lands.
 *
 * These tests import buildLineStartOffsets to wrap it with a spy; that name is
 * already exported. A developer activates them by removing `.skip` after the
 * params are added.
 *
 * Test IDs follow the convention.
 *
 * Run: cd frontend && npx vitest run src/lib/findMatches.lineStartOffsets.test.ts
 */
import { describe, test, expect, vi, afterEach } from 'vitest';
import * as findMatchesModule from './findMatches';
import { findMatches } from './findMatches';

afterEach(() => {
  vi.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// When a caller-supplied lineStartOffsets is provided, the internal
// buildLineStartOffsets(content) call is SKIPPED.
// ---------------------------------------------------------------------------

describe('findMatches skips internal buildLineStartOffsets when offsets supplied', () => {
  test('buildLineStartOffsets is NOT called when an offset table is supplied', () => {
    const corpus = 'foo\nbar\nfoo';
    const offsets = findMatchesModule.buildLineStartOffsets(corpus);
    // Spy AFTER building so the legitimate setup call is not counted.
    const spy = vi.spyOn(findMatchesModule, 'buildLineStartOffsets');
    // Post-fix 4th positional arg: lineStartOffsets.
    findMatches(corpus, 'foo', false, offsets);
    expect(spy).not.toHaveBeenCalled();
  });

  test('buildLineStartOffsets IS called when the offset arg is omitted (backward compatible)', () => {
    const corpus = 'foo\nbar\nfoo';
    const spy = vi.spyOn(findMatchesModule, 'buildLineStartOffsets');
    findMatches(corpus, 'foo', false);
    expect(spy).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// The supplied offset table drives the reported `line` field. A
// VALID-but-distinct table that partitions the content into different line
// boundaries than the natural split must change the reported lines.
// ---------------------------------------------------------------------------

describe('supplied offset table determines reported line numbers', () => {
  test('line reflects the supplied partition, not the natural split', () => {
    // No newlines in the corpus, so the natural table is [0] (everything on
    // line 1). Supply a valid-but-distinct table that splits at offset 6 so
    // the second "foo" (start 6) is reported on line 2.
    const corpus = 'foofoofoo'; // matches "foo" at offsets 0, 3, 6
    const naturalLine = findMatches(corpus, 'foo', false).map((m) => m.line);
    expect(naturalLine).toEqual([1, 1, 1]);

    // Valid table: monotonic non-decreasing, in-range. Lines: 1 starts at 0,
    // line 2 starts at offset 6.
    const customOffsets = [0, 6];
    const customLine = findMatches(corpus, 'foo', false, customOffsets).map((m) => m.line);
    // Offsets 0 and 3 fall on line 1; offset 6 falls on line 2.
    expect(customLine).toEqual([1, 1, 2]);
  });

  test('match start/end are unaffected by the supplied offset table', () => {
    const corpus = 'foofoofoo';
    const customOffsets = [0, 6];
    const matches = findMatches(corpus, 'foo', false, customOffsets);
    expect(matches.map((m) => m.start)).toEqual([0, 3, 6]);
    expect(matches.map((m) => m.end)).toEqual([3, 6, 9]);
  });
});

// ---------------------------------------------------------------------------
// findMatches accepts a pre-built haystack and uses it instead of computing
// content.toLowerCase() internally. The haystack length must equal content
// length (Latin-1 length-preserving invariant).
// ---------------------------------------------------------------------------

describe('findMatches uses a supplied haystack', () => {
  test('case-insensitive search uses the supplied lowercased haystack', () => {
    const content = 'FooBARfoo';
    const haystack = content.toLowerCase(); // 'foobarfoo'
    expect(haystack.length).toBe(content.length); // invariant
    // 5th positional arg: haystack. 4th (offsets) omitted -> undefined.
    const matches = findMatches(content, 'foo', false, undefined, haystack);
    expect(matches.map((m) => m.start)).toEqual([0, 6]);
    // start/end index into the haystack, which equals content length, so the
    // offsets are valid against content too.
    expect(matches.map((m) => m.end)).toEqual([3, 9]);
  });

  test('case-sensitive search with haystack === content matches exactly', () => {
    const content = 'FooBARFoo';
    const matches = findMatches(content, 'Foo', true, undefined, content);
    expect(matches.map((m) => m.start)).toEqual([0, 6]);
  });

  // Gap (automate): the load-bearing invariant is that toLowerCase is
  // length-preserving for the FULL Latin-1 range (U+00C0..U+00FF), not just
  // ASCII. UNIT-003 only uses ASCII. Supply a haystack lowercased from accented
  // uppercase (E-acute U+00C9 -> e-acute U+00E9) and assert offsets still index
  // identically into content and haystack.
  test('accented Latin-1 haystack stays length-aligned with content so offsets are valid', () => {
    const content = 'CAFÉ X café Y CAFÉ'; // 'CAFÉ X café Y CAFÉ'
    const haystack = content.toLowerCase();
    // Invariant the implementation relies on (Decision section: load-bearing).
    expect(haystack.length).toBe(content.length);

    const matches = findMatches(content, 'café', false, undefined, haystack);
    // Three occurrences of 'café' (case-insensitive): offsets 0, 7, 14.
    expect(matches.map((m) => m.start)).toEqual([0, 7, 14]);
    // start/end index into content correctly because lengths are equal.
    matches.forEach((m) => {
      expect(content.slice(m.start, m.end).toLowerCase()).toBe('café');
    });
  });
});
