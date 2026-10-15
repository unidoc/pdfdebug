/**
 * Per-tab on-disk status store, kept outside AppState so a status update
 * re-renders only the subscribers of that tab.
 *
 * Contract used here:
 *   applyDiskStatusEvent({tabId, seq, state, loadedModTime, diskModTime, diskSize})
 *     - drops an event whose seq is not above the last one applied for the tab
 *       (also after the entry was removed)
 *     - a 'current' event removes the tab's entry
 *   setDiskStatusFromCheck(tabId, {state, loadedModTime, diskModTime, diskSize}, seqAtRequest)
 *     - writes a GetDiskStatus result; 'current' removes the entry
 *   retainDiskStatus(tabIds) drops entries for every other tab
 *   readDiskStatus(tabId) / useDiskStatus(tabId) return the entry or undefined
 *   resetDiskStatusStore() empties the store
 *
 * Run: cd frontend && npx vitest run src/lib/diskStatusStore.test.ts
 */
import { renderHook, act } from '@testing-library/react';
import { describe, test, expect, beforeEach } from 'vitest';
import {
  applyDiskStatusEvent,
  setDiskStatusFromCheck,
  diskStatusSeq,
  retainDiskStatus,
  readDiskStatus,
  useDiskStatus,
  resetDiskStatusStore,
} from './diskStatusStore';

const loaded = 1_768_219_200_000;

function ev(tabId: string, seq: number, state: string, diskSize = 200) {
  return { tabId, seq, state, loadedModTime: loaded, diskModTime: state === 'missing' ? 0 : loaded + 60_000, diskSize: state === 'missing' ? 0 : diskSize };
}

beforeEach(() => {
  resetDiskStatusStore();
});

describe('diskStatusStore', () => {
  test('a changed event creates an entry with the payload', () => {
    applyDiskStatusEvent(ev('t1', 1, 'changed'));
    expect(readDiskStatus('t1')).toMatchObject({ seq: 1, state: 'changed', loadedModTime: loaded, diskModTime: loaded + 60_000, diskSize: 200 });
  });

  test('a current event removes the entry', () => {
    applyDiskStatusEvent(ev('t1', 1, 'changed'));
    applyDiskStatusEvent(ev('t1', 2, 'current'));
    expect(readDiskStatus('t1')).toBeUndefined();
  });

  test('an event whose seq is not above the last applied one changes nothing', () => {
    applyDiskStatusEvent(ev('t1', 5, 'changed', 300));
    applyDiskStatusEvent(ev('t1', 4, 'missing'));
    applyDiskStatusEvent(ev('t1', 5, 'current'));
    expect(readDiskStatus('t1')).toMatchObject({ seq: 5, state: 'changed', diskSize: 300 });
  });

  test('a late lower-seq change does not re-mark a tab that went back to current', () => {
    applyDiskStatusEvent(ev('t1', 2, 'changed'));
    applyDiskStatusEvent(ev('t1', 3, 'current'));
    applyDiskStatusEvent(ev('t1', 2, 'changed'));
    expect(readDiskStatus('t1')).toBeUndefined();
  });

  test('seq is tracked per tab', () => {
    applyDiskStatusEvent(ev('t1', 9, 'changed'));
    applyDiskStatusEvent(ev('t2', 1, 'missing'));
    expect(readDiskStatus('t2')?.state).toBe('missing');
  });

  test('a GetDiskStatus result marks or clears the tab', () => {
    setDiskStatusFromCheck('t1', { state: 'changed', loadedModTime: loaded, diskModTime: loaded + 1, diskSize: 10 }, diskStatusSeq('t1'));
    expect(readDiskStatus('t1')?.state).toBe('changed');
    setDiskStatusFromCheck('t1', { state: 'current', loadedModTime: loaded, diskModTime: loaded, diskSize: 10 }, diskStatusSeq('t1'));
    expect(readDiskStatus('t1')).toBeUndefined();
  });

  test('a watcher event after a check result still applies', () => {
    applyDiskStatusEvent(ev('t1', 3, 'changed'));
    setDiskStatusFromCheck('t1', { state: 'missing', loadedModTime: loaded, diskModTime: 0, diskSize: 0 }, diskStatusSeq('t1'));
    applyDiskStatusEvent(ev('t1', 4, 'current'));
    expect(readDiskStatus('t1')).toBeUndefined();
  });

  test('a check result older than a watcher event applied since the request is dropped', () => {
    applyDiskStatusEvent(ev('t1', 3, 'changed'));
    const seqAtRequest = diskStatusSeq('t1');
    applyDiskStatusEvent(ev('t1', 4, 'current'));

    const written = setDiskStatusFromCheck('t1', { state: 'changed', loadedModTime: loaded, diskModTime: loaded + 1, diskSize: 10 }, seqAtRequest);

    expect(written).toBe(false);
    expect(readDiskStatus('t1')).toBeUndefined();
  });

  test('a check result for a tab that left the tab list is dropped', () => {
    retainDiskStatus(['t1', 't2']);
    retainDiskStatus(['t2']);

    const written = setDiskStatusFromCheck('t1', { state: 'changed', loadedModTime: loaded, diskModTime: loaded + 1, diskSize: 10 }, 0);

    expect(written).toBe(false);
    expect(readDiskStatus('t1')).toBeUndefined();
  });

  test('retain drops entries of tabs that left the tab list', () => {
    applyDiskStatusEvent(ev('t1', 1, 'changed'));
    applyDiskStatusEvent(ev('t2', 1, 'changed'));
    retainDiskStatus(['t2']);
    expect(readDiskStatus('t1')).toBeUndefined();
    expect(readDiskStatus('t2')?.state).toBe('changed');
  });

  test('useDiskStatus re-renders only for its own tab', () => {
    let rendersT1 = 0;
    const { result } = renderHook(() => {
      rendersT1 += 1;
      return useDiskStatus('t1');
    });
    expect(result.current).toBeUndefined();
    const before = rendersT1;

    act(() => applyDiskStatusEvent(ev('t2', 1, 'changed')));
    expect(rendersT1).toBe(before);

    act(() => applyDiskStatusEvent(ev('t1', 1, 'changed')));
    expect(result.current?.state).toBe('changed');
    expect(rendersT1).toBe(before + 1);
  });

  test('entries are replaced, never mutated', () => {
    applyDiskStatusEvent(ev('t1', 1, 'changed', 100));
    const first = readDiskStatus('t1');
    applyDiskStatusEvent(ev('t1', 2, 'changed', 200));
    expect(first?.diskSize).toBe(100);
    expect(readDiskStatus('t1')).not.toBe(first);
  });
});
