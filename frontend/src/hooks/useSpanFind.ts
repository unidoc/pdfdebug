/**
 * @file Span-based find-bar state hook. Generalizes the Plain Text find bar
 * (see useFindBar) over an arbitrary "match source": a linear sequence of
 * matchable text spans a tab can highlight and scroll to. Each DetailPanel tab
 * that opts in (Object, XREF) supplies its own spans plus a matcher; this hook
 * owns the shared state (open / query / matches / activeIndex / wrapped /
 * openedOnce) and the Cmd+F / Ctrl+F / F3 / Shift+F3 window listener, gated on
 * the `active` prop so only the active tab's instance listens. Esc is handled
 * in FindBar (scoped to its subtree), not here.
 */
import { useCallback, useDeferredValue, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { getPlatformModifier } from '../lib/platform';
import { useLatest } from './useLatest';

/** One matchable text span in a tab's linear match source. */
export interface FindSpan {
  /** Stable identifier the tab uses to map a match back to a rendered element. */
  id: string;
  /** The searchable text of this span. */
  text: string;
}

/** One match inside a span. `start`/`end` index into that span's text. */
export interface SpanMatch {
  spanId: string;
  start: number;
  end: number;
}

/** Computes the flat, ordered match list for a query over the spans. */
export type SpanMatcher = (spans: FindSpan[], query: string, caseSensitive: boolean) => SpanMatch[];

/** Arguments accepted by {@link useSpanFind}. */
export interface UseSpanFindArgs {
  /**
   * Identity of the searchable content. A change resets all find-bar state
   * (closes the bar, clears the query). For a document-level source this is the
   * document tab id; for a per-selection source (the Object tab) it also folds
   * in the selected node, so navigating to a different object starts fresh.
   */
  resetKey: string;
  /** The tab's linear match source. */
  spans: FindSpan[];
  /** Maps a query to matches over the spans. */
  matcher: SpanMatcher;
  /** Per-document case-sensitivity toggle (shared TabState.findCaseSensitive). */
  caseSensitive: boolean;
  /** True when this instance's tab is active. Gates the window listener. */
  active: boolean;
  /**
   * True when the tab holds searchable content. When false, Cmd+F consumes the
   * keystroke but does not open the bar (mirrors the Plain Text null-content
   * gate).
   */
  ready: boolean;
  /** data-testid of this instance's find bar, for the focus-guard selector. */
  barTestId: string;
}

/** Return shape of {@link useSpanFind}. */
export interface UseSpanFindReturn {
  open: boolean;
  query: string;
  matches: SpanMatch[];
  activeIndex: number;
  activeMatch: SpanMatch | null;
  wrapped: 'top' | 'bottom' | null;
  openedOnce: boolean;
  focusVersion: number;
  openBar: () => void;
  closeBar: () => void;
  setQuery: (q: string) => void;
  next: () => void;
  prev: () => void;
}

/** True when the event target is an unrelated text input (mirror useFindBar). */
function isInTextField(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA') return true;
  if (target.isContentEditable) return true;
  const ce = target.contentEditable;
  if (ce === 'true' || ce === 'plaintext-only') return true;
  return false;
}

/**
 * Case-insensitive (or -sensitive) non-overlapping literal-substring matcher.
 * Non-Latin-1-capable: normal JS strings match (unlike findMatches, which
 * short-circuits on non-Latin-1 for the byte-oriented Plain Text corpus).
 */
export function substringSpanMatcher(spans: FindSpan[], query: string, caseSensitive: boolean): SpanMatch[] {
  if (query === '') return [];
  const lowerNeedle = query.toLowerCase();
  const out: SpanMatch[] = [];
  for (const span of spans) {
    // Match offsets index back into span.text, so the haystack must have the
    // same length as span.text. toLowerCase is length-preserving for Latin-1
    // and nearly all text, but a few code points (e.g. U+0130) fold to a
    // different length; matching those spans case-sensitively keeps offsets
    // valid instead of desyncing the highlight slices.
    let hay = span.text;
    let needle = query;
    if (!caseSensitive) {
      const lower = span.text.toLowerCase();
      if (lower.length === span.text.length) {
        hay = lower;
        needle = lowerNeedle;
      }
    }
    const step = needle.length;
    let from = 0;
    while (from <= hay.length - step) {
      const idx = hay.indexOf(needle, from);
      if (idx === -1) break;
      out.push({ spanId: span.id, start: idx, end: idx + step });
      from = idx + step;
    }
  }
  return out;
}

/** Stable key for a match so activeIndex can be preserved across a recompute. */
function matchKey(m: SpanMatch): string {
  return `${m.spanId}:${m.start}`;
}

/**
 * Span-based find-bar state owner. The FindBar component is the presentational
 * consumer; each tab renders its own highlights + scroll from `matches` and
 * `activeMatch`.
 */
export function useSpanFind(args: UseSpanFindArgs): UseSpanFindReturn {
  const { resetKey, spans, matcher, caseSensitive, active, ready, barTestId } = args;

  const [open, setOpen] = useState(false);
  const [query, setQueryState] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);
  const [wrapped, setWrapped] = useState<'top' | 'bottom' | null>(null);
  const [openedOnce, setOpenedOnce] = useState(false);
  const [focusVersion, setFocusVersion] = useState(0);

  const deferredQuery = useDeferredValue(query);

  const matches = useMemo(
    () => matcher(spans, deferredQuery, caseSensitive),
    [spans, deferredQuery, caseSensitive, matcher],
  );

  // Reconcile activeIndex with a new match list: reset to 0 on a query change,
  // preserve by match key on a case flip, clamp otherwise.
  const prevDepsRef = useRef<{ query: string; caseSensitive: boolean; matches: SpanMatch[] }>({
    query: '',
    caseSensitive,
    matches: [],
  });
  const matchesRef = useLatest(matches);
  const activeIndexRef = useLatest(activeIndex);

  useLayoutEffect(() => {
    const queryChanged = prevDepsRef.current.query !== deferredQuery;
    const caseFlipped = prevDepsRef.current.caseSensitive !== caseSensitive;
    if (!queryChanged && !caseFlipped) {
      // Reaching here means the match set changed under an unchanged query and
      // case (the span source was replaced, e.g. a new object was selected).
      // Clear any stale wrap indicator so it does not linger over new content.
      const current = activeIndexRef.current;
      const clamped = matches.length === 0 ? 0 : Math.min(current, matches.length - 1);
      prevDepsRef.current = { query: deferredQuery, caseSensitive, matches };
      if (clamped !== current) setActiveIndex(clamped);
      setWrapped(null);
      return;
    }
    let nextIndex = 0;
    if (caseFlipped && !queryChanged) {
      const prev = prevDepsRef.current.matches[activeIndexRef.current];
      if (prev) {
        const key = matchKey(prev);
        const found = matches.findIndex((m) => matchKey(m) === key);
        nextIndex = found >= 0 ? found : 0;
      }
    }
    prevDepsRef.current = { query: deferredQuery, caseSensitive, matches };
    setActiveIndex(nextIndex);
    // Both a query change and a case flip rebuild the match set, so a wrap
    // indicator left over from earlier navigation is stale in either case.
    setWrapped(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deferredQuery, caseSensitive, matches]);

  // Reset when the searchable content identity changes (document tab, or the
  // selected object for the Object tab). Inner-tab toggle (active prop) does NOT
  // reset, so each tab's find state persists across inner-tab switches.
  useEffect(() => {
    setOpen(false);
    setQueryState('');
    setActiveIndex(0);
    setWrapped(null);
    setOpenedOnce(false);
    prevDepsRef.current = { query: '', caseSensitive, matches: [] };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);

  // Close an already-open bar when the tab's content stops being searchable
  // (e.g. the selected object switched to a render path with no highlight
  // surface). `ready` gates opening; this stops a stranded "0 of 0" bar from
  // lingering over content it can never match.
  useEffect(() => {
    if (!ready) setOpen(false);
  }, [ready]);

  const openBar = useCallback(() => {
    setOpen(true);
    setOpenedOnce(true);
    setFocusVersion((v) => v + 1);
  }, []);

  const closeBar = useCallback(() => {
    // Closing clears the search: query, matches and highlights all go, so the
    // tab shows no leftover marks once the bar is dismissed. activeIndex is left
    // for the reconcile effect to reset once the empty query commits; forcing it
    // to 0 here would move the active match under the still-live (deferred)
    // match list and scroll the viewport for one frame.
    setOpen(false);
    setQueryState('');
    setWrapped(null);
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
    if (cur === m.length - 1 && m.length > 1) setWrapped('top');
    else setWrapped(null);
  }, [matchesRef, activeIndexRef]);

  const prev = useCallback(() => {
    const m = matchesRef.current;
    if (m.length === 0) return;
    const cur = activeIndexRef.current;
    const nxt = cur === 0 ? m.length - 1 : cur - 1;
    setActiveIndex(nxt);
    if (cur === 0 && m.length > 1) setWrapped('bottom');
    else setWrapped(null);
  }, [matchesRef, activeIndexRef]);

  const openBarRef = useRef(openBar);
  openBarRef.current = openBar;
  const nextRef = useRef(next);
  nextRef.current = next;
  const prevRef = useRef(prev);
  prevRef.current = prev;

  const openRef = useLatest(open);
  const queryRef = useLatest(query);
  const openedOnceRef = useLatest(openedOnce);
  const readyRef = useLatest(ready);
  const barTestIdRef = useLatest(barTestId);

  // Window-level Cmd+F / Ctrl+F / F3 / Shift+F3 listener, bound only while
  // active. Consumes ONLY Cmd+F and F3 -- Cmd+G / Cmd+K fall through untouched.
  useEffect(() => {
    if (!active) return;
    const wantsMeta = getPlatformModifier() === 'Cmd';

    function isInThisBar(target: EventTarget | null): boolean {
      if (!(target instanceof HTMLElement)) return false;
      return target.closest(`[data-testid="${barTestIdRef.current}"]`) !== null;
    }

    function onKeyDown(e: KeyboardEvent) {
      const mod = wantsMeta ? e.metaKey : e.ctrlKey;
      const targetIsFindBar = isInThisBar(e.target);
      const targetIsTextField = isInTextField(e.target);

      if (mod && (e.key === 'f' || e.key === 'F')) {
        if (targetIsTextField && !targetIsFindBar) return;
        e.preventDefault();
        if (!readyRef.current) return;
        if (openRef.current) setFocusVersion((v) => v + 1);
        else openBarRef.current();
        return;
      }

      if (e.key === 'F3') {
        if (targetIsTextField && !targetIsFindBar) return;
        const gate = openRef.current || (openedOnceRef.current && queryRef.current !== '');
        if (!gate) return;
        e.preventDefault();
        if (e.shiftKey) prevRef.current();
        else nextRef.current();
        return;
      }
    }

    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [active, openRef, queryRef, openedOnceRef, readyRef, barTestIdRef]);

  const activeMatch = matches[activeIndex] ?? null;

  return {
    open,
    query,
    matches,
    activeIndex,
    activeMatch,
    wrapped,
    openedOnce,
    focusVersion,
    openBar,
    closeBar,
    setQuery,
    next,
    prev,
  };
}
