/**
 * @file Global application state via React context + useReducer.
 * Manages document tabs, node selection, navigation, and error banners.
 */
import { createContext, useContext, useReducer, type Dispatch, type ReactNode } from 'react';

// --- Types ---

/** A node in the PDF object tree returned by the backend. */
export interface TreeNode {
  id: string;
  label: string;
  rawKey: string;
  nodeType: string;
  valueType: string;
  hasChildren: boolean;
  childCount: number;
  iconHint: string;
  error: string;
}

/** Entry in the navigation history stack. */
export interface NavHistoryEntry {
  nodeId: string;
  label: string | null;
  rawKey: string | null;
  iconHint?: string;
}

/** Per-document tab state including tree root, selection, and navigation. */
export interface TabState {
  tabId: string;
  fileName: string;
  filePath: string;
  pageCount: number;
  rootNode: TreeNode | null;
  rootChildren: TreeNode[] | null;
  selectedNodeId: string | null;
  selectedNodeLabel: string | null;
  selectedNodeRawKey: string | null;
  selectedNodeIconHint: string | null;
  pendingNavTarget: string | null;
  navError: string | null;
  navHistory: NavHistoryEntry[];
  navHistoryIndex: number;
}

/** Top-level application state. */
export interface AppState {
  tabs: TabState[];
  activeTabId: string | null;
  documentError: string | null;
  documentWarning: string | null;
  goToPageOpen: boolean;
  // Batch open progress: when batchOpenTotal > 0 a multi-file drop is in
  // flight; the BatchOpenDialog is visible and disappears when total resets
  // to 0 (BATCH_OPEN_COMPLETE). Single-file drops never set these.
  batchOpenTotal: number;
  batchOpenCompleted: number;
}

/** Union of all actions the app reducer handles. */
export type AppAction =
  | { type: 'OPEN_DOCUMENT'; payload: { tabId: string; fileName: string; filePath: string; pageCount?: number; rootNode: TreeNode | null; rootChildren: TreeNode[] | null } }
  | { type: 'ACTIVATE_TAB'; payload: { tabId: string } }
  | { type: 'CLOSE_DOCUMENT'; payload: { tabId: string } }
  | { type: 'SELECT_NODE'; payload: { nodeId: string; label?: string; rawKey?: string; iconHint?: string; isHistoryNav?: boolean } }
  | { type: 'SET_DOCUMENT_ERROR'; payload: { message: string } }
  | { type: 'DISMISS_ERROR' }
  | { type: 'NAVIGATE_TO_REF'; payload: { targetNodeId: string } }
  | { type: 'CLEAR_NAV_TARGET' }
  | { type: 'NAV_ERROR'; payload: { message: string } }
  | { type: 'DISMISS_NAV_ERROR' }
  | { type: 'SET_DOCUMENT_WARNING'; payload: { message: string } }
  | { type: 'DISMISS_WARNING' }
  | { type: 'NAVIGATE_BACK' }
  | { type: 'NAVIGATE_FORWARD' }
  | { type: 'OPEN_GO_TO_PAGE' }
  | { type: 'CLOSE_GO_TO_PAGE' }
  | { type: 'BATCH_OPEN_START'; payload: { total: number } }
  | { type: 'BATCH_OPEN_PROGRESS'; payload: { completed: number } }
  | { type: 'BATCH_OPEN_COMPLETE' };

// --- Reducer ---

const initialState: AppState = {
  tabs: [],
  activeTabId: null,
  documentError: null,
  documentWarning: null,
  goToPageOpen: false,
  batchOpenTotal: 0,
  batchOpenCompleted: 0,
};

/**
 * Pure reducer for all app state transitions.
 * Exhaustive switch ensures every action is handled at compile time.
 */
