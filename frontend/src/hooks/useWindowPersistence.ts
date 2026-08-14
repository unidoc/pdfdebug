/**
 * Hook for saving and restoring panel sizes and window geometry via window.localStorage.
 *
 * Persists tree width (horizontal split), tree pane height, and sub-panel
 * height (vertical split), plus window geometry (x, y, width, height), under a
 * single STORAGE_KEY. Both fields share a 500ms debounce timer; the flush
 * reads the current localStorage state and merges, so a save of one field
 * never erases the other.
 *
 * The debounce timer and pending-write buffers live at module scope so that
 * multiple consumers (e.g. App.jsx for window geometry, MainLayout.tsx for
 * panel sizes) coalesce into a single write. If the refs were
 * per-instance, each consumer would maintain its own timer and a panel save
 * + a geometry save within 500ms would produce two writes.
 */
import { useState, useCallback, useEffect } from 'react';

const STORAGE_KEY = 'unidoc-pdf-debugger:window-state';

/** Shape stored in window.localStorage. */
interface PersistedWindowState {
  panelSizes?: {
    treeWidth: number;
    subPanelHeight: number;
    treePaneHeight?: number;
  };
  windowGeometry?: {
    x: number;
    y: number;
    width: number;
    height: number;
  };
}

/** Panel sizes returned by the hook. */
export interface PanelSizes {
  treeWidth: number;
  subPanelHeight: number;
  treePaneHeight?: number;
}

/** Window geometry (absolute position and size in screen pixels). */
export interface WindowGeometry {
  x: number;
  y: number;
  width: number;
  height: number;
}

/** Validate that a value is a finite positive number. */
function isValidSize(v: unknown): v is number {
  return typeof v === 'number' && Number.isFinite(v) && v > 0;
}

/** Validate that a value is any finite number (negative allowed for multi-monitor x/y). */
function isFiniteNumber(v: unknown): v is number {
  return typeof v === 'number' && Number.isFinite(v);
}

/**
 * Validate a window geometry object: width and height must be finite positive,
 * x and y must be finite (negative allowed for multi-monitor setups).
 */
export function isValidGeometry(g: unknown): g is WindowGeometry {
  if (typeof g !== 'object' || g === null) return false;
  const { x, y, width, height } = g as Record<string, unknown>;
  return isFiniteNumber(x) && isFiniteNumber(y) && isValidSize(width) && isValidSize(height);
}

/** Validated panelSizes loaded from a parsed PersistedWindowState, or null on shape mismatch. */
function loadPanelSizes(state: PersistedWindowState): PanelSizes | null {
  if (!state.panelSizes) return null;
  const { treeWidth, subPanelHeight, treePaneHeight } = state.panelSizes;
  if (!isValidSize(treeWidth) || !isValidSize(subPanelHeight)) return null;
  const result: PanelSizes = { treeWidth, subPanelHeight };
  if (isValidSize(treePaneHeight)) {
    result.treePaneHeight = treePaneHeight;
  }
  return result;
}

/** Validated windowGeometry loaded from a parsed PersistedWindowState, or null on shape mismatch. */
function loadWindowGeometry(state: PersistedWindowState): WindowGeometry | null {
  if (!state.windowGeometry) return null;
  return isValidGeometry(state.windowGeometry) ? state.windowGeometry : null;
}

/**
 * Read and validate persisted state from window.localStorage. Each field
 * (panelSizes, windowGeometry) is validated independently so a corrupt
 * value in one does not invalidate the other.
 */
function loadPersistedState(): { panelSizes: PanelSizes | null; windowGeometry: WindowGeometry | null } {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (raw == null) return { panelSizes: null, windowGeometry: null };

    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== 'object' || parsed == null) {
      return { panelSizes: null, windowGeometry: null };
    }

    const state = parsed as PersistedWindowState;
    return {
      panelSizes: loadPanelSizes(state),
      windowGeometry: loadWindowGeometry(state),
    };
  } catch {
    return { panelSizes: null, windowGeometry: null };
  }
}

// Module-scoped shared state. All hook instances share one timer + pending
// buffers so the single-write coalescing holds across multiple consumers
// (App.jsx + MainLayout.tsx).
let sharedTimer: ReturnType<typeof setTimeout> | null = null;
let pendingPanelSizes: PanelSizes | null = null;
let pendingGeometry: WindowGeometry | null = null;
// Reference count: incremented on each hook mount, decremented on unmount.
// The shared timer is only cleared when the count reaches zero so a stale
// unmount of one consumer cannot cancel writes pending for another.
let activeHookCount = 0;

// Read current localStorage, merge pending fields, write back. Pending
// buffers are cleared after the write so subsequent saves start fresh.
function flush(): void {
  let existing: PersistedWindowState = {};
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === 'object') {
        existing = parsed as PersistedWindowState;
      }
    }
  } catch {
    // Corrupt JSON -- treat as empty.
  }

  const next: PersistedWindowState = {};
  const panel = pendingPanelSizes ?? existing.panelSizes;
  const geometry = pendingGeometry ?? existing.windowGeometry;
  if (panel) next.panelSizes = panel;
  if (geometry) next.windowGeometry = geometry;

  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  } catch {
    // localStorage may be full or unavailable -- silently skip.
  }
  pendingPanelSizes = null;
  pendingGeometry = null;
  sharedTimer = null;
}

function scheduleFlush(): void {
  if (sharedTimer != null) {
    clearTimeout(sharedTimer);
  }
  sharedTimer = setTimeout(flush, 500);
}

/**
 * Hook that manages panel size and window geometry persistence via
 * window.localStorage. Returns the loaded values plus debounced save
 * functions. The two save functions share one debounce timer (at module
 * scope) so an alternating sequence -- including across multiple hook
 * consumers -- coalesces into a single write 500ms after the last call.
 * The flush reads localStorage and merges so neither save erases the
 * other field.
 */
export function useWindowPersistence() {
  const [initial] = useState(() => loadPersistedState());
  const [panelSizes] = useState<PanelSizes | null>(initial.panelSizes);
  const [windowGeometry] = useState<WindowGeometry | null>(initial.windowGeometry);

  // Reference-count active consumers; only cancel the shared timer when
  // the last consumer unmounts (otherwise a transient unmount of one
  // consumer would discard pending writes for the other).
  useEffect(() => {
    activeHookCount += 1;
    return () => {
      activeHookCount -= 1;
      if (activeHookCount <= 0) {
        activeHookCount = 0;
        if (pendingPanelSizes !== null || pendingGeometry !== null) {
          // Last consumer is leaving with un-debounced writes (#9). flush()
          // persists them synchronously and already nulls both pending buffers
          // and sharedTimer, so it SUBSUMES the clearTimeout/null-out below --
          // do not run both.
          flush();
        } else if (sharedTimer != null) {
          // No pending writes; a timer may still be armed from a flushed-then-
          // rescheduled state. Cancel it.
          clearTimeout(sharedTimer);
          sharedTimer = null;
        }
      }
    };
  }, []);

  const savePanelSizes = useCallback((sizes: PanelSizes) => {
    pendingPanelSizes = sizes;
    scheduleFlush();
  }, []);

  const saveWindowGeometry = useCallback((geometry: WindowGeometry) => {
    pendingGeometry = geometry;
    scheduleFlush();
  }, []);

  return { panelSizes, savePanelSizes, windowGeometry, saveWindowGeometry };
}
