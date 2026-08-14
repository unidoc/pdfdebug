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
  /**
   * "<num> <gen> R" for indirect objects; "" for inline scalars/arrays/dicts
   * without an indirect identity. Renders as the [N G R] suffix on tree rows
   * (Story 9-8).
   */
  objectRef: string;
  /**
   * Literal /Type value of the resolved dict (e.g. "Pages", "Page", "Font");
   * "" when absent. Frontend dedups this against the semantic label before
   * rendering /T:<TypeName>.
   */
  typeName: string;
}

/** Entry in the navigation history stack. */
export interface NavHistoryEntry {
  nodeId: string;
  label: string | null;
  rawKey: string | null;
  iconHint?: string;
}

/**
 * One recent palette jump. Mirrors ObjectIndexEntry fields the palette needs
 * to re-render the row without re-querying the backend.
 */
export interface RecentJump {
  objNum: number;
  gen: number;
  typeName: string;
  nodeId: string;
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
  /**
   * Most-recent palette jumps for this tab, newest first. LRU capped at 5;
   * Deduped by nodeId. Lives in memory only -- dies with the tab on
   * CLOSE_DOCUMENT.
   */
  recentJumps: RecentJump[];
  /**
   * Plain Text find-bar case-sensitivity toggle. Persists across find-bar
   * reopen on the same tab and dies with the tab on CLOSE_DOCUMENT (Story
   * 10-2).
   */
  findCaseSensitive: boolean;
}

