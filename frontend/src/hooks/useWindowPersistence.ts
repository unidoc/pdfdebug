/**
 * Hook for saving and restoring panel sizes via window.localStorage.
 *
 * Persists tree width (horizontal split), tree pane height, and sub-panel
 * height (vertical split) with 500ms debounce to avoid excessive writes
 * during continuous resize.
 *
 * TODO: Window position/size (x, y, width, height) persistence is deferred --
 * Wails v3 alpha does not reliably expose window geometry to the frontend.
 */
import { useState, useCallback, useRef, useEffect } from 'react';

const STORAGE_KEY = 'unidoc-pdf-debugger:window-state';

/** Shape stored in window.localStorage. */
interface PersistedWindowState {
  panelSizes?: {
    treeWidth: number;
    subPanelHeight: number;
    treePaneHeight?: number;
  };
}

/** Panel sizes returned by the hook. */
export interface PanelSizes {
  treeWidth: number;
  subPanelHeight: number;
  treePaneHeight?: number;
}

/** Validate that a value is a finite positive number. */
function isValidSize(v: unknown): v is number {
  return typeof v === 'number' && Number.isFinite(v) && v > 0;
}

/** Read and validate persisted state from window.localStorage. Returns null on any failure. */
function loadPersistedState(): PanelSizes | null {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (raw == null) return null;

    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== 'object' || parsed == null) return null;

    const state = parsed as PersistedWindowState;
    if (!state.panelSizes) return null;

    const { treeWidth, subPanelHeight, treePaneHeight } = state.panelSizes;
    if (!isValidSize(treeWidth) || !isValidSize(subPanelHeight)) return null;

    const result: PanelSizes = { treeWidth, subPanelHeight };
    if (isValidSize(treePaneHeight)) {
      result.treePaneHeight = treePaneHeight;
    }
    return result;
  } catch {
    return null;
  }
}

/**
 * Hook that manages panel size persistence via window.localStorage.
 * Returns current panel sizes (or null) and a debounced save function.
 */
export function useWindowPersistence() {
  const [panelSizes] = useState<PanelSizes | null>(() => loadPersistedState());
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Cleanup debounce timer on unmount
  useEffect(() => {
    return () => {
      if (timerRef.current != null) {
        clearTimeout(timerRef.current);
      }
    };
  }, []);

  const savePanelSizes = useCallback((sizes: PanelSizes) => {
    if (timerRef.current != null) {
      clearTimeout(timerRef.current);
    }
    timerRef.current = setTimeout(() => {
      try {
        const state: PersistedWindowState = { panelSizes: sizes };
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
      } catch {
        // localStorage may be full or unavailable -- silently skip.
      }
    }, 500);
  }, []);

  return { panelSizes, savePanelSizes };
}
