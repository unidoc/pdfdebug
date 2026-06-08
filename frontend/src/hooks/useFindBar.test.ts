/**
 * Story 10.2: Find Bar in Plain Text View -- useFindBar hook red-phase suite.
 *
 * TDD RED PHASE: every test below fails until frontend/src/hooks/useFindBar.ts
 * is implemented per Task 3.
 *
 * Scope:
 * - openBar / closeBar / setQuery / next / prev surface
 * - Cmd+F / Ctrl+F keystroke handler (open + re-focus + AC2 select-all path)
 * - Esc close path (scoped via the FindBar root)
 * - F3 / Shift+F3 stepping with the bar closed (AC9)
 * - openedOnce flag: persists across Esc-close on the same tab, clears on tab change
 * - activeIndex preservation algorithm on case-toggle (AC10)
 * - Wrap-status one-shot flag (AC7 / AC8 / AC15)
 * - tabId-change reset (AC11)
 * - content===null gate (AC13)
 * - isInTextField focus guard (AC1 / AC22)
 * - nonLatin1 derived flag (AC12)
 *
 * Test IDs follow the 10-2-HOOK-NNN convention.
 *
 * Run: cd frontend && npx vitest run src/hooks/useFindBar.test.ts
 */
import { renderHook, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
// RED PHASE: this import fails until Task 3.1 lands.
import { useFindBar } from './useFindBar';

// Force the platform modifier to "Cmd" so keystroke assertions are platform
// invariant under jsdom. platform.ts reads navigator.platform; we stub it via
// Object.defineProperty so getPlatformModifier() returns 'Cmd'.
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

// Helper: dispatch a real KeyboardEvent on window so the hook's window-level
// listener catches it. fireEvent.keyDown does not bubble to window listeners
// reliably under jsdom; manual dispatchEvent is the canonical path.
function dispatchKey(opts: {
  key: string;
  metaKey?: boolean;
  ctrlKey?: boolean;
  shiftKey?: boolean;
  target?: EventTarget | null;
}) {
  const ev = new KeyboardEvent('keydown', {
    key: opts.key,
    metaKey: opts.metaKey ?? false,
    ctrlKey: opts.ctrlKey ?? false,
    shiftKey: opts.shiftKey ?? false,
    bubbles: true,
    cancelable: true,
  });
  if (opts.target) {
    Object.defineProperty(ev, 'target', { value: opts.target });
  }
  window.dispatchEvent(ev);
  return ev;
}

// ---------------------------------------------------------------------------
// 10-2-HOOK-001 [P0] AC#1: Cmd+F on an inactive Plain Text tab does NOT
// open the bar. active=false short-circuits the listener.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-001: active=false suppresses Cmd+F', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('Cmd+F dispatched while active=false does not open the bar', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'hello world', caseSensitive: false, active: false }),
    );
    expect(result.current.open).toBe(false);
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    expect(result.current.open).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-002 [P0] AC#1: Cmd+F on an active Plain Text tab with data ready
// opens the bar.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-002: Cmd+F opens the bar when active + data ready', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('Cmd+F opens the bar', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'hello world', caseSensitive: false, active: true }),
    );
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    expect(result.current.open).toBe(true);
  });

  test('Cmd+F sets openedOnce=true on first open', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'hello world', caseSensitive: false, active: true }),
    );
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    expect(result.current.openedOnce).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-003 [P0] AC#13: Cmd+F when content===null does NOT open the
// bar AND DOES call preventDefault on the keystroke.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-003: Cmd+F gated on content!==null', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('Cmd+F with content===null leaves open=false and preventDefault is called', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: null, caseSensitive: false, active: true }),
    );
    let ev: KeyboardEvent | null = null;
    act(() => {
      ev = dispatchKey({ key: 'f', metaKey: true });
    });
    expect(result.current.open).toBe(false);
    expect(ev!.defaultPrevented).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-004 [P0] AC#1, AC#22: Cmd+F when focus is in a non-FindBar text
