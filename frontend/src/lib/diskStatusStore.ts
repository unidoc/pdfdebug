/**
 * @file Per-tab on-disk status, kept outside AppState so a status update
 * re-renders only the subscribers of that tab. Fed by the watcher's
 * `document:disk-status` events and by GetDiskStatus results.
 */
import { useCallback, useSyncExternalStore } from 'react';

/** On-disk state of a tab's file; `current` entries are never stored. */
export type DiskState = 'current' | 'changed' | 'missing';

/** Disk status as reported by GetDiskStatus. Times are Unix milliseconds. */
export interface DiskStatusInfo {
  state: string;
  loadedModTime: number;
  diskModTime: number;
  diskSize: number;
}

/** A `document:disk-status` event payload. */
export interface DiskStatusEvent extends DiskStatusInfo {
  tabId: string;
  seq: number;
}

/** Stored entry for a tab whose file is changed or missing. */
export interface DiskStatusEntry {
  seq: number;
  state: DiskState;
  loadedModTime: number;
  diskModTime: number;
  diskSize: number;
}

type Listener = () => void;
const listeners = new Set<Listener>();
let entries = new Map<string, DiskStatusEntry>();
const lastSeq = new Map<string, number>();

function notify() {
  for (const l of listeners) l();
}

function subscribe(l: Listener): () => void {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

function write(tabId: string, seq: number, info: DiskStatusInfo) {
  const next = new Map(entries);
  if (info.state === 'changed' || info.state === 'missing') {
    next.set(tabId, {
      seq,
      state: info.state,
      loadedModTime: info.loadedModTime,
      diskModTime: info.diskModTime,
      diskSize: info.diskSize,
    });
  } else if (!next.delete(tabId)) {
    return;
  }
  entries = next;
  notify();
}

/**
 * Applies a watcher event and returns whether it was applied. An event whose
 * `seq` is not above the last one applied for its tab is dropped; a `current`
 * event removes the entry.
 */
export function applyDiskStatusEvent(ev: DiskStatusEvent): boolean {
  const last = lastSeq.get(ev.tabId);
  if (last !== undefined && ev.seq <= last) return false;
  lastSeq.set(ev.tabId, ev.seq);
  write(ev.tabId, ev.seq, ev);
  return true;
}

/** Writes a GetDiskStatus result for tabId; `current` removes the entry. */
export function setDiskStatusFromCheck(tabId: string, info: DiskStatusInfo): void {
  write(tabId, lastSeq.get(tabId) ?? 0, info);
}

/** Drops entries (and seq history) of every tab not in tabIds. */
export function retainDiskStatus(tabIds: readonly string[]): void {
  const keep = new Set(tabIds);
  for (const id of lastSeq.keys()) {
    if (!keep.has(id)) lastSeq.delete(id);
  }
  let changed = false;
  const next = new Map(entries);
  for (const id of entries.keys()) {
    if (!keep.has(id)) {
      next.delete(id);
      changed = true;
    }
  }
  if (!changed) return;
  entries = next;
  notify();
}

/** Returns the stored entry for tabId, or undefined when its file is current. */
export function readDiskStatus(tabId: string): DiskStatusEntry | undefined {
  return entries.get(tabId);
}

/** Subscribes to tabId's entry; re-renders only when that entry changes. */
export function useDiskStatus(tabId: string | null | undefined): DiskStatusEntry | undefined {
  const get = useCallback(() => (tabId ? entries.get(tabId) : undefined), [tabId]);
  return useSyncExternalStore(subscribe, get, get);
}

/** Empties the store. */
export function resetDiskStatusStore(): void {
  entries = new Map();
  lastSeq.clear();
  notify();
}
