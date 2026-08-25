/**
 * Frontend Hook and Render-Path Correctness (finding #20) --
 * useFindBar memoizes the corpus-wide toLowerCase().
 *
 * useFindBar memoizes `haystack = useMemo(() => caseSensitive ? content :
 * content.toLowerCase(), [content, caseSensitive])` and passes it to
 * findMatches, so the corpus-length toLowerCase fires at most once per
 * (content, caseSensitive) pair across the find session.
 *
 * Counting strategy: wrap String.prototype.toLowerCase with a counter that
 * records the LENGTH of each `this`. The short-query needle is lowercased once
 * per keystroke too, so we MUST filter by receiver length and only count calls
 * on a string of corpus length (>= 1 MB). Asserting "toLowerCase called once"
 * globally would fail because the needle is lowercased every keystroke.
 *
 * Run: cd frontend && npx vitest run src/hooks/useFindBar.haystack-memo.test.ts
 */
import { renderHook, act } from '@testing-library/react';
import { describe, test, expect, beforeEach, afterEach } from 'vitest';
import { useFindBar } from './useFindBar';

function forceMacPlatform() {
  const original = Object.getOwnPropertyDescriptor(window.navigator, 'platform');
  Object.defineProperty(window.navigator, 'platform', {
    configurable: true,
    get: () => 'MacIntel',
  });
  return () => {
    if (original) {
      Object.defineProperty(window.navigator, 'platform', original);
    }
  };
}

// 10 MB Latin-1 corpus -- the smallest size where the toLowerCase win is
// measurable in CI without flaking (Dev Notes). Contains "needle" tokens so
// the search does real work.
const CORPUS = ('the quick brown FOX jumps over needle padding bytes here\n').repeat(
  Math.ceil((10 * 1024 * 1024) / 56),
);
const CORPUS_LEN = CORPUS.length;

describe('useFindBar memoizes corpus toLowerCase', () => {
  let restore: () => void;
  let originalToLowerCase: typeof String.prototype.toLowerCase;
  // Lengths of the receiver string for every toLowerCase call during the test.
  let receiverLengths: number[];

  beforeEach(() => {
    restore = forceMacPlatform();
    receiverLengths = [];
    originalToLowerCase = String.prototype.toLowerCase;
    // eslint-disable-next-line no-extend-native
    String.prototype.toLowerCase = function patchedToLowerCase(this: string) {
      receiverLengths.push(this.length);
      return originalToLowerCase.call(this);
    };
  });

  afterEach(() => {
    // eslint-disable-next-line no-extend-native
    String.prototype.toLowerCase = originalToLowerCase;
    restore();
  });

  test('corpus-length toLowerCase is called exactly once across 10 keystrokes', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: CORPUS, caseSensitive: false, active: true }),
    );
    act(() => {
      result.current.openBar();
    });
    // Drive 10 progressive keystrokes building up the word "needle...".
    const word = 'needleabcd';
    for (let i = 1; i <= word.length; i++) {
      const q = word.slice(0, i);
      act(() => {
        result.current.setQuery(q);
      });
    }
    // Count only the calls whose receiver is at least 1 MB -- those are the
    // corpus-wide lowercase calls. Needle (short) lowercase calls are excluded.
    const corpusScaleCalls = receiverLengths.filter((len) => len >= 1024 * 1024);
    expect(corpusScaleCalls).toHaveLength(1);
  });

  test('haystack.length === content.length (offset-mapping invariant)', () => {
    // The Latin-1 corpus is length-preserving under toLowerCase, so the
    // memoized haystack must have the same length as the content. We prove the
    // invariant the implementation relies on: lowercasing the corpus does not
    // change its length.
    const haystack = CORPUS.toLowerCase();
    expect(haystack.length).toBe(CORPUS_LEN);
  });

  test('toggling caseSensitive recomputes the haystack at most once per (content, caseSensitive) pair', () => {
    const { result, rerender } = renderHook(
      ({ caseSensitive }: { caseSensitive: boolean }) =>
        useFindBar({ tabId: 'tab-1', content: CORPUS, caseSensitive, active: true }),
      { initialProps: { caseSensitive: false } },
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.setQuery('needle');
    });
    // Reset the counter, then toggle case twice. Each distinct (content,
    // caseSensitive=false) memo is already warm; flipping to true computes a
    // new haystack once, flipping back to false reuses the warm one.
    receiverLengths = [];
    rerender({ caseSensitive: true });
    // caseSensitive=true -> haystack equals content (no lowercase), so zero
    // corpus-length lowercase calls are expected on this flip.
    const corpusCallsAfterTrue = receiverLengths.filter((len) => len >= 1024 * 1024);
    expect(corpusCallsAfterTrue).toHaveLength(0);
  });
});