// input does NOT open the bar. Mirrors App.jsx's isInTextField guard so the
// Cmd+K palette + ordinary search inputs are not stolen.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-004: focus-guard against unrelated text inputs', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('Cmd+F fired from an unrelated <input> does NOT open the bar', () => {
    const input = document.createElement('input');
    document.body.appendChild(input);
    try {
      const { result } = renderHook(() =>
        useFindBar({ tabId: 'tab-1', content: 'hello world', caseSensitive: false, active: true }),
      );
      act(() => {
        dispatchKey({ key: 'f', metaKey: true, target: input });
      });
      expect(result.current.open).toBe(false);
    } finally {
      document.body.removeChild(input);
    }
  });

  test('Cmd+F fired from a contentEditable element does NOT open the bar', () => {
    const div = document.createElement('div');
    div.contentEditable = 'true';
    document.body.appendChild(div);
    try {
      const { result } = renderHook(() =>
        useFindBar({ tabId: 'tab-1', content: 'hello world', caseSensitive: false, active: true }),
      );
      act(() => {
        dispatchKey({ key: 'f', metaKey: true, target: div });
      });
      expect(result.current.open).toBe(false);
    } finally {
      document.body.removeChild(div);
    }
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-005 [P0] AC#2: Cmd+F when bar is already open does NOT close it.
// AC2 mandates the bar stays open; the input is re-focused and contents are
// selected by the component (the hook surfaces an explicit `focusRequested`
// signal). We assert here only the state-level invariant: open stays true.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-005: Cmd+F is non-toggling', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('Cmd+F twice leaves the bar open (no toggle close)', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'hello world', caseSensitive: false, active: true }),
    );
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    expect(result.current.open).toBe(true);
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    expect(result.current.open).toBe(true);
  });

  // AC2: when the bar is already open, the second Cmd+F bumps focusVersion so
  // the FindBar component re-focuses + select-all on the input. The hook does
  // not own focus directly -- it signals via a monotonic counter that the
  // component watches in a useEffect.
  test('Cmd+F while open bumps focusVersion (the AC2 re-focus signal)', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'hello world', caseSensitive: false, active: true }),
    );
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    expect(result.current.open).toBe(true);
    const versionAfterFirstOpen = result.current.focusVersion;
    act(() => {
      dispatchKey({ key: 'f', metaKey: true });
    });
    expect(result.current.focusVersion).toBeGreaterThan(versionAfterFirstOpen);
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-006 [P0] AC#3: closeBar() flips open=false but PRESERVES the
// query, matches, and openedOnce flag on the same tab.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-006: closeBar preserves query + openedOnce', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('closeBar() preserves query, openedOnce stays true', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'foo bar foo', caseSensitive: false, active: true }),
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.setQuery('foo');
    });
    expect(result.current.open).toBe(true);
    expect(result.current.openedOnce).toBe(true);
    act(() => {
      result.current.closeBar();
    });
    expect(result.current.open).toBe(false);
    expect(result.current.query).toBe('foo');
    expect(result.current.openedOnce).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-007 [P0] AC#4: setQuery triggers a match recompute.
// useDeferredValue is acceptable; tests run act() so the deferred pass
// commits before assertions.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-007: setQuery recomputes matches', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('setQuery("foo") populates matches[]', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'foo bar foo baz foo', caseSensitive: false, active: true }),
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.setQuery('foo');
    });
    expect(result.current.matches.length).toBe(3);
    expect(result.current.activeIndex).toBe(0);
  });

  test('setQuery("") collapses matches to []', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'foo bar foo baz foo', caseSensitive: false, active: true }),
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.setQuery('foo');
    });
    expect(result.current.matches.length).toBe(3);
    act(() => {
      result.current.setQuery('');
    });
    expect(result.current.matches).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-008 [P0] AC#7: next() advances activeIndex with wrap; wrap-step
