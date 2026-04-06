import { createContext, useContext, useReducer, type Dispatch, type ReactNode } from 'react';

// --- Types ---

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

export interface TabState {
  tabId: string;
  fileName: string;
  rootNode: TreeNode | null;
  rootChildren: TreeNode[] | null;
  selectedNodeId: string | null;
  selectedNodeLabel: string | null;
  selectedNodeRawKey: string | null;
  pendingNavTarget: string | null;
  navError: string | null;
}

export interface AppState {
  tabs: TabState[];
  activeTabId: string | null;
  documentError: string | null;
  documentWarning: string | null;
}

export type AppAction =
  | { type: 'OPEN_DOCUMENT'; payload: { tabId: string; fileName: string; rootNode: TreeNode | null; rootChildren: TreeNode[] | null } }
  | { type: 'CLOSE_DOCUMENT'; payload: { tabId: string } }
  | { type: 'SELECT_NODE'; payload: { nodeId: string; label?: string; rawKey?: string } }
  | { type: 'SET_DOCUMENT_ERROR'; payload: { message: string } }
  | { type: 'DISMISS_ERROR' }
  | { type: 'NAVIGATE_TO_REF'; payload: { targetNodeId: string } }
  | { type: 'CLEAR_NAV_TARGET' }
  | { type: 'NAV_ERROR'; payload: { message: string } }
  | { type: 'DISMISS_NAV_ERROR' }
  | { type: 'SET_DOCUMENT_WARNING'; payload: { message: string } }
  | { type: 'DISMISS_WARNING' };

// --- Reducer ---

const initialState: AppState = {
  tabs: [],
  activeTabId: null,
  documentError: null,
  documentWarning: null,
};

function appReducer(state: AppState, action: AppAction): AppState {
  switch (action.type) {
    case 'OPEN_DOCUMENT': {
      const newTab: TabState = {
        tabId: action.payload.tabId,
        fileName: action.payload.fileName,
        rootNode: action.payload.rootNode,
        rootChildren: action.payload.rootChildren,
        selectedNodeId: null,
        selectedNodeLabel: null,
        selectedNodeRawKey: null,
        pendingNavTarget: null,
        navError: null,
      };
      // Single document at a time -- replace all tabs (multi-tab is Epic 4)
      return {
        ...state,
        tabs: [newTab],
        activeTabId: action.payload.tabId,
        documentError: null,
        documentWarning: null,
      };
    }
    case 'CLOSE_DOCUMENT': {
      const filtered = state.tabs.filter((t) => t.tabId !== action.payload.tabId);
      return {
        ...state,
        tabs: filtered,
        activeTabId: filtered.length > 0 ? filtered[filtered.length - 1].tabId : null,
        documentWarning: null,
      };
    }
    case 'SELECT_NODE': {
      if (state.activeTabId === null) return state;
      if (!action.payload.nodeId) return state;
      return {
        ...state,
        tabs: state.tabs.map((tab) =>
          tab.tabId === state.activeTabId
            ? { ...tab, selectedNodeId: action.payload.nodeId, selectedNodeLabel: action.payload.label ?? null, selectedNodeRawKey: action.payload.rawKey ?? null }
            : tab
        ),
      };
    }
    case 'SET_DOCUMENT_ERROR': {
      return {
        ...state,
        tabs: [],
        documentError: action.payload.message,
        documentWarning: null,
        activeTabId: null,
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

export function useAppState(): AppState {
  const context = useContext(AppStateContext);
  if (context === null) {
    throw new Error('useAppState must be used within an AppProvider');
  }
  return context;
}

export function useAppDispatch(): Dispatch<AppAction> {
  const context = useContext(AppDispatchContext);
  if (context === null) {
    throw new Error('useAppDispatch must be used within an AppProvider');
  }
  return context;
}