function appReducer(state: AppState, action: AppAction): AppState {
  switch (action.type) {
    case 'OPEN_DOCUMENT': {
      // Duplicate file detection: if a tab with the same filePath exists, activate it.
      // Backend resource cleanup for the discarded tabId is handled in App.jsx.
      if (action.payload.filePath) {
        const existing = state.tabs.find((t) => t.filePath === action.payload.filePath);
        if (existing) {
          return {
            ...state,
            activeTabId: existing.tabId,
            documentError: null,
            documentWarning: null,
          };
        }
      }
      const newTab: TabState = {
        tabId: action.payload.tabId,
        fileName: action.payload.fileName,
        filePath: action.payload.filePath,
        pageCount: action.payload.pageCount ?? 0,
        rootNode: action.payload.rootNode,
        rootChildren: action.payload.rootChildren,
        selectedNodeId: null,
        selectedNodeLabel: null,
        selectedNodeRawKey: null,
        selectedNodeIconHint: null,
        pendingNavTarget: null,
        navError: null,
        navHistory: [],
        navHistoryIndex: -1,
      };
      return {
        ...state,
        tabs: [...state.tabs, newTab],
        activeTabId: action.payload.tabId,
        documentError: null,
        documentWarning: null,
      };
    }
    case 'CLOSE_DOCUMENT': {
      const closingActive = state.activeTabId === action.payload.tabId;
      const closedIndex = state.tabs.findIndex((t) => t.tabId === action.payload.tabId);
      const filtered = state.tabs.filter((t) => t.tabId !== action.payload.tabId);
      let nextActiveId = state.activeTabId;
      if (closingActive) {
        if (filtered.length === 0) {
          nextActiveId = null;
        } else {
          // Prefer the tab that was to the right; fall back to the one to the left
          const adjacentIndex = Math.min(closedIndex, filtered.length - 1);
          nextActiveId = filtered[adjacentIndex].tabId;
        }
      }
      return {
        ...state,
        tabs: filtered,
        activeTabId: nextActiveId,
        documentError: closingActive ? null : state.documentError,
        documentWarning: closingActive ? null : state.documentWarning,
      };
    }
    case 'ACTIVATE_TAB': {
      const tabExists = state.tabs.some((t) => t.tabId === action.payload.tabId);
      if (!tabExists) return state;
      return { ...state, activeTabId: action.payload.tabId };
    }
    case 'SELECT_NODE': {
      if (state.activeTabId === null) return state;
      if (!action.payload.nodeId) return state;
      return {
        ...state,
        tabs: state.tabs.map((tab) => {
          if (tab.tabId !== state.activeTabId) return tab;
          const entry: NavHistoryEntry = {
            nodeId: action.payload.nodeId,
            label: action.payload.label ?? null,
            rawKey: action.payload.rawKey ?? null,
            iconHint: action.payload.iconHint,
          };
          // History navigation: just update selection, don't push
          if (action.payload.isHistoryNav) {
            return {
              ...tab,
              selectedNodeId: entry.nodeId,
              selectedNodeLabel: entry.label,
              selectedNodeRawKey: entry.rawKey,
              selectedNodeIconHint: action.payload.iconHint ?? null,
            };
          }
          // Skip duplicate if same node is already current
          const current = tab.navHistoryIndex >= 0 ? tab.navHistory[tab.navHistoryIndex] : null;
          if (current && current.nodeId === entry.nodeId) {
            return {
              ...tab,
              selectedNodeId: entry.nodeId,
              selectedNodeLabel: entry.label,
              selectedNodeRawKey: entry.rawKey,
              selectedNodeIconHint: action.payload.iconHint ?? null,
            };
          }
          // Normal navigation: truncate forward history, push new entry
          const truncated = tab.navHistory.slice(0, tab.navHistoryIndex + 1);
          return {
            ...tab,
            selectedNodeId: entry.nodeId,
            selectedNodeLabel: entry.label,
            selectedNodeRawKey: entry.rawKey,
            selectedNodeIconHint: action.payload.iconHint ?? null,
            navHistory: [...truncated, entry],
            navHistoryIndex: truncated.length,
          };
        }),
      };
    }
    case 'SET_DOCUMENT_ERROR': {
      // In multi-tab mode, preserve existing tabs -- the error banner is
      // informational about the failed open attempt, not a reason to close
      // every open document.
      return {
        ...state,
        documentError: action.payload.message,
        documentWarning: null,
      };
    }
    case 'DISMISS_ERROR': {
      return {
        ...state,
        documentError: null,
      };
    }
    case 'NAVIGATE_TO_REF': {
      if (state.activeTabId === null) return state;
      return {
        ...state,
        tabs: state.tabs.map((tab) =>
          tab.tabId === state.activeTabId
            ? { ...tab, pendingNavTarget: action.payload.targetNodeId, navError: null }
            : tab
        ),
      };
    }
    case 'CLEAR_NAV_TARGET': {
      if (state.activeTabId === null) return state;
      return {
        ...state,
        tabs: state.tabs.map((tab) =>
          tab.tabId === state.activeTabId
            ? { ...tab, pendingNavTarget: null }
            : tab
        ),
      };
    }
    case 'NAV_ERROR': {
      if (state.activeTabId === null) return state;
      return {
        ...state,
        tabs: state.tabs.map((tab) =>
          tab.tabId === state.activeTabId
            ? { ...tab, navError: action.payload.message, pendingNavTarget: null }
            : tab
        ),
      };
    }
    case 'DISMISS_NAV_ERROR': {
      if (state.activeTabId === null) return state;
      return {
        ...state,
        tabs: state.tabs.map((tab) =>
          tab.tabId === state.activeTabId
            ? { ...tab, navError: null }
            : tab
        ),
      };
    }
    case 'SET_DOCUMENT_WARNING': {
      return { ...state, documentWarning: action.payload.message };
    }
    case 'DISMISS_WARNING': {
      return { ...state, documentWarning: null };
    }
    case 'NAVIGATE_BACK': {
      if (state.activeTabId === null) return state;
      return {
        ...state,
        tabs: state.tabs.map((tab) => {
          if (tab.tabId !== state.activeTabId) return tab;
          if (tab.navHistoryIndex <= 0) return tab;
          const newIndex = tab.navHistoryIndex - 1;
          const entry = tab.navHistory[newIndex];
          return {
            ...tab,
            navHistoryIndex: newIndex,
            selectedNodeId: entry.nodeId,
            selectedNodeLabel: entry.label,
            selectedNodeRawKey: entry.rawKey,
            selectedNodeIconHint: entry.iconHint ?? null,
          };
        }),
      };
    }
    case 'NAVIGATE_FORWARD': {
      if (state.activeTabId === null) return state;
      return {
        ...state,
        tabs: state.tabs.map((tab) => {
          if (tab.tabId !== state.activeTabId) return tab;
          if (tab.navHistoryIndex >= tab.navHistory.length - 1) return tab;
          const newIndex = tab.navHistoryIndex + 1;
          const entry = tab.navHistory[newIndex];
          return {
            ...tab,
            navHistoryIndex: newIndex,
            selectedNodeId: entry.nodeId,
            selectedNodeLabel: entry.label,
            selectedNodeRawKey: entry.rawKey,
            selectedNodeIconHint: entry.iconHint ?? null,
          };
        }),
      };
    }
    case 'OPEN_GO_TO_PAGE': {
      // No-op when no document is loaded; the dialog needs an active tab and
      // a positive pageCount to be useful.
      if (state.activeTabId === null) return state;
      const active = state.tabs.find((t) => t.tabId === state.activeTabId);
      if (!active || active.pageCount <= 0) return state;
      return { ...state, goToPageOpen: true };
    }
    case 'CLOSE_GO_TO_PAGE': {
      return { ...state, goToPageOpen: false };
    }
    case 'BATCH_OPEN_START': {
      return { ...state, batchOpenTotal: action.payload.total, batchOpenCompleted: 0 };
    }
    case 'BATCH_OPEN_PROGRESS': {
      return { ...state, batchOpenCompleted: action.payload.completed };
    }
    case 'BATCH_OPEN_COMPLETE': {
      return { ...state, batchOpenTotal: 0, batchOpenCompleted: 0 };
    }
    default: {
      const _exhaustive: never = action;
      return state;
    }
  }
}

// --- Contexts ---

const AppStateContext = createContext<AppState | null>(null);
const AppDispatchContext = createContext<Dispatch<AppAction> | null>(null);

// --- Provider ---

/** Context provider that makes app state and dispatch available to the tree. */
export function AppProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(appReducer, initialState);

  return (
    <AppStateContext.Provider value={state}>
      <AppDispatchContext.Provider value={dispatch}>
        {children}
      </AppDispatchContext.Provider>
    </AppStateContext.Provider>
  );
}

// --- Hooks ---

/** Read the current app state. Must be called inside AppProvider. */
export function useAppState(): AppState {
  const context = useContext(AppStateContext);
  if (context === null) {
    throw new Error('useAppState must be used within an AppProvider');
  }
  return context;
}

/** Get the dispatch function for app actions. Must be called inside AppProvider. */
export function useAppDispatch(): Dispatch<AppAction> {
  const context = useContext(AppDispatchContext);
  if (context === null) {
    throw new Error('useAppDispatch must be used within an AppProvider');
  }
  return context;
}