// sets the one-shot wrapped='top' flag.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-008: next() advances with wrap-status', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('next() advances activeIndex by 1 within bounds', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'foo foo foo', caseSensitive: false, active: true }),
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.setQuery('foo');
    });
    expect(result.current.activeIndex).toBe(0);
    act(() => {
      result.current.next();
    });
    expect(result.current.activeIndex).toBe(1);
    expect(result.current.wrapped).toBeNull();
  });

  test('next() at the last match wraps to 0 AND sets wrapped="top"', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'foo foo foo', caseSensitive: false, active: true }),
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
    expect(result.current.activeIndex).toBe(2);
    act(() => {
      result.current.next();
    });
    expect(result.current.activeIndex).toBe(0);
    expect(result.current.wrapped).toBe('top');
  });

  test('wrapped flag clears on the next navigation', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'foo foo', caseSensitive: false, active: true }),
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
    expect(result.current.wrapped).toBe('top');
    act(() => {
      result.current.next();
    });
    expect(result.current.wrapped).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-009 [P0] AC#8: prev() retreats with wrap; wrap-step sets the
// one-shot wrapped='bottom' flag.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-009: prev() retreats with wrap-status', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('prev() at activeIndex=0 wraps to last AND sets wrapped="bottom"', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'foo foo foo', caseSensitive: false, active: true }),
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.setQuery('foo');
    });
    expect(result.current.activeIndex).toBe(0);
    act(() => {
      result.current.prev();
    });
    expect(result.current.activeIndex).toBe(2);
    expect(result.current.wrapped).toBe('bottom');
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-010 [P0] AC#15: last-match-then-Enter wraps to first.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-010: AC15 wrap-to-top from last match', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('advancing past the last match wraps to 0', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'foo foo foo foo', caseSensitive: false, active: true }),
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.setQuery('foo');
    });
    const last = result.current.matches.length - 1;
    for (let i = 0; i < last; i++) {
      act(() => {
        result.current.next();
      });
    }
    expect(result.current.activeIndex).toBe(last);
    act(() => {
      result.current.next();
    });
    expect(result.current.activeIndex).toBe(0);
    expect(result.current.wrapped).toBe('top');
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-011 [P0] AC#9: F3 navigates when the bar is closed but
// openedOnce && query !== ''. The bar does NOT auto-reopen.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-011: F3 navigates when bar is closed (after openedOnce)', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('F3 with bar closed + openedOnce=true + query non-empty advances activeIndex', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'foo foo foo', caseSensitive: false, active: true }),
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.setQuery('foo');
    });
    act(() => {
      result.current.closeBar();
    });
    expect(result.current.open).toBe(false);
    act(() => {
      dispatchKey({ key: 'F3' });
    });
    expect(result.current.activeIndex).toBe(1);
    // Bar must NOT auto-reopen.
    expect(result.current.open).toBe(false);
  });

  test('Shift+F3 with bar closed retreats with wrap', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'foo foo foo', caseSensitive: false, active: true }),
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.setQuery('foo');
    });
    act(() => {
      result.current.closeBar();
    });
    act(() => {
      dispatchKey({ key: 'F3', shiftKey: true });
    });
    expect(result.current.activeIndex).toBe(2);
    expect(result.current.wrapped).toBe('bottom');
  });

  test('F3 with openedOnce=false (bar never opened) does NOT navigate', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'foo foo foo', caseSensitive: false, active: true }),
    );
    expect(result.current.openedOnce).toBe(false);
    act(() => {
      dispatchKey({ key: 'F3' });
    });
    expect(result.current.activeIndex).toBe(0);
    expect(result.current.open).toBe(false);
  });

  test('F3 with empty query does NOT navigate (matches.length === 0)', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'foo foo foo', caseSensitive: false, active: true }),
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.closeBar();
    });
    act(() => {
      dispatchKey({ key: 'F3' });
    });
    expect(result.current.activeIndex).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-012 [P0] AC#10: case-toggle activeIndex preservation algorithm.
