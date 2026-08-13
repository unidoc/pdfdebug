/**
 * @file Find-bar state hook for Story 10-2. Owns the local find-bar state
 * (open / query / matches / activeIndex / wrapped / openedOnce) and wires the
 * Cmd+F / Ctrl+F / F3 / Shift+F3 keyboard listeners at window level, gated on
 * the active prop. Esc is NOT bound here -- the FindBar component scopes its
 * own Esc handler to its DOM subtree so the Cmd+K palette's Esc handler is
 * not co-fired (Story 10-2).
 */
import { useCallback, useDeferredValue, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { findMatches, buildLineStartOffsets, type Match } from '../lib/findMatches';
import { getPlatformModifier } from '../lib/platform';
import { useLatest } from './useLatest';

/** Arguments accepted by {@link useFindBar}. */
export interface UseFindBarArgs {
  /** Active document tab ID. A change resets all find-bar state. */
  tabId: string;
  /**
   * The Plain Text corpus. `null` when the load is not in the ready state;
   * Cmd+F consumes the keystroke but does not open the bar.
   */
  content: string | null;
  /** Per-tab case-sensitivity toggle (lives on TabState; sourced by caller). */
  caseSensitive: boolean;
  /** True when the Plain Text inner tab is active. Gates the listeners. */
  active: boolean;
}

/** Return shape of {@link useFindBar}. */
export interface UseFindBarReturn {
  open: boolean;
  query: string;
  matches: Match[];
  activeIndex: number;
  wrapped: 'top' | 'bottom' | null;
  openedOnce: boolean;
  nonLatin1: boolean;
  /** Line-start offset table memoized on content; consumed by per-row slicing. */
  lineStartOffsets: number[];
  /** Bumped when the bar should re-focus + select-all its input. */
  focusVersion: number;
  openBar: () => void;
  closeBar: () => void;
  setQuery: (q: string) => void;
  next: () => void;
  prev: () => void;
}

/** True when the event target is an unrelated text input (mirror App.jsx). */
function isInTextField(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA') return true;
  // isContentEditable is the canonical browser API but jsdom does not
  // implement it; fall through to contentEditable property + attribute.
  if (target.isContentEditable) return true;
  const ce = target.contentEditable;
  if (ce === 'true' || ce === 'plaintext-only') return true;
  return false;
}

/** True when the target sits inside an element with data-testid="plain-text-find-bar". */
function isInFindBar(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.closest('[data-testid="plain-text-find-bar"]') !== null;
}

/** True when the query contains any codepoint > U+00FF. */
function detectNonLatin1(query: string): boolean {
  for (const ch of query) {
    const cp = ch.codePointAt(0);
    if (cp !== undefined && cp > 0xff) return true;
  }
  return false;
}

/**
 * Find-bar state owner. Returns derived match index + navigation primitives;
 * the FindBar component is the presentational consumer.
 */
export function useFindBar(args: UseFindBarArgs): UseFindBarReturn {
  const { tabId, content, caseSensitive, active } = args;

  const [open, setOpen] = useState(false);
  const [query, setQueryState] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);
  const [wrapped, setWrapped] = useState<'top' | 'bottom' | null>(null);
  const [openedOnce, setOpenedOnce] = useState(false);
  const [focusVersion, setFocusVersion] = useState(0);

  // Deferred query feeds the match recompute so input character paint stays
  // synchronous on large corpora.
  const deferredQuery = useDeferredValue(query);

  // Memoized line-start offsets; recomputed only when content changes.
  const lineStartOffsets = useMemo(
    () => (content !== null ? buildLineStartOffsets(content) : [0]),
    [content],
  );

  // Memoized case-folded corpus (#20). On case-insensitive searches the
  // corpus-wide toLowerCase() runs at most once per (content, caseSensitive)
  // pair instead of once per keystroke. Latin-1 toLowerCase is length-
  // preserving, so haystack.length === content.length and match offsets index
  // identically into haystack and content (load-bearing invariant for line
  // numbers, which are computed from lineStartOffsets built off content).
  const haystack = useMemo(
    () => (content === null ? '' : caseSensitive ? content : content.toLowerCase()),
    [content, caseSensitive],
  );

  // Memoized match index. Empty when content is null. Passes the prebuilt
  // offset table and haystack so findMatches skips its internal rebuild +
  // toLowerCase (#8, #20).
  const matches = useMemo(() => {
    if (content === null) return [];
    return findMatches(content, deferredQuery, caseSensitive, lineStartOffsets, haystack);
  }, [content, deferredQuery, caseSensitive, lineStartOffsets, haystack]);

  const nonLatin1 = useMemo(() => detectNonLatin1(query), [query]);

  // Track the previous matches list so the case-toggle preservation algorithm
  // can capture prevStart BEFORE the recompute. We compare the current
  // deferredQuery + caseSensitive against the previous render to decide whether
  // a recompute was case-toggle-only (preserve activeIndex) or query-driven
  // (reset to 0).
  const prevDepsRef = useRef<{ query: string; caseSensitive: boolean; matches: Match[]; activeIndex: number }>({
    query: '',
    caseSensitive,
    matches: [],
    activeIndex: 0,
  });

  // Live mirrors of matches + activeIndex. next/prev read these to see the
  // latest list/index without re-binding; the reconciliation effect reads
  // activeIndexRef so it observes the value navigation last committed even
  // when next/prev changed no keyed dep (a snapshot would go stale).
  const matchesRef = useLatest(matches);
  const activeIndexRef = useLatest(activeIndex);

  // Reconcile activeIndex with the new matches list (query change -> reset to
  // 0; case-toggle -> preserve by start offset). Moved out of render (#7)
  // into a single useLayoutEffect keyed on the recompute inputs so no
  // prevDepsRef.current mutation happens during render. useLayoutEffect (not
  // useEffect) so activeIndex is settled before paint -- the active-match
  // highlight and the downstream auto-scroll both read it.
  useLayoutEffect(() => {
    const queryChanged = prevDepsRef.current.query !== deferredQuery;
    const caseFlipped = prevDepsRef.current.caseSensitive !== caseSensitive;
    if (!queryChanged && !caseFlipped) {
      // matches changed without a query/case change (e.g. content swap):
      // refresh the snapshot's matches so the next comparison is accurate.
      // Clamp activeIndex to the new list: a shrunk-but-nonempty matches list
      // would otherwise leave activeIndex past the end, surfacing as an
      // "N of M" counter with N > M and a missing active-match highlight.
      const current = activeIndexRef.current;
      const clamped = matches.length === 0 ? 0 : Math.min(current, matches.length - 1);
      prevDepsRef.current = {
        query: deferredQuery,
        caseSensitive,
        matches,
        activeIndex: clamped,
      };
      if (clamped !== current) {
        setActiveIndex(clamped);
      }
      return;
    }

    let nextIndex = 0;
    if (caseFlipped && !queryChanged) {
      // Preserve by start offset when surviving. Read the live
      // activeIndex (post-navigation), not the prior snapshot's.
      const prevStart = prevDepsRef.current.matches[activeIndexRef.current]?.start;
      if (prevStart !== undefined) {
        const found = matches.findIndex((m) => m.start === prevStart);
        nextIndex = found >= 0 ? found : 0;
      }
    }
    prevDepsRef.current = {
      query: deferredQuery,
      caseSensitive,
      matches,
      activeIndex: nextIndex,
    };
    setActiveIndex(nextIndex);
    // Clear wrapped on a query change so a prior navigation's one-shot status
    // does not stick around.
    if (queryChanged) {
      setWrapped(null);
    }
    // activeIndexRef is a stable ref; reading .current inside is intentional.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deferredQuery, caseSensitive, matches]);

  // Reset on document-tab change. Inner-tab toggle (active prop) does NOT
  // reset because PlainTextView stays mounted across inner-tab switches.
  useEffect(() => {
    setOpen(false);
    setQueryState('');
    setActiveIndex(0);
    setWrapped(null);
    setOpenedOnce(false);
    prevDepsRef.current = {
      query: '',
      caseSensitive,
      matches: [],
      activeIndex: 0,
    };
    // caseSensitive intentionally read at reset time; the per-tab value comes
    // from the new tab's TabState in the next render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tabId]);

  const openBar = useCallback(() => {
    setOpen(true);
    setOpenedOnce(true);
    setFocusVersion((v) => v + 1);
  }, []);

  const closeBar = useCallback(() => {
    setOpen(false);
    // PRESERVE query, matches, activeIndex, openedOnce so the Notepad++ "open
    // -> type -> Esc -> F3 F3 F3" muscle-memory path works (Story 10-2).
  }, []);

  const setQuery = useCallback((q: string) => {
    setQueryState(q);
  }, []);

  const next = useCallback(() => {
    const m = matchesRef.current;
    if (m.length === 0) return;
    const cur = activeIndexRef.current;
    const nxt = (cur + 1) % m.length;
    setActiveIndex(nxt);
    // Wrap-status is one-shot: set when the wrap fires, clear on the next
    // navigation. We clear unconditionally at the start of every next()/prev()
    // call and re-set when the wrap condition holds for this step.
    if (cur === m.length - 1 && m.length > 1) {
      setWrapped('top');
    } else {
      setWrapped(null);
    }
    // matchesRef / activeIndexRef are stable useLatest refs (identity never
    // changes); listed so exhaustive-deps stays clean without a disable.
  }, [matchesRef, activeIndexRef]);

  const prev = useCallback(() => {
    const m = matchesRef.current;
    if (m.length === 0) return;
    const cur = activeIndexRef.current;
    const nxt = cur === 0 ? m.length - 1 : cur - 1;
    setActiveIndex(nxt);
    if (cur === 0 && m.length > 1) {
      setWrapped('bottom');
    } else {
      setWrapped(null);
    }
  }, [matchesRef, activeIndexRef]);

  // Stable refs for the keystroke handler so we don't tear down + rebind on
  // every render.
  const openBarRef = useRef(openBar);
  openBarRef.current = openBar;
  const nextRef = useRef(next);
  nextRef.current = next;
  const prevRef = useRef(prev);
  prevRef.current = prev;

  const openRef = useLatest(open);
  const queryRef = useLatest(query);
  const openedOnceRef = useLatest(openedOnce);
  const contentRef = useLatest(content);

  // Window-level Cmd+F / Ctrl+F / F3 / Shift+F3 listener. Esc is handled in
  // the FindBar component (scoped to its own subtree).
  useEffect(() => {
    if (!active) return;
    const wantsMeta = getPlatformModifier() === 'Cmd';

    function onKeyDown(e: KeyboardEvent) {
      // Cmd+F / Ctrl+F: open the bar (or re-focus if already open).
      const mod = wantsMeta ? e.metaKey : e.ctrlKey;
      const targetIsFindBar = isInFindBar(e.target);
      const targetIsTextField = isInTextField(e.target);

      if (mod && (e.key === 'f' || e.key === 'F')) {
        // Focus-guard: a Cmd+F dispatched from a non-FindBar text input falls
        // through to whatever owns that input.
        if (targetIsTextField && !targetIsFindBar) return;
        // Consume the keystroke regardless of data state.
        e.preventDefault();
        if (contentRef.current === null) {
          // Data not ready -> no-op, but keystroke is consumed.
          return;
        }
        if (openRef.current) {
          // The bar is already open -> re-focus + select-all (driven by
          // focusVersion bump on the component side).
          setFocusVersion((v) => v + 1);
        } else {
          openBarRef.current();
        }
        return;
      }

      // F3 / Shift+F3: navigate matches even when the bar is closed, provided
      // openedOnce && query !== '' on the current tab.
      if (e.key === 'F3') {
        // Focus-guard: don't steal F3 from arbitrary text inputs.
        if (targetIsTextField && !targetIsFindBar) return;
        const gate = openRef.current || (openedOnceRef.current && queryRef.current !== '');
        if (!gate) return;
        e.preventDefault();
        if (e.shiftKey) {
          prevRef.current();
        } else {
          nextRef.current();
        }
        return;
      }
    }

    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
    // openRef / queryRef / openedOnceRef / contentRef are stable useLatest refs
    // (and openBarRef / nextRef / prevRef are stable useRef refs); only `active`
    // re-binds the listener. Listed to satisfy exhaustive-deps without a disable.
  }, [active, openRef, queryRef, openedOnceRef, contentRef]);

  return {
    open,
    query,
    matches,
    activeIndex,
    wrapped,
    openedOnce,
    nonLatin1,
    lineStartOffsets,
    focusVersion,
    openBar,
    closeBar,
    setQuery,
    next,
    prev,
  };
}