/** Top-level application state. */
export interface AppState {
  tabs: TabState[];
  activeTabId: string | null;
  documentError: string | null;
  documentWarning: string | null;
  goToPageOpen: boolean;
  // Dialog visibility for an in-flight multi-file open. Set true on
  // BATCH_OPEN_START, false on BATCH_OPEN_COMPLETE.
  batchOpenActive: boolean;
  // Total/completed counts for the batch. Kept alive past COMPLETE while
  // batchOpenCancelled is true so late OPEN_DOCUMENT events (Wails alpha.85
  // dispatches each Emit in its own goroutine, so events can arrive after
  // batch-complete) can update the cancellation toast count. Reset on the
  // next BATCH_OPEN_START or on DISMISS_WARNING.
  batchOpenTotal: number;
  batchOpenCompleted: number;
  // True after the user clicks Cancel. Persists past BATCH_OPEN_COMPLETE
  // until BATCH_OPEN_START or DISMISS_WARNING.
  batchOpenCancelled: boolean;
  /**
   * Monotonic counter incremented on every ACTIVATE_TAB dispatch (even when
   * the target is the already-active tab). The Cmd+K palette subscribes to
   * this so any user-initiated tab activation closes the palette.
   */
  tabActivationVersion: number;
  // True while a single-file open is in flight. Drives the inline loading
  // state in EmptyState so the user gets immediate feedback on large PDFs
  // (where pdfcpu's xref walk can take a second or more).
  isOpening: boolean;
  openingFileName: string | null;
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
  | { type: 'BATCH_OPEN_CANCEL' }
  | { type: 'BATCH_OPEN_COMPLETE' }
  | { type: 'OPENING_START'; payload: { fileName: string } }
  | { type: 'PUSH_RECENT_JUMP'; payload: { tabId: string; entry: RecentJump } }
  | { type: 'SET_FIND_CASE_SENSITIVE'; payload: { tabId: string; value: boolean } };

// --- Reducer ---

const initialState: AppState = {
  tabs: [],
  activeTabId: null,
  documentError: null,
  documentWarning: null,
  goToPageOpen: false,
  batchOpenActive: false,
  batchOpenTotal: 0,
  batchOpenCompleted: 0,
  batchOpenCancelled: false,
  tabActivationVersion: 0,
  isOpening: false,
  openingFileName: null,
};

/**
 * Pure reducer for all app state transitions.
 * Exhaustive switch ensures every action is handled at compile time.
 */
function appReducer(state: AppState, action: AppAction): AppState {
  switch (action.type) {
    case 'OPEN_DOCUMENT': {
      // Drive batch progress from OPEN_DOCUMENT itself: incrementing here is
      // atomic with the tab being added, so the count can never lag behind
      // the actual number of opened tabs.
      const inBatch = state.batchOpenTotal > 0;
      const batchOpenCompleted = inBatch
        ? state.batchOpenCompleted + 1
        : state.batchOpenCompleted;
      // Effectively-no-op cancel: if late drain events end up landing every
      // file in the batch anyway, drop the cancel toast and flag so the user
      // does not see a misleading "cancelled" notification for a fully-loaded
      // batch.
      const cancelDidNothing = state.batchOpenCancelled
        && inBatch
        && batchOpenCompleted >= state.batchOpenTotal;
      // Refresh the cancellation toast each time a file lands so late events
      // (after BATCH_OPEN_COMPLETE, due to Wails' goroutine race) update the
      // displayed count. When not cancelled, OPEN_DOCUMENT clears the warning
      // as before.
      const documentWarning = cancelDidNothing
        ? null
        : state.batchOpenCancelled
          ? (inBatch
            ? `Loading cancelled. ${batchOpenCompleted} of ${state.batchOpenTotal} files opened.`
            : state.documentWarning)
          : null;
      const batchOpenCancelled = cancelDidNothing ? false : state.batchOpenCancelled;
      // Duplicate file detection: if a tab with the same filePath exists, activate it.
      // Backend resource cleanup for the discarded tabId is handled in App.jsx.
      if (action.payload.filePath) {
        const existing = state.tabs.find((t) => t.filePath === action.payload.filePath);
        if (existing) {
          // Dedup re-activates an existing tab. Bump the activation counter
          // only when the resolved tab differs from the current active one,
          // so subscribers (e.g. Cmd+K palette) treat it as a tab switch.
          const activatedDifferentTab = state.activeTabId !== existing.tabId;
          return {
            ...state,
            activeTabId: existing.tabId,
            documentError: null,
            documentWarning,
            batchOpenCompleted,
            batchOpenCancelled,
            tabActivationVersion: activatedDifferentTab
              ? state.tabActivationVersion + 1
              : state.tabActivationVersion,
            isOpening: false,
            openingFileName: null,
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
        recentJumps: [],
        findCaseSensitive: false,
      };
      // Opening a new tab that becomes active is a tab-context change; bump
      // the version so the Cmd+K palette closes (mirrors ACTIVATE_TAB and the
      // CLOSE_DOCUMENT-of-active-tab path).
      const activatedNewTab = state.activeTabId !== action.payload.tabId;
      return {
        ...state,
        tabs: [...state.tabs, newTab],
        activeTabId: action.payload.tabId,
        documentError: null,
        documentWarning,
        batchOpenCompleted,
        batchOpenCancelled,
        tabActivationVersion: activatedNewTab
          ? state.tabActivationVersion + 1
          : state.tabActivationVersion,
        isOpening: false,
        openingFileName: null,
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
        // Closing the active tab is a tab-context change; bump the version
        // so subscribers (e.g. Cmd+K palette) treat it like an ACTIVATE_TAB.
        tabActivationVersion: closingActive
          ? state.tabActivationVersion + 1
          : state.tabActivationVersion,
      };
    }
    case 'ACTIVATE_TAB': {
      const tabExists = state.tabs.some((t) => t.tabId === action.payload.tabId);
      if (!tabExists) return state;
      return {
        ...state,
        activeTabId: action.payload.tabId,
        tabActivationVersion: state.tabActivationVersion + 1,
      };
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
        isOpening: false,
        openingFileName: null,
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
      // Suppress per-file warnings while the cancellation toast is active.
      if (state.batchOpenCancelled) return state;
      return { ...state, documentWarning: action.payload.message };
    }
    case 'DISMISS_WARNING': {
      // Also clear batch state so subsequent opens aren't suppressed and the
      // count info from the prior cancelled batch doesn't leak.
      return {
        ...state,
        documentWarning: null,
        batchOpenCancelled: false,
        batchOpenTotal: 0,
        batchOpenCompleted: 0,
      };
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
      return {
        ...state,
        batchOpenActive: true,
        batchOpenTotal: action.payload.total,
        batchOpenCompleted: 0,
        batchOpenCancelled: false,
      };
    }
    case 'BATCH_OPEN_CANCEL': {
      if (!state.batchOpenActive) return state;
      // Set toast at click time so the user sees feedback immediately.
      // OPEN_DOCUMENT regenerates the text as more files land, keeping the
      // count current.
      return {
        ...state,
        batchOpenCancelled: true,
        documentWarning: `Loading cancelled. ${state.batchOpenCompleted} of ${state.batchOpenTotal} files opened.`,
      };
    }
    case 'BATCH_OPEN_COMPLETE': {
      // Close the dialog but keep total/completed and the cancelled flag
      // alive when cancelled, so late OPEN_DOCUMENT events (Wails goroutine
      // race) can still update the toast count. They reset on the next
      // BATCH_OPEN_START or on DISMISS_WARNING.
      if (state.batchOpenCancelled) {
        return { ...state, batchOpenActive: false, isOpening: false, openingFileName: null };
      }
      return {
        ...state,
        batchOpenActive: false,
        batchOpenTotal: 0,
        batchOpenCompleted: 0,
        isOpening: false,
        openingFileName: null,
      };
    }
    case 'OPENING_START': {
      return {
        ...state,
        isOpening: true,
        openingFileName: action.payload.fileName,
      };
    }
    case 'PUSH_RECENT_JUMP': {
      // Per-tab LRU: dedup by nodeId, push to front, cap at 5. Newest first.
      const { tabId, entry } = action.payload;
      return {
        ...state,
        tabs: state.tabs.map((tab) => {
          if (tab.tabId !== tabId) return tab;
          const filtered = tab.recentJumps.filter((r) => r.nodeId !== entry.nodeId);
          const next = [entry, ...filtered].slice(0, 5);
          return { ...tab, recentJumps: next };
        }),
      };
    }
    case 'SET_FIND_CASE_SENSITIVE': {
      const { tabId, value } = action.payload;
      // No-op when the target tab is absent so unknown-tabId dispatches do
      // not corrupt other tabs.
      if (!state.tabs.some((t) => t.tabId === tabId)) return state;
      return {
        ...state,
        tabs: state.tabs.map((tab) =>
          tab.tabId === tabId ? { ...tab, findCaseSensitive: value } : tab,
        ),
      };
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
