/**
 * Frontend Hook and Render-Path Correctness
 * useWindowedRows viewport measurement + ResizeObserver path.
 *
 * The pure-math cases live in useWindowedRows.test.ts; those never attach
 * scrollRef, so the measure effect exits at !el. These mount a real container
 * with a mocked ResizeObserver to cover the measured-height, resize, 0-to-N
 * attach, and disconnect-on-unmount paths.
 *
 * Run: cd frontend && npx vitest run src/hooks/useWindowedRows.measure.test.tsx
 */
import { render, cleanup, act } from '@testing-library/react';
import { describe, test, expect, beforeEach, afterEach } from 'vitest';
import { useWindowedRows } from './useWindowedRows';

let roCallbacks: Array<() => void> = [];
let observeCount = 0;
let disconnectCount = 0;

class MockResizeObserver {
  constructor(cb: () => void) {
    roCallbacks.push(cb);
  }
  observe() {
    observeCount += 1;
  }
  unobserve() {}
  disconnect() {
    disconnectCount += 1;
  }
}

beforeEach(() => {
  roCallbacks = [];
  observeCount = 0;
  disconnectCount = 0;
  (globalThis as { ResizeObserver?: unknown }).ResizeObserver = MockResizeObserver;
});

afterEach(() => {
  cleanup();
  delete (globalThis as { ResizeObserver?: unknown }).ResizeObserver;
});

/** Renders the hook's lastVisible into the DOM and attaches scrollRef to a
 *  real container (only when rowCount > 0, mirroring the callers' gate). */
function Harness({ rowCount }: { rowCount: number }) {
  const { scrollRef, onScroll, lastVisible } = useWindowedRows({
    rowCount,
    rowHeight: 20,
    overscan: 10,
    viewportFallback: 400,
  });
  if (rowCount === 0) {
    return <span data-testid="last">{lastVisible}</span>;
  }
  return (
    <div ref={scrollRef} data-testid="scroll" onScroll={onScroll}>
      <span data-testid="last">{lastVisible}</span>
    </div>
  );
}

function setClientHeight(el: HTMLElement, value: number) {
  Object.defineProperty(el, 'clientHeight', { configurable: true, value });
}

describe('useWindowedRows viewport measurement', () => {
  test('a measured height replaces the fallback and drives the window', () => {
    const { getByTestId } = render(<Harness rowCount={1000} />);
    // clientHeight is 0 in jsdom at mount, so the window uses the 400 fallback:
    // ceil(400/20) + 20 = 40.
    expect(getByTestId('last').textContent).toBe('40');
    expect(observeCount).toBe(1);

    // A real measurement of 200px arrives through the ResizeObserver callback:
    // ceil(200/20) + 20 = 30.
    setClientHeight(getByTestId('scroll'), 200);
    act(() => roCallbacks.forEach((cb) => cb()));
    expect(getByTestId('last').textContent).toBe('30');
  });

  test('a resize re-measures and re-windows', () => {
    const { getByTestId } = render(<Harness rowCount={1000} />);
    setClientHeight(getByTestId('scroll'), 200);
    act(() => roCallbacks.forEach((cb) => cb()));
    expect(getByTestId('last').textContent).toBe('30');

    // Shrink to 100px: ceil(100/20) + 20 = 25.
    setClientHeight(getByTestId('scroll'), 100);
    act(() => roCallbacks.forEach((cb) => cb()));
    expect(getByTestId('last').textContent).toBe('25');
  });

  test('the observer attaches only once the container mounts (rowCount 0 -> N)', () => {
    const { rerender } = render(<Harness rowCount={0} />);
    // No container while empty, so no observer is created.
    expect(roCallbacks.length).toBe(0);
    expect(observeCount).toBe(0);

    rerender(<Harness rowCount={1000} />);
    // The rowCount-keyed effect re-runs, finds the now-mounted container, and
    // attaches.
    expect(roCallbacks.length).toBe(1);
    expect(observeCount).toBe(1);
  });

  test('the observer disconnects on unmount', () => {
    const { unmount } = render(<Harness rowCount={1000} />);
    expect(disconnectCount).toBe(0);
    unmount();
    expect(disconnectCount).toBe(1);
  });
});
