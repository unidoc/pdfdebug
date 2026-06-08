/**
 * Story 10.2: Find Bar in Plain Text View -- pure-function red-phase suite.
 *
 * TDD RED PHASE: every test below fails until frontend/src/lib/findMatches.ts
 * is implemented per Task 2.
 *
 * Scope:
 * - findMatches(content, query, caseSensitive): Match[] -- AC4 algorithm,
 *   AC12 non-Latin-1 detection, AC19 performance budget.
 * - buildLineStartOffsets(content): number[] -- AC4 memoizable line table.
 *
 * Test IDs follow the 10-2-UNIT-NNN convention.
 *
 * Run: cd frontend && npx vitest run src/lib/findMatches.test.ts
 */
import { describe, test, expect } from 'vitest';
// RED PHASE: this import fails until Task 2.1 lands.
import { findMatches, buildLineStartOffsets, type Match } from './findMatches';

// ---------------------------------------------------------------------------
// 10-2-UNIT-001 [P0] AC#4: empty query yields zero matches.
// ---------------------------------------------------------------------------

describe('10-2-UNIT-001: empty query', () => {
  test('empty query over non-empty corpus returns []', () => {
    expect(findMatches('hello world', '', false)).toEqual([]);
  });

  test('empty query over empty corpus returns []', () => {
    expect(findMatches('', '', false)).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// 10-2-UNIT-002 [P0] AC#4: single literal substring match.
// ---------------------------------------------------------------------------

describe('10-2-UNIT-002: single match', () => {
  test('finds one match in a single-line corpus', () => {
    const matches = findMatches('hello world', 'world', false);
    expect(matches).toHaveLength(1);
    expect(matches[0].start).toBe(6);
    expect(matches[0].end).toBe(11);
    expect(matches[0].line).toBe(1);
  });

  test('query longer than corpus returns []', () => {
    expect(findMatches('foo', 'longer than corpus', false)).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// 10-2-UNIT-003 [P0] AC#4: multiple matches across multiple lines.
// ---------------------------------------------------------------------------

describe('10-2-UNIT-003: multiple matches', () => {
  test('finds all matches across LF-separated lines with 1-based line numbers', () => {
    const corpus = 'foo bar\nbaz foo\nfoo qux';
    const matches = findMatches(corpus, 'foo', false);
    expect(matches).toHaveLength(3);
    expect(matches[0].start).toBe(0);
    expect(matches[0].line).toBe(1);
    expect(matches[1].start).toBe(12);
    expect(matches[1].line).toBe(2);
    expect(matches[2].start).toBe(16);
    expect(matches[2].line).toBe(3);
  });
});

// ---------------------------------------------------------------------------
// 10-2-UNIT-004 [P0] AC#4 (Decision: non-overlapping): "aaaa" searched for
// "aa" yields 2 matches at offsets [0, 2].
// ---------------------------------------------------------------------------

describe('10-2-UNIT-004: non-overlapping matches', () => {
  test('"aaaa" find "aa" returns 2 matches at offsets 0 and 2', () => {
    const matches = findMatches('aaaa', 'aa', false);
    expect(matches).toHaveLength(2);
    expect(matches[0].start).toBe(0);
    expect(matches[0].end).toBe(2);
    expect(matches[1].start).toBe(2);
    expect(matches[1].end).toBe(4);
  });

  test('"aaaaaa" find "aa" returns 3 matches at offsets 0, 2, 4', () => {
    const matches = findMatches('aaaaaa', 'aa', false);
    expect(matches.map((m) => m.start)).toEqual([0, 2, 4]);
  });
});

// ---------------------------------------------------------------------------
// 10-2-UNIT-005 [P0] AC#4: case-insensitive default.
// ---------------------------------------------------------------------------

describe('10-2-UNIT-005: case-insensitive default', () => {
  test('"HELVETICA" matches "helvetica" when caseSensitive=false', () => {
    const corpus = 'Font /HELVETICA bold';
    const matches = findMatches(corpus, 'helvetica', false);
    expect(matches).toHaveLength(1);
    expect(matches[0].start).toBe(6);
  });

  test('mixed-case query matches mixed-case corpus when caseSensitive=false', () => {
    const matches = findMatches('FooBarFOObarFoo', 'foo', false);
    expect(matches).toHaveLength(3);
  });
});

// ---------------------------------------------------------------------------
// 10-2-UNIT-006 [P0] AC#4: case-sensitive opt-in.
// ---------------------------------------------------------------------------

describe('10-2-UNIT-006: case-sensitive when caseSensitive=true', () => {
  test('"HELVETICA" does NOT match "helvetica" when caseSensitive=true', () => {
    const corpus = 'Font /HELVETICA bold';
    expect(findMatches(corpus, 'helvetica', true)).toHaveLength(0);
  });

  test('exact-case match still hits when caseSensitive=true', () => {
    const corpus = 'Font /HELVETICA bold';
    const matches = findMatches(corpus, 'HELVETICA', true);
    expect(matches).toHaveLength(1);
    expect(matches[0].start).toBe(6);
  });
});

// ---------------------------------------------------------------------------
// 10-2-UNIT-007 [P0] AC#12: query with codepoint > U+00FF returns [].
// ---------------------------------------------------------------------------

describe('10-2-UNIT-007: non-Latin-1 query rejected', () => {
  test('query containing U+2192 (right arrow) returns []', () => {
    expect(findMatches('hello world', 'hello→world', false)).toEqual([]);
  });

  test('query containing an emoji (surrogate pair) returns []', () => {
    expect(findMatches('hello world', 'hello😀', false)).toEqual([]);
  });

  test('query containing a CJK codepoint returns []', () => {
    expect(findMatches('hello world', '中文', false)).toEqual([]);
  });

  test('Latin-1 codepoint U+00E9 (e-acute) IS valid and matches its counterpart', () => {
    // 0xE9 is valid Latin-1. The corpus is byte-for-codepoint Latin-1 from
    // the backend's latin1Decode, so a JS string containing U+00E9 DOES match.
    const corpus = 'café byteÿ';
    const matches = findMatches(corpus, 'café', false);
    expect(matches).toHaveLength(1);
    expect(matches[0].start).toBe(0);
  });

  test('Latin-1 codepoint U+00FF (y-dieresis) IS valid and matches its counterpart', () => {
    const corpus = 'café byteÿ';
    const matches = findMatches(corpus, 'byteÿ', false);
    expect(matches).toHaveLength(1);
  });
});

// ---------------------------------------------------------------------------
// 10-2-UNIT-008 [P0] AC#4: line number is 1-based and derived from a
// memoized line-start offset table.
// ---------------------------------------------------------------------------

describe('10-2-UNIT-008: line numbers are 1-based', () => {
  test('match on line 1 has line=1', () => {
    expect(findMatches('foo\nbar\nbaz', 'foo', false)[0].line).toBe(1);
  });

  test('match on line 2 has line=2 (LF separator)', () => {
    expect(findMatches('foo\nbar\nbaz', 'bar', false)[0].line).toBe(2);
  });

  test('match on line 3 has line=3 (LF separator)', () => {
    expect(findMatches('foo\nbar\nbaz', 'baz', false)[0].line).toBe(3);
  });

  test('match across a CRLF boundary stays on the line it begins on', () => {
    // Corpus: "ab\r\ncd" -- match "bc" spans the line break, but starts on
    // line 1. The PlainTextView splits on /\r\n?|\n/ so the displayed lines
    // are "ab" and "cd"; the find layer reports the start-line.
    const corpus = 'ab\r\ncd';
    const matches = findMatches(corpus, 'bc', false);
    if (matches.length > 0) {
      // Implementation choice: across-newline matches are valid (raw byte
      // search). They report the start-line. If the impl chooses to forbid
      // cross-line matches, that's also acceptable per spec -- in that case
      // the array is empty.
      expect(matches[0].line).toBe(1);
    }
  });
});

// ---------------------------------------------------------------------------
// 10-2-UNIT-009 [P0] AC#4: buildLineStartOffsets returns 1-based-line offset
// table where index i is the code-unit offset of line (i+1).
// ---------------------------------------------------------------------------

describe('10-2-UNIT-009: buildLineStartOffsets', () => {
  test('empty corpus -> table with a single zero (line 1 starts at offset 0)', () => {
    expect(buildLineStartOffsets('')).toEqual([0]);
  });

  test('"foo" (no newlines) -> single zero entry', () => {
    expect(buildLineStartOffsets('foo')).toEqual([0]);
  });

  test('"foo\\nbar" -> [0, 4] (line 1 at 0, line 2 at 4 after the LF)', () => {
    expect(buildLineStartOffsets('foo\nbar')).toEqual([0, 4]);
  });

  test('"foo\\r\\nbar" -> [0, 5] (line 2 starts AFTER both CR and LF)', () => {
    expect(buildLineStartOffsets('foo\r\nbar')).toEqual([0, 5]);
  });

  test('"foo\\rbar" -> [0, 4] (lone CR also breaks a line)', () => {
    expect(buildLineStartOffsets('foo\rbar')).toEqual([0, 4]);
  });

  test('"a\\nb\\nc\\nd" -> [0, 2, 4, 6]', () => {
    expect(buildLineStartOffsets('a\nb\nc\nd')).toEqual([0, 2, 4, 6]);
  });
});

// ---------------------------------------------------------------------------
// 10-2-UNIT-010 [P0] AC#4: zero-byte / empty corpus edge case.
// ---------------------------------------------------------------------------

describe('10-2-UNIT-010: empty corpus', () => {
  test('empty corpus + any non-empty query returns []', () => {
    expect(findMatches('', 'anything', false)).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// 10-2-UNIT-011 [P0] AC#4: Match.end == Match.start + query.length for the
// case-insensitive path (no normalization expansion).
// ---------------------------------------------------------------------------

describe('10-2-UNIT-011: match end aligns with start + query.length', () => {
  test('case-insensitive: end - start equals query.length', () => {
    const matches = findMatches('FooBarFooBaz', 'foo', false);
    matches.forEach((m: Match) => {
      expect(m.end - m.start).toBe('foo'.length);
    });
  });

  test('case-sensitive: end - start equals query.length', () => {
    const matches = findMatches('FooBarFooBaz', 'Foo', true);
    matches.forEach((m: Match) => {
      expect(m.end - m.start).toBe('Foo'.length);
    });
  });
});

// ---------------------------------------------------------------------------
// 10-2-UNIT-012 [P1] AC#19: performance budget on a 25 MiB synthetic corpus
// with ~10000 matches completes well under the CI ceiling.
//
// The CI assertion uses 500 ms to accommodate slow GHA runners; local dev
// typically sees <50 ms (documented in Dev Notes). This test is the only
// place the perf budget is asserted -- if it fails, look for an algorithmic
// regression (overlapping search, accidental regex, missed Latin-1 fast path).
// ---------------------------------------------------------------------------

describe('10-2-UNIT-012: performance budget', () => {
  test('25 MiB corpus with 10000 matches completes in under 500 ms', () => {
    const chunk = 'helvetica padded with some other bytes to space them out\n';
    // Tuned so the final corpus is ~25 MiB and contains ~10000 "helvetica"
    // matches. The exact count depends on chunk length; the test asserts
    // both a lower bound on match count and an upper bound on elapsed time.
    const repeat = Math.ceil((25 * 1024 * 1024) / chunk.length);
    const corpus = chunk.repeat(repeat);
    const start = performance.now();
    const matches = findMatches(corpus, 'helvetica', false);
    const elapsed = performance.now() - start;
    expect(matches.length).toBeGreaterThan(1000);
    expect(elapsed).toBeLessThan(500);
  });
});

// ---------------------------------------------------------------------------
// 10-2-UNIT-013 [P1] AC#4: query equal to the entire corpus returns one match
// covering [0, corpus.length).
// ---------------------------------------------------------------------------

describe('10-2-UNIT-013: query equal to corpus', () => {
  test('query == corpus returns single match spanning the whole corpus', () => {
    const corpus = 'exactly this content';
    const matches = findMatches(corpus, corpus, false);
    expect(matches).toHaveLength(1);
    expect(matches[0].start).toBe(0);
    expect(matches[0].end).toBe(corpus.length);
  });
});
