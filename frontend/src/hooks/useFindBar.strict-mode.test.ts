/**
 * Frontend Hook and Render-Path Correctness (finding #7) --
 * useFindBar render-phase side-effect removal.
 *
 * The absence-of-warning check is NOT the pass condition -- the
 * idempotency assertion is. A console.error spy is included only as a
 * supplemental regression guard.
 *
 * StrictMode CANNOT be enabled globally in test-setup.ts (that file renders
 * nothing). It is enabled per-test via renderHook(fn, { reactStrictMode: true })
 * per Dev Notes.
 *
 * Test IDs follow the convention.
 *
 * Run: cd frontend && npx vitest run src/hooks/useFindBar.strict-mode.test.ts
 */
import { renderHook, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import { useFindBar } from './useFindBar';

// Force the platform modifier to "Cmd" so keystroke paths are deterministic.
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

// Drive an identical "open -> query 'foo' -> next -> next -> toggle case ->
// query 'bar'" sequence and return the final activeIndex + matches snapshot.
// The same driver is run under strict and non-strict render; the two results
// must be byte-identical.
function driveSequence(reactStrictMode: boolean) {
  const corpus = 'foo X FOO Y foo Z FOO W foo V foo bar BAR bar';
  const { result, rerender } = renderHook(
    ({ caseSensitive }: { caseSensitive: boolean }) =>
      useFindBar({ tabId: 'tab-1', content: corpus, caseSensitive, active: true }),
    { initialProps: { caseSensitive: false }, reactStrictMode },
  );
  act(() => {
    result.current.openBar();
  });
  act(() => {
    result.current.setQuery('foo');
  });
  act(() => {
    result.current.next();
  });
  act(() => {
    result.current.next();
  });
  // Case toggle (case-toggle-only path: preserve by start offset).
  rerender({ caseSensitive: true });
  const afterToggle = {
    activeIndex: result.current.activeIndex,
    starts: result.current.matches.map((m) => m.start),
  };
  // Query change (must reset activeIndex to 0).
  act(() => {
    result.current.setQuery('bar');
  });
  const afterQueryChange = {
    activeIndex: result.current.activeIndex,
    starts: result.current.matches.map((m) => m.start),
  };
  return { afterToggle, afterQueryChange };
}

describe('useFindBar is StrictMode-idempotent', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('StrictMode double-invoke yields the same activeIndex/matches as non-strict', () => {
    const nonStrict = driveSequence(false);
    const strict = driveSequence(true);
    // A: under StrictMode the double-invoke must not corrupt state.
    expect(strict.afterToggle.activeIndex).toBe(nonStrict.afterToggle.activeIndex);
    expect(strict.afterToggle.starts).toEqual(nonStrict.afterToggle.starts);
    expect(strict.afterQueryChange.activeIndex).toBe(nonStrict.afterQueryChange.activeIndex);
    expect(strict.afterQueryChange.starts).toEqual(nonStrict.afterQueryChange.starts);
  });

  test('a no-op render (none of the keyed deps change) does not stale prevDepsRef and the next case-toggle still preserves by start', () => {
    // Dev Notes call-out: after moving the snapshot into useLayoutEffect, a
    // render where none of [deferredQuery, caseSensitive, matches] changed must
    // NOT leave prevDepsRef stale in a way that breaks the next case-toggle.
    const corpus = 'foo X FOO Y foo Z FOO W foo';
    const { result, rerender } = renderHook(
      ({ caseSensitive, active }: { caseSensitive: boolean; active: boolean }) =>
        useFindBar({ tabId: 'tab-1', content: corpus, caseSensitive, active }),
      { initialProps: { caseSensitive: false, active: true }, reactStrictMode: true },
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.setQuery('foo');
    });
    act(() => {
      result.current.next();
    });
    act(() => {
      result.current.next();
    });
    const survivingStart = result.current.matches[result.current.activeIndex].start;
    // No-op render: flip an UNkeyed prop (active) without touching query/case/matches.
    rerender({ caseSensitive: false, active: false });
    rerender({ caseSensitive: false, active: true });
    // Now toggle case: preservation-by-start must still work.
    rerender({ caseSensitive: true, active: true });
    const newIndex = result.current.matches.findIndex((m) => m.start === survivingStart);
    expect(newIndex).toBeGreaterThanOrEqual(0);
    expect(result.current.activeIndex).toBe(newIndex);
  });

  // Supplemental ONLY (not the pass condition): the deprecated
  // cross-component warning must not regress.
  test('no React "Cannot update a component while rendering" warning regression', () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      driveSequence(true);
      const offending = errorSpy.mock.calls.filter((call) =>
        String(call[0] ?? '').includes('Cannot update a component while rendering a different component'),
      );
      expect(offending).toHaveLength(0);
    } finally {
      errorSpy.mockRestore();
    }
  });
});

describe('case-toggle activeIndex preservation survives the deps-comparison move', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('toggling case preserves activeIndex on the match whose start survives', () => {
    const corpus = 'foo X FOO Y foo Z FOO W foo V foo';
    const { result, rerender } = renderHook(
      ({ caseSensitive }: { caseSensitive: boolean }) =>
        useFindBar({ tabId: 'tab-1', content: corpus, caseSensitive, active: true }),
      { initialProps: { caseSensitive: false } },
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.setQuery('foo');
    });
    act(() => {
      result.current.next();
    });
    act(() => {
      result.current.next();
    });
    const survivingStart = result.current.matches[result.current.activeIndex].start;
    rerender({ caseSensitive: true });
    const newIndex = result.current.matches.findIndex((m) => m.start === survivingStart);
    expect(newIndex).toBeGreaterThanOrEqual(0);
    expect(result.current.activeIndex).toBe(newIndex);
  });

  test('toggling case resets activeIndex to 0 when prevStart does NOT survive', () => {
    const corpus = 'FOO X foo Y FOO Z foo';
    const { result, rerender } = renderHook(
      ({ caseSensitive }: { caseSensitive: boolean }) =>
        useFindBar({ tabId: 'tab-1', content: corpus, caseSensitive, active: true }),
      { initialProps: { caseSensitive: false } },
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.setQuery('foo');
    });
    expect(result.current.matches[0].start).toBe(0);
    rerender({ caseSensitive: true });
    expect(result.current.matches.find((m) => m.start === 0)).toBeUndefined();
    expect(result.current.activeIndex).toBe(0);
  });
});
