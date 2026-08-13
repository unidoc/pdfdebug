/**
 * Story 10.2: Find Bar in Plain Text View -- reducer red-phase suite for
 * per-tab `findCaseSensitive` field + SET_FIND_CASE_SENSITIVE action.
 *
 * TDD RED PHASE: every test below fails until Task 1 of Story 10-2 is
 * implemented (TabState gains findCaseSensitive: boolean, AppAction gains
 * SET_FIND_CASE_SENSITIVE, OPEN_DOCUMENT defaults the field to false,
 * CLOSE_DOCUMENT drops it with the tab).
 *
 * Test IDs follow the convention.
 *
 * Run: cd frontend && npx vitest run src/hooks/useDocumentState.find.test.tsx
 */
import { render, screen, act } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import {
  AppProvider,
  useAppState,
  useAppDispatch,
  type AppAction,
  type TreeNode,
} from './useDocumentState';

const catalogNode: TreeNode = {
  id: 'root',
  label: 'Catalog',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 0,
  iconHint: 'catalog',
  error: '',
  objectRef: '',
  typeName: '',
};

function Inspector({ actions }: { actions: AppAction[] }) {
  const state = useAppState();
  const dispatch = useAppDispatch();
  const findCase = state.tabs[0]?.findCaseSensitive;
  return (
    <div>
      <span data-testid="tab-count">{state.tabs.length}</span>
      <span data-testid="tab-0-id">{state.tabs[0]?.tabId ?? 'null'}</span>
      <span data-testid="tab-0-find-case">{String(findCase)}</span>
      <span data-testid="tab-1-find-case">{String(state.tabs[1]?.findCaseSensitive)}</span>
      <button
        data-testid="dispatch"
        onClick={() => {
          actions.forEach((a) => dispatch(a));
        }}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// OPEN_DOCUMENT seeds findCaseSensitive=false on the new TabState.
// ---------------------------------------------------------------------------

describe('OPEN_DOCUMENT defaults findCaseSensitive to false', () => {
  test('a freshly opened tab has findCaseSensitive=false', () => {
    const actions: AppAction[] = [
      {
        type: 'OPEN_DOCUMENT',
        payload: {
          tabId: 'tab-1',
          fileName: 'a.pdf',
          filePath: '/tmp/a.pdf',
          rootNode: catalogNode,
          rootChildren: [],
        },
      },
    ];
    render(
      <AppProvider>
        <Inspector actions={actions} />
      </AppProvider>,
    );
    act(() => {
      screen.getByTestId('dispatch').click();
    });
    expect(screen.getByTestId('tab-count').textContent).toBe('1');
    expect(screen.getByTestId('tab-0-find-case').textContent).toBe('false');
  });
});

// ---------------------------------------------------------------------------
// SET_FIND_CASE_SENSITIVE flips the targeted tab's flag without touching
// other tabs.
// ---------------------------------------------------------------------------

describe('SET_FIND_CASE_SENSITIVE updates one tab', () => {
  test('dispatching flip on tab-1 updates only tab-1', () => {
    const actions: AppAction[] = [
      {
        type: 'OPEN_DOCUMENT',
        payload: {
          tabId: 'tab-1',
          fileName: 'a.pdf',
          filePath: '/tmp/a.pdf',
          rootNode: catalogNode,
          rootChildren: [],
        },
      },
      {
        type: 'OPEN_DOCUMENT',
        payload: {
          tabId: 'tab-2',
          fileName: 'b.pdf',
          filePath: '/tmp/b.pdf',
          rootNode: catalogNode,
          rootChildren: [],
        },
      },
      {
        type: 'SET_FIND_CASE_SENSITIVE',
        payload: { tabId: 'tab-1', value: true },
      },
    ];
    render(
      <AppProvider>
        <Inspector actions={actions} />
      </AppProvider>,
    );
    act(() => {
      screen.getByTestId('dispatch').click();
    });
    expect(screen.getByTestId('tab-0-find-case').textContent).toBe('true');
    expect(screen.getByTestId('tab-1-find-case').textContent).toBe('false');
  });

  test('flipping twice returns the value to its original state', () => {
    function Sequencer() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      return (
        <div>
          <span data-testid="value">{String(state.tabs[0]?.findCaseSensitive)}</span>
          <button
            data-testid="open"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: {
                  tabId: 'tab-1',
                  fileName: 'a.pdf',
                  filePath: '/tmp/a.pdf',
                  rootNode: catalogNode,
                  rootChildren: [],
                },
              })
            }
          />
          <button
            data-testid="on"
            onClick={() =>
              dispatch({
                type: 'SET_FIND_CASE_SENSITIVE',
                payload: { tabId: 'tab-1', value: true },
              })
            }
          />
          <button
            data-testid="off"
            onClick={() =>
              dispatch({
                type: 'SET_FIND_CASE_SENSITIVE',
                payload: { tabId: 'tab-1', value: false },
              })
            }
          />
        </div>
      );
    }
    render(
      <AppProvider>
        <Sequencer />
      </AppProvider>,
    );
    act(() => {
      screen.getByTestId('open').click();
    });
    expect(screen.getByTestId('value').textContent).toBe('false');
    act(() => {
      screen.getByTestId('on').click();
    });
    expect(screen.getByTestId('value').textContent).toBe('true');
    act(() => {
      screen.getByTestId('off').click();
    });
    expect(screen.getByTestId('value').textContent).toBe('false');
  });
});