// Captures prevStart, then either finds the same start in the new matches
// list OR resets activeIndex to 0.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-012: case-toggle preserves activeIndex when prevStart survives', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('toggling case keeps activeIndex on the same match when prevStart survives', () => {
    // Corpus: 4 lowercase + 2 uppercase "foo" -> 6 matches under
    // case-insensitive, 4 under case-sensitive (the lowercase ones).
    // Pre-toggle activeIndex sits on a lowercase match that survives the
    // flip.
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
    // Six matches under case-insensitive.
    expect(result.current.matches.length).toBe(6);
    // Pick the 3rd match (activeIndex=2). Its start offset must survive when
    // we flip to case-sensitive, because at offset 12 the corpus has a
    // lowercase "foo".
    act(() => {
      result.current.next();
    });
    act(() => {
      result.current.next();
    });
    const survivingStart = result.current.matches[result.current.activeIndex].start;
    rerender({ caseSensitive: true });
    // After the toggle, find the entry whose start === survivingStart in the
    // new matches array.
    const newIndex = result.current.matches.findIndex((m) => m.start === survivingStart);
    expect(newIndex).toBeGreaterThanOrEqual(0);
    expect(result.current.activeIndex).toBe(newIndex);
  });

  test('toggling case resets activeIndex to 0 when prevStart does NOT survive', () => {
    // Corpus chosen so the pre-toggle active match is an uppercase one that
    // does NOT survive the flip to case-sensitive.
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
    // 4 matches. Move to the first uppercase one (index 0, start 0).
    expect(result.current.matches[0].start).toBe(0);
    rerender({ caseSensitive: true });
    // The match at start=0 ("FOO") does NOT survive case-sensitive recompute.
    expect(result.current.matches.find((m) => m.start === 0)).toBeUndefined();
    expect(result.current.activeIndex).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-013 [P0] AC#4: setQuery resets activeIndex to 0 unconditionally.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-013: setQuery resets activeIndex', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('after navigating into a match list, a setQuery resets activeIndex to 0', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'foo foo foo bar bar bar', caseSensitive: false, active: true }),
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
    expect(result.current.activeIndex).toBe(2);
    act(() => {
      result.current.setQuery('bar');
    });
    expect(result.current.activeIndex).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-014 [P0] AC#11: tabId-change clears query, activeIndex, and
// openedOnce. The hook closes the bar.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-014: tabId-change resets find state', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('changing tabId resets open=false, query="", activeIndex=0, openedOnce=false', () => {
    const { result, rerender } = renderHook(
      ({ tabId }: { tabId: string }) =>
        useFindBar({ tabId, content: 'foo foo foo', caseSensitive: false, active: true }),
      { initialProps: { tabId: 'tab-1' } },
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
    expect(result.current.open).toBe(true);
    expect(result.current.openedOnce).toBe(true);
    expect(result.current.activeIndex).toBe(1);
    rerender({ tabId: 'tab-2' });
    expect(result.current.open).toBe(false);
    expect(result.current.query).toBe('');
    expect(result.current.activeIndex).toBe(0);
    expect(result.current.openedOnce).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-015 [P0] AC#12: nonLatin1 flag derives from query codepoints.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-015: nonLatin1 derived flag', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('Latin-1 query has nonLatin1=false', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'café byteÿ', caseSensitive: false, active: true }),
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.setQuery('café');
    });
    expect(result.current.nonLatin1).toBe(false);
  });

  test('query with U+2192 has nonLatin1=true and zero matches', () => {
    const { result } = renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'café byteÿ', caseSensitive: false, active: true }),
    );
    act(() => {
      result.current.openBar();
    });
    act(() => {
      result.current.setQuery('a→b');
    });
    expect(result.current.nonLatin1).toBe(true);
    expect(result.current.matches).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// 10-2-HOOK-016 [P0] AC#3: Cmd+F preventDefault contract. Cmd+F must consume
// the keystroke even when content===null (AC13) so the WebView's native find
// dialog does not surface.
// ---------------------------------------------------------------------------

describe('10-2-HOOK-016: Cmd+F preventDefault contract', () => {
  let restore: () => void;
  beforeEach(() => {
    restore = forceMacPlatform();
  });
  afterEach(() => {
    restore();
  });

  test('Cmd+F with active=true + data ready calls preventDefault', () => {
    renderHook(() =>
      useFindBar({ tabId: 'tab-1', content: 'hello world', caseSensitive: false, active: true }),
    );
    let ev: KeyboardEvent | null = null;
    act(() => {
      ev = dispatchKey({ key: 'f', metaKey: true });
    });
    expect(ev!.defaultPrevented).toBe(true);
  });
});
