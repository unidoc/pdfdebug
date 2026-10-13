/**
 * @file Viewport virtualization for fixed-height rows. Consolidates the
 * hand-rolled windowing that XRefTableView, FontMappingTable, and PlainTextView
 * each carried: a scroll ref, scrollTop tracking, a ResizeObserver measuring
 * the viewport, and the firstVisible/lastVisible + spacer math that renders only
 * the visible slice of a large row set.
 */
import { useCallback, useEffect, useRef, useState, type MutableRefObject } from 'react';

/** Inputs to {@link useWindowedRows}. */
export interface UseWindowedRowsOptions {
  /** Total number of rows in the full (unwindowed) data set. */
  rowCount: number;
  /** Fixed pixel height of one row; the windowing math assumes every row is
   *  exactly this tall (callers clamp rows to guarantee it). */
  rowHeight: number;
  /** Rows rendered above and below the viewport for smooth scrolling. */
  overscan: number;
  /** Viewport height (px) used before the first measure and under jsdom, where
   *  clientHeight reads 0. Keeps the initial window bounded rather than rendering
   *  every row. */
  viewportFallback?: number;
}

/** Windowing state and handles returned by {@link useWindowedRows}. */
export interface WindowedRows {
  /** Attach to the scroll container. `.current` is readable for imperative
   *  scroll adjustments (keyboard navigation, match auto-scroll). */
  scrollRef: MutableRefObject<HTMLDivElement | null>;
  /** Current scroll offset (px). Exposed so callers can key effects on window
   *  shifts. */
  scrollTop: number;
  /** onScroll handler for the scroll container; keeps scrollTop in sync. */
  onScroll: (e: React.UIEvent<HTMLDivElement>) => void;
  /** First row index committed to the DOM (overscan included). */
  firstVisible: number;
  /** One past the last row index committed to the DOM. */
  lastVisible: number;
  /** Scroll height (px) of the rows above the window; a top spacer reserves it. */
  topPad: number;
  /** Scroll height (px) of the rows below the window; a bottom spacer reserves it. */
  bottomPad: number;
  /** Scroll the container to the top and reset the window. */
  scrollToTop: () => void;
  /** Re-read the container's scrollTop into state after an imperative
   *  `scrollTop` assignment (a programmatic assignment does not fire onScroll in
   *  every environment). */
  syncScrollTop: () => void;
}

/**
 * Windows a fixed-height row list to its visible slice. Returns the scroll ref,
 * onScroll handler, the visible index range, and the top/bottom spacer heights
 * that keep the scrollbar geometry matching the full data set.
 */
export function useWindowedRows({
  rowCount,
  rowHeight,
  overscan,
  viewportFallback = 320,
}: UseWindowedRowsOptions): WindowedRows {
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(0);

  // Measure the viewport and re-measure on resize. Keyed on rowCount so the
  // observer attaches once the scroll container mounts: callers gate the
  // container behind a data-present check, so scrollRef.current is null until
  // rowCount goes 0 -> N.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    setViewportHeight(el.clientHeight);
    if (typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(() => setViewportHeight(el.clientHeight));
    ro.observe(el);
    return () => ro.disconnect();
  }, [rowCount]);

  const onScroll = useCallback((e: React.UIEvent<HTMLDivElement>) => {
    setScrollTop(e.currentTarget.scrollTop);
  }, []);

  const scrollToTop = useCallback(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = 0;
    setScrollTop(0);
  }, []);

  const syncScrollTop = useCallback(() => {
    if (scrollRef.current) setScrollTop(scrollRef.current.scrollTop);
  }, []);

  const visibleCount = Math.ceil((viewportHeight || viewportFallback) / rowHeight) + overscan * 2;
  // Clamp firstVisible so a stale-large scrollTop over a just-shrunken row set
  // can never start the window past the tail (which would slice to an empty
  // window with an oversized top spacer for one frame). Keeps a full window at
  // the bottom instead of relying on a post-paint scroll-reset.
  const maxFirst = Math.max(0, rowCount - visibleCount);
  const firstVisible = Math.min(Math.max(0, Math.floor(scrollTop / rowHeight) - overscan), maxFirst);
  const lastVisible = Math.min(rowCount, firstVisible + visibleCount);
  const topPad = firstVisible * rowHeight;
  const bottomPad = (rowCount - lastVisible) * rowHeight;

  return {
    scrollRef,
    scrollTop,
    onScroll,
    firstVisible,
    lastVisible,
    topPad,
    bottomPad,
    scrollToTop,
    syncScrollTop,
  };
}
