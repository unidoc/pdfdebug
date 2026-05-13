/**
 * @file useObjectIndex -- per-tab cache of the backend ObjectIndex.
 * Lazy-fetched on first access; the backend already caches per DocumentState,
 * so re-fetching here is a no-op IPC call. The frontend cache exists so the
 * palette can render the index synchronously after the first open per tab.
 */
import { useCallback, useEffect, useRef, useState } from 'react';
import { GetObjectIndex } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import type { ObjectIndexEntry } from '../types/palette';
import { useAppState } from './useDocumentState';

/**
 * Returns the cached object index for the active tab and a getter that
 * triggers a lazy fetch on first call per tab. Drops cache entries when
 * tabs close (the AppState.tabs list shrinks).
 */
export function useObjectIndex() {
  const { tabs, activeTabId } = useAppState();
  const cacheRef = useRef<Map<string, ObjectIndexEntry[]>>(new Map());
  const inflightRef = useRef<Map<string, Promise<ObjectIndexEntry[]>>>(new Map());
  const [, setVersion] = useState(0); // force re-render after async fetch

  // Live tab IDs mirrored into a ref so async fetch resolution can re-check
  // whether the tab still exists before populating the cache. Refs (not state)
  // because the check must see the latest value at await resolution, not the
  // value captured when ensureIndex was first called.
  const liveTabIdsRef = useRef<Set<string>>(new Set());

  // Drop cache entries for tabs that are no longer open.
  const tabIdsKey = tabs.map((t) => t.tabId).join(',');
  useEffect(() => {
    const liveIds = new Set(tabs.map((t) => t.tabId));
    liveTabIdsRef.current = liveIds;
    for (const id of Array.from(cacheRef.current.keys())) {
      if (!liveIds.has(id)) cacheRef.current.delete(id);
    }
    for (const id of Array.from(inflightRef.current.keys())) {
      if (!liveIds.has(id)) inflightRef.current.delete(id);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tabIdsKey]);

  /** Fetch (lazy) and return the index for the given tab. */
  const ensureIndex = useCallback(async (tabId: string): Promise<ObjectIndexEntry[]> => {
    const cached = cacheRef.current.get(tabId);
    if (cached) return cached;
    const inflight = inflightRef.current.get(tabId);
    if (inflight) return inflight;
    const p = (async () => {
      try {
        const raw = await GetObjectIndex(tabId);
        const entries: ObjectIndexEntry[] = (raw ?? [])
          .filter((e): e is NonNullable<typeof e> => e !== null)
          .map((e) => ({
            objNum: e.objNum,
            gen: e.gen,
            typeName: e.typeName,
            free: e.free,
            reachable: e.reachable,
            nodeId: e.nodeId,
          }));
        // Guard: if the tab was closed mid-fetch, the cleanup effect already
        // ran and deleted any entry for tabId. Re-inserting here would leak
        // the entry until app shutdown. Drop the result instead.
        if (!liveTabIdsRef.current.has(tabId)) return entries;
        cacheRef.current.set(tabId, entries);
        setVersion((v) => v + 1);
        return entries;
      } finally {
        // Clear inflight on both success and failure so a transient backend
        // error doesn't poison the cache: the next ensureIndex(tabId) call
        // gets to retry from scratch instead of inheriting a rejected promise.
        inflightRef.current.delete(tabId);
      }
    })();
    inflightRef.current.set(tabId, p);
    return p;
  }, []);

  const currentIndex = activeTabId ? cacheRef.current.get(activeTabId) ?? null : null;

  return { index: currentIndex, ensureIndex };
}
