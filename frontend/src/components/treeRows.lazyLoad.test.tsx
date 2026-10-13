import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { useLazyChildren, useLazyLoad, type TreeNodeData } from './treeRows';

const mockGetChildren = vi.fn();
vi.mock('../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  GetChildren: (...args: unknown[]) => mockGetChildren(...args),
}));

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => { resolve = r; });
  return { promise, resolve };
}

const row = { id: 'r1', backendId: 'obj:0:5', children: [] } as unknown as TreeNodeData;

describe('lazy row loading', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockGetChildren.mockReset();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  test('the children spinner shows only while the tab that asked is active', async () => {
    const pending = deferred<unknown[]>();
    mockGetChildren.mockReturnValue(pending.promise);
    const { result, rerender } = renderHook(({ tabId }) => useLazyChildren(tabId, () => row, () => {}), {
      initialProps: { tabId: 'tab-a' as string | null },
    });

    let toggled!: Promise<void>;
    act(() => { toggled = result.current.toggle('r1'); });
    act(() => { vi.advanceTimersByTime(200); });
    expect(result.current.loadingNodeId).toBe('r1');

    rerender({ tabId: 'tab-b' });
    expect(result.current.loadingNodeId).toBeNull();

    rerender({ tabId: 'tab-a' });
    expect(result.current.loadingNodeId).toBe('r1');

    await act(async () => {
      pending.resolve([]);
      await toggled;
    });
    expect(result.current.loadingNodeId).toBeNull();
  });

  test('a superseded request does not apply after the row was fetched again', async () => {
    const { result } = renderHook(() => useLazyLoad('tab-a'));
    const first = deferred<string>();
    const second = deferred<string>();
    const third = deferred<string>();
    const applied: string[] = [];
    const apply = (v: string) => { applied.push(v); };

    let p1!: Promise<void>, p2!: Promise<void>, p3!: Promise<void>;
    act(() => { p1 = result.current.load('tab-a', 'r1', () => first.promise, apply); });
    act(() => { p2 = result.current.load('tab-a', 'r1', () => second.promise, apply); });
    await act(async () => { second.resolve('second'); await p2; });
    act(() => { p3 = result.current.load('tab-a', 'r1', () => third.promise, apply); });
    await act(async () => { first.resolve('first'); await p1; });
    await act(async () => { third.resolve('third'); await p3; });

    expect(applied).toEqual(['second', 'third']);
  });

  test('a failure reaches onError only for the latest request', async () => {
    const { result } = renderHook(() => useLazyLoad('tab-a'));
    const errors: unknown[] = [];
    await act(async () => {
      await result.current.load('tab-a', 'r1', () => Promise.reject(new Error('boom')), () => {}, (e) => { errors.push(e); });
    });
    expect(errors).toHaveLength(1);
    expect((errors[0] as Error).message).toBe('boom');
  });
});
