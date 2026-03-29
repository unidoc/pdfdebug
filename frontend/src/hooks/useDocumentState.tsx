import { createContext, useContext, useReducer, type Dispatch, type ReactNode } from 'react';

// --- Types ---

export interface TabState {
  tabId: string;
  fileName: string;
}

export interface AppState {
  tabs: TabState[];
  activeTabId: string | null;
}

export type AppAction =
  | { type: 'OPEN_DOCUMENT'; payload: { tabId: string; fileName: string } }
  | { type: 'CLOSE_DOCUMENT'; payload: { tabId: string } };

// --- Reducer ---

const initialState: AppState = {
  tabs: [],
  activeTabId: null,
};

function appReducer(state: AppState, _action: AppAction): AppState {
  // Stub reducer -- returns current state for all actions in this story
  return state;
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
