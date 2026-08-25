/**
 * Frontend Hook and Render-Path Correctness
 * (finding #28) -- useLatest ref-mirror consolidation hook.
 *
 * Run: cd frontend && npx vitest run src/hooks/useLatest.test.ts
 */
import { renderHook } from '@testing-library/react';
import { describe, test, expect } from 'vitest';

async function loadUseLatest() {
  // The @vite-ignore comment plus a non-literal specifier keeps Vite's
  // import analysis from statically resolving the module, so the import is
  // deferred to call time rather than suite-load time.
  const specifier = './useLatest';
  const mod = await import(/* @vite-ignore */ specifier);
  return mod.useLatest as (value: unknown) => { current: unknown };
}

// ---------------------------------------------------------------------------
// useLatest returns a ref whose .current reflects the latest render's value
// across rerenders.
// ---------------------------------------------------------------------------

describe('useLatest reflects the latest value', () => {
  test('ref.current equals the initial value on first render', async () => {
    const useLatest = await loadUseLatest();
    const { result } = renderHook(({ value }: { value: number }) => useLatest(value), {
      initialProps: { value: 1 },
    });
    expect(result.current.current).toBe(1);
  });

  test('ref.current updates to the new value after a rerender', async () => {
    const useLatest = await loadUseLatest();
    const { result, rerender } = renderHook(({ value }: { value: number }) => useLatest(value), {
      initialProps: { value: 1 },
    });
    expect(result.current.current).toBe(1);
    rerender({ value: 42 });
    expect(result.current.current).toBe(42);
  });

  test('the same ref object identity is preserved across rerenders', async () => {
    const useLatest = await loadUseLatest();
    const { result, rerender } = renderHook(({ value }: { value: string }) => useLatest(value), {
      initialProps: { value: 'a' },
    });
    const firstRef = result.current;
    rerender({ value: 'b' });
    // useRef returns a stable object; only .current changes.
    expect(result.current).toBe(firstRef);
    expect(result.current.current).toBe('b');
  });

  test('works with object values (reference identity tracked, not deep-copied)', async () => {
    const useLatest = await loadUseLatest();
    const objA = { id: 1 };
    const objB = { id: 2 };
    const { result, rerender } = renderHook(({ value }: { value: { id: number } }) => useLatest(value), {
      initialProps: { value: objA },
    });
    expect(result.current.current).toBe(objA);
    rerender({ value: objB });
    expect(result.current.current).toBe(objB);
  });
});