// ---------------------------------------------------------------------------
// CLOSE_DOCUMENT drops the findCaseSensitive field with the rest of TabState
// (covered by tab-count dropping).
// ---------------------------------------------------------------------------

describe('CLOSE_DOCUMENT drops the field with the tab', () => {
  test('closing the only tab leaves tabs[] empty (the field dies with the tab)', () => {
    const actions: AppAction[] = [
      {
        type: 'OPEN_DOCUMENT',
        payload: {
          tabId: 'tab-1',
          fileName: 'a.pdf',
          filePath: '/tmp/a.pdf',
          rootNode: catalogNode,
          rootChildren: [],
        },
      },
      {
        type: 'SET_FIND_CASE_SENSITIVE',
        payload: { tabId: 'tab-1', value: true },
      },
      { type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-1' } },
    ];
    render(
      <AppProvider>
        <Inspector actions={actions} />
      </AppProvider>,
    );
    act(() => {
      screen.getByTestId('dispatch').click();
    });
    expect(screen.getByTestId('tab-count').textContent).toBe('0');
    expect(screen.getByTestId('tab-0-find-case').textContent).toBe('undefined');
  });
});

// ---------------------------------------------------------------------------
// SET_FIND_CASE_SENSITIVE on an unknown tabId is a no-op (does not throw,
// does not corrupt other tabs).
// ---------------------------------------------------------------------------

describe('SET_FIND_CASE_SENSITIVE on unknown tabId is a no-op', () => {
  test('dispatching to non-existent tabId leaves existing tabs unchanged', () => {
    const actions: AppAction[] = [
      {
        type: 'OPEN_DOCUMENT',
        payload: {
          tabId: 'tab-1',
          fileName: 'a.pdf',
          filePath: '/tmp/a.pdf',
          rootNode: catalogNode,
          rootChildren: [],
        },
      },
      {
        type: 'SET_FIND_CASE_SENSITIVE',
        payload: { tabId: 'tab-ghost', value: true },
      },
    ];
    render(
      <AppProvider>
        <Inspector actions={actions} />
      </AppProvider>,
    );
    act(() => {
      screen.getByTestId('dispatch').click();
    });
    expect(screen.getByTestId('tab-count').textContent).toBe('1');
    expect(screen.getByTestId('tab-0-find-case').textContent).toBe('false');
  });
});
