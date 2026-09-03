/**
 * Frontend Hook and Render-Path Correctness
 * useWindowedRows viewport-virtualization hook.
 *
 * Run: cd frontend && npx vitest run src/hooks/useWindowedRows.test.ts
 */
import { renderHook, act } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import type { UIEvent } from 'react';

async function loadHook() {
  const specifier = './useWindowedRows';
  const mod = await import(/* @vite-ignore */ specifier);
  return mod.useWindowedRows as typeof import('./useWindowedRows').useWindowedRows;
}

/** Fires the hook's onScroll with a synthetic event carrying the given scrollTop.
 *  jsdom reports clientHeight 0, so the window uses viewportFallback -- keeping
 *  the visible count deterministic across environments. */
function scrollTo(
  result: { current: { onScroll: (e: UIEvent<HTMLDivElement>) => void } },
  scrollTop: number,
) {
  act(() => {
    result.current.onScroll({ currentTarget: { scrollTop } } as unknown as UIEvent<HTMLDivElement>);
  });
}

describe('useWindowedRows window math', () => {
  test('initial window starts at row 0 and is bounded by the viewport fallback', async () => {
    const useWindowedRows = await loadHook();
    const { result } = renderHook(() =>
      useWindowedRows({ rowCount: 1000, rowHeight: 20, overscan: 10, viewportFallback: 400 }),
    );
    // visibleCount = ceil(400/20) + 10*2 = 40.
    expect(result.current.firstVisible).toBe(0);
    expect(result.current.lastVisible).toBe(40);
    expect(result.current.topPad).toBe(0);
    expect(result.current.bottomPad).toBe((1000 - 40) * 20);
  });

  test('scrolling shifts the window and the spacer pads by rowHeight', async () => {
    const useWindowedRows = await loadHook();
    const { result } = renderHook(() =>
      useWindowedRows({ rowCount: 1000, rowHeight: 20, overscan: 10, viewportFallback: 400 }),
    );
    scrollTo(result, 2000);
    // firstVisible = floor(2000/20) - 10 = 90; lastVisible = 90 + 40 = 130.
    expect(result.current.firstVisible).toBe(90);
    expect(result.current.lastVisible).toBe(130);
    expect(result.current.topPad).toBe(90 * 20);
    expect(result.current.bottomPad).toBe((1000 - 130) * 20);
  });

  test('lastVisible clamps to rowCount and bottomPad reaches 0 at the end', async () => {
    const useWindowedRows = await loadHook();
    const { result } = renderHook(() =>
      useWindowedRows({ rowCount: 50, rowHeight: 20, overscan: 10, viewportFallback: 400 }),
    );
    scrollTo(result, 100000);
    expect(result.current.lastVisible).toBe(50);
    expect(result.current.bottomPad).toBe(0);
  });

  test('a stale-large scrollTop over a short list clamps firstVisible so the window is never empty', async () => {
    const useWindowedRows = await loadHook();
    // visibleCount = ceil(400/20) + 20 = 40; maxFirst = max(0, 100 - 40) = 60.
    const { result } = renderHook(() =>
      useWindowedRows({ rowCount: 100, rowHeight: 20, overscan: 10, viewportFallback: 400 }),
    );
    scrollTo(result, 100000);
    expect(result.current.firstVisible).toBe(60);
    expect(result.current.lastVisible).toBe(100);
    // Non-empty window with a bounded top spacer, not slice([]) with an
    // oversized pad.
    expect(result.current.lastVisible).toBeGreaterThan(result.current.firstVisible);
    expect(result.current.topPad).toBe(60 * 20);
  });

  test('when the list is shorter than the window, firstVisible stays 0', async () => {
    const useWindowedRows = await loadHook();
    // rowCount 30 < visibleCount 40, so the whole list renders from the top.
    const { result } = renderHook(() =>
      useWindowedRows({ rowCount: 30, rowHeight: 20, overscan: 10, viewportFallback: 400 }),
    );
    scrollTo(result, 100000);
    expect(result.current.firstVisible).toBe(0);
    expect(result.current.lastVisible).toBe(30);
    expect(result.current.topPad).toBe(0);
    expect(result.current.bottomPad).toBe(0);
  });

  test('scrollToTop resets the window to row 0', async () => {
    const useWindowedRows = await loadHook();
    const { result } = renderHook(() =>
      useWindowedRows({ rowCount: 1000, rowHeight: 20, overscan: 10, viewportFallback: 400 }),
    );
    scrollTo(result, 2000);
    expect(result.current.firstVisible).toBe(90);
    act(() => result.current.scrollToTop());
    expect(result.current.firstVisible).toBe(0);
    expect(result.current.topPad).toBe(0);
  });

  test('scrollRef, onScroll, and scrollToTop keep stable identity across renders', async () => {
    const useWindowedRows = await loadHook();
    const { result, rerender } = renderHook(
      ({ rowCount }: { rowCount: number }) =>
        useWindowedRows({ rowCount, rowHeight: 20, overscan: 10 }),
      { initialProps: { rowCount: 100 } },
    );
    const { scrollRef, onScroll, scrollToTop } = result.current;
    rerender({ rowCount: 200 });
    expect(result.current.scrollRef).toBe(scrollRef);
    expect(result.current.onScroll).toBe(onScroll);
    expect(result.current.scrollToTop).toBe(scrollToTop);
  });
});
