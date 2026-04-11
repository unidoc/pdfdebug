/**
 * 2.4-UNIT-001 [P1]: useDocumentState reducer handles OPEN_DOCUMENT action,
 * transitions from empty to loaded state.
 *
 * Also covers CLOSE_DOCUMENT, SET_DOCUMENT_ERROR, DISMISS_ERROR for completeness
 * since these are pure reducer logic at the lowest viable test layer.
 */
import { render, screen, act } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import { AppProvider, useAppState, useAppDispatch, type AppAction } from './useDocumentState';

// Helper component that exposes state and a dispatch trigger
function StateInspector({ action }: { action?: AppAction }) {
  const state = useAppState();
  const dispatch = useAppDispatch();
  return (
    <div>
      <span data-testid="active-tab-id">{state.activeTabId ?? 'null'}</span>
      <span data-testid="tab-count">{state.tabs.length}</span>
      <span data-testid="document-error">{state.documentError ?? 'null'}</span>
      <span data-testid="tab-filename">
        {state.tabs[0]?.fileName ?? 'null'}
      </span>
      <span data-testid="root-node-label">
        {state.tabs[0]?.rootNode?.label ?? 'null'}
      </span>
      <span data-testid="root-children-count">
        {state.tabs[0]?.rootChildren?.length ?? 'null'}
      </span>
      {action && (
        <button data-testid="dispatch" onClick={() => dispatch(action)}>
          dispatch
        </button>
      )}
    </div>
  );
}

const catalogNode = {
  id: 'root',
  label: 'Catalog',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 2,
  iconHint: 'catalog',
  error: '',
};

const childNodes = [
  {
    id: 'dict:root:Type',
    label: 'Type',
    rawKey: '/Type',
    nodeType: 'scalar',
    valueType: 'name',
    hasChildren: false,
    childCount: 0,
    iconHint: 'default',
    error: '',
  },
];

describe('2.4-UNIT-001: appReducer', () => {
  test('initial state has no tabs, no activeTabId, no error', () => {
    render(
      <AppProvider>
        <StateInspector />
      </AppProvider>
    );
    expect(screen.getByTestId('active-tab-id').textContent).toBe('null');
    expect(screen.getByTestId('tab-count').textContent).toBe('0');
    expect(screen.getByTestId('document-error').textContent).toBe('null');
  });

  test('OPEN_DOCUMENT creates tab with rootNode and rootChildren', () => {
    const action: AppAction = {
      type: 'OPEN_DOCUMENT',
      payload: {
        tabId: 'tab-1',
        fileName: 'test.pdf',
        filePath: '/path/to/test.pdf',
        rootNode: catalogNode,
        rootChildren: childNodes,
      },
    };
    render(
      <AppProvider>
        <StateInspector action={action} />
      </AppProvider>
    );

    act(() => {
      screen.getByTestId('dispatch').click();
    });

    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');
    expect(screen.getByTestId('tab-count').textContent).toBe('1');
    expect(screen.getByTestId('tab-filename').textContent).toBe('test.pdf');
    expect(screen.getByTestId('root-node-label').textContent).toBe('Catalog');
    expect(screen.getByTestId('root-children-count').textContent).toBe('1');
    expect(screen.getByTestId('document-error').textContent).toBe('null');
  });

  test('OPEN_DOCUMENT clears previous documentError', () => {
    // Dispatch SET_DOCUMENT_ERROR first, then OPEN_DOCUMENT
    function Sequencer() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      return (
        <div>
          <span data-testid="error">{state.documentError ?? 'null'}</span>
          <span data-testid="tabs">{state.tabs.length}</span>
          <button
            data-testid="set-error"
            onClick={() =>
              dispatch({
                type: 'SET_DOCUMENT_ERROR',
                payload: { message: 'bad file' },
              })
            }
          />
          <button
            data-testid="open-doc"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: {
                  tabId: 'tab-2',
                  fileName: 'good.pdf',
                  filePath: '/path/to/good.pdf',
                  rootNode: catalogNode,
                  rootChildren: childNodes,
                },
              })
            }
          />
        </div>
      );
    }

    render(
      <AppProvider>
        <Sequencer />
      </AppProvider>
    );

    act(() => screen.getByTestId('set-error').click());
    expect(screen.getByTestId('error').textContent).toBe('bad file');

    act(() => screen.getByTestId('open-doc').click());
    expect(screen.getByTestId('error').textContent).toBe('null');
    expect(screen.getByTestId('tabs').textContent).toBe('1');
  });

  test('CLOSE_DOCUMENT removes tab and clears activeTabId', () => {
    function Sequencer() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      return (
        <div>
          <span data-testid="tabs">{state.tabs.length}</span>
          <span data-testid="active">{state.activeTabId ?? 'null'}</span>
          <button
            data-testid="open"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: {
                  tabId: 'tab-x',
                  fileName: 'x.pdf',
                  filePath: '/path/to/x.pdf',
                  rootNode: null,
                  rootChildren: null,
                },
              })
            }
          />
          <button
            data-testid="close"
            onClick={() =>
              dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-x' } })
            }
          />
        </div>
      );
    }

    render(
      <AppProvider>
        <Sequencer />
      </AppProvider>
    );

    act(() => screen.getByTestId('open').click());
    expect(screen.getByTestId('tabs').textContent).toBe('1');

    act(() => screen.getByTestId('close').click());
    expect(screen.getByTestId('tabs').textContent).toBe('0');
    expect(screen.getByTestId('active').textContent).toBe('null');
  });

  test('SET_DOCUMENT_ERROR sets error and clears warning (from empty state)', () => {
    const action: AppAction = {
      type: 'SET_DOCUMENT_ERROR',
      payload: { message: 'encrypted file' },
    };
    render(
      <AppProvider>
        <StateInspector action={action} />
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    expect(screen.getByTestId('document-error').textContent).toBe(
      'encrypted file'
    );
    expect(screen.getByTestId('tab-count').textContent).toBe('0');
    expect(screen.getByTestId('active-tab-id').textContent).toBe('null');
  });

  test('DISMISS_ERROR clears documentError', () => {
    function Sequencer() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      return (
        <div>
          <span data-testid="error">{state.documentError ?? 'null'}</span>
          <button
            data-testid="set-error"
            onClick={() =>
              dispatch({
                type: 'SET_DOCUMENT_ERROR',
                payload: { message: 'oops' },
              })
            }
          />
          <button
            data-testid="dismiss"
            onClick={() => dispatch({ type: 'DISMISS_ERROR' })}
          />
        </div>
      );
    }

    render(
      <AppProvider>
        <Sequencer />
      </AppProvider>
    );

    act(() => screen.getByTestId('set-error').click());
    expect(screen.getByTestId('error').textContent).toBe('oops');

    act(() => screen.getByTestId('dismiss').click());
    expect(screen.getByTestId('error').textContent).toBe('null');
  });
});

// ---------------------------------------------------------------------------
// Story 4.2: Multi-Document State Isolation
//
// These tests verify tab-scoped reducer actions serve as regression guards.
// ---------------------------------------------------------------------------

const catalogNodeA = {
  id: 'root',
  label: 'Catalog A',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 2,
  iconHint: 'catalog',
  error: '',
};

const catalogNodeB = {
  id: 'root',
  label: 'Catalog B',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 3,
  iconHint: 'catalog',
  error: '',
};

// ---------------------------------------------------------------------------
// 4.2-UNIT-001 [P0]: SELECT_NODE only modifies the active tab's state;
// other tabs remain unchanged.
// AC#3: Each TabState is independent with its own selectedNodeId.
//
// Given two tabs are open with tab-1 active,
// When SELECT_NODE is dispatched selecting node "obj:0:5" in tab-1,
// And ACTIVATE_TAB switches to tab-2,
// And SELECT_NODE is dispatched selecting node "obj:0:10" in tab-2,
// Then tab-1's selectedNodeId remains "obj:0:5",
// And tab-2's selectedNodeId is "obj:0:10".
// ---------------------------------------------------------------------------

describe('4.2-UNIT-001: SELECT_NODE isolation', () => {
  test('selecting a node in one tab does not affect another tab', () => {
    function MultiTabInspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      const tab1 = state.tabs.find((t) => t.tabId === 'tab-1');
      const tab2 = state.tabs.find((t) => t.tabId === 'tab-2');
      return (
        <div>
          <span data-testid="active">{state.activeTabId ?? 'null'}</span>
          <span data-testid="tab1-selected">{tab1?.selectedNodeId ?? 'null'}</span>
          <span data-testid="tab2-selected">{tab2?.selectedNodeId ?? 'null'}</span>
          <button
            data-testid="open-tab1"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: catalogNodeA, rootChildren: null },
              })
            }
          />
          <button
            data-testid="open-tab2"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: catalogNodeB, rootChildren: null },
              })
            }
          />
          <button
            data-testid="activate-tab1"
            onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } })}
          />
          <button
            data-testid="activate-tab2"
            onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-2' } })}
          />
          <button
            data-testid="select-node-5"
            onClick={() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:5', label: 'Pages' } })}
          />
          <button
            data-testid="select-node-10"
            onClick={() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:10', label: 'Info' } })}
          />
        </div>
      );
    }

    render(
      <AppProvider>
        <MultiTabInspector />
      </AppProvider>
    );

    // Open two tabs
    act(() => screen.getByTestId('open-tab1').click());
    act(() => screen.getByTestId('open-tab2').click());
    expect(screen.getByTestId('active').textContent).toBe('tab-2');

    // Switch to tab-1 and select a node
    act(() => screen.getByTestId('activate-tab1').click());
    act(() => screen.getByTestId('select-node-5').click());
    expect(screen.getByTestId('tab1-selected').textContent).toBe('obj:0:5');
    expect(screen.getByTestId('tab2-selected').textContent).toBe('null');

    // Switch to tab-2 and select a different node
    act(() => screen.getByTestId('activate-tab2').click());
    act(() => screen.getByTestId('select-node-10').click());
    expect(screen.getByTestId('tab2-selected').textContent).toBe('obj:0:10');
    // Tab-1 must be unchanged
    expect(screen.getByTestId('tab1-selected').textContent).toBe('obj:0:5');
  });
});

// ---------------------------------------------------------------------------
// 4.2-UNIT-002 [P0]: NAVIGATE_TO_REF only modifies the active tab's
// pendingNavTarget; other tabs remain unchanged.
// AC#3: Each TabState has independent pendingNavTarget.
//
// Given two tabs are open with tab-1 active,
// When NAVIGATE_TO_REF is dispatched with targetNodeId "obj:0:7",
// Then tab-1's pendingNavTarget is "obj:0:7",
// And tab-2's pendingNavTarget remains null.
// ---------------------------------------------------------------------------

describe('4.2-UNIT-002: NAVIGATE_TO_REF isolation', () => {
  test('reference navigation only affects active tab', () => {
    function NavInspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      const tab1 = state.tabs.find((t) => t.tabId === 'tab-1');
      const tab2 = state.tabs.find((t) => t.tabId === 'tab-2');
      return (
        <div>
          <span data-testid="tab1-nav">{tab1?.pendingNavTarget ?? 'null'}</span>
          <span data-testid="tab2-nav">{tab2?.pendingNavTarget ?? 'null'}</span>
          <button
            data-testid="open-tab1"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: catalogNodeA, rootChildren: null },
              })
            }
          />
          <button
            data-testid="open-tab2"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: catalogNodeB, rootChildren: null },
              })
            }
          />
          <button
            data-testid="activate-tab1"
            onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } })}
          />
          <button
            data-testid="nav-to-ref"
            onClick={() => dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: 'obj:0:7' } })}
          />
        </div>
      );
    }

    render(
      <AppProvider>
        <NavInspector />
      </AppProvider>
    );

    act(() => screen.getByTestId('open-tab1').click());
    act(() => screen.getByTestId('open-tab2').click());

    // Switch to tab-1 and navigate to a reference
    act(() => screen.getByTestId('activate-tab1').click());
    act(() => screen.getByTestId('nav-to-ref').click());

    expect(screen.getByTestId('tab1-nav').textContent).toBe('obj:0:7');
    expect(screen.getByTestId('tab2-nav').textContent).toBe('null');
  });
});

// ---------------------------------------------------------------------------
// 4.2-UNIT-003 [P0]: SET_DOCUMENT_ERROR does not destroy other tabs' state.
// AC#1: Errors are global banners that do not destroy other tabs' state.
//
// Given two tabs are open with nodes selected in each,
// When SET_DOCUMENT_ERROR is dispatched,
// Then state.tabs still contains both tabs,
// And state.documentError is set,
// And each tab's selectedNodeId is preserved.
// ---------------------------------------------------------------------------

describe('4.2-UNIT-003: SET_DOCUMENT_ERROR preserves tabs', () => {
  test('error does not clear existing tabs or their state', () => {
    function ErrorInspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      const tab1 = state.tabs.find((t) => t.tabId === 'tab-1');
      const tab2 = state.tabs.find((t) => t.tabId === 'tab-2');
      return (
        <div>
          <span data-testid="tab-count">{state.tabs.length}</span>
          <span data-testid="error">{state.documentError ?? 'null'}</span>
          <span data-testid="tab1-selected">{tab1?.selectedNodeId ?? 'null'}</span>
          <span data-testid="tab2-selected">{tab2?.selectedNodeId ?? 'null'}</span>
          <span data-testid="tab1-file">{tab1?.fileName ?? 'null'}</span>
          <span data-testid="tab2-file">{tab2?.fileName ?? 'null'}</span>
          <button
            data-testid="open-tab1"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: catalogNodeA, rootChildren: null },
              })
            }
          />
          <button
            data-testid="open-tab2"
            onClick={() =>
              dispatch({
                type: 'OPEN_DOCUMENT',
                payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: catalogNodeB, rootChildren: null },
              })
            }
          />
          <button
            data-testid="activate-tab1"
            onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } })}
          />
          <button
            data-testid="select-root"
            onClick={() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'root', label: 'Catalog' } })}
          />
          <button
            data-testid="set-error"
            onClick={() =>
              dispatch({ type: 'SET_DOCUMENT_ERROR', payload: { message: 'encrypted file' } })
            }
          />
        </div>
      );
    }

    render(
      <AppProvider>
        <ErrorInspector />
      </AppProvider>
    );

    // Open two tabs and select nodes in each
    act(() => screen.getByTestId('open-tab1').click());
    act(() => screen.getByTestId('select-root').click());
    act(() => screen.getByTestId('open-tab2').click());
    act(() => screen.getByTestId('select-root').click());

    // Both tabs have selected nodes
    expect(screen.getByTestId('tab1-selected').textContent).toBe('root');
    expect(screen.getByTestId('tab2-selected').textContent).toBe('root');

    // Fire an error
    act(() => screen.getByTestId('set-error').click());

    // Error is set, but tabs are fully preserved
    expect(screen.getByTestId('error').textContent).toBe('encrypted file');
    expect(screen.getByTestId('tab-count').textContent).toBe('2');
    expect(screen.getByTestId('tab1-file').textContent).toBe('a.pdf');
    expect(screen.getByTestId('tab2-file').textContent).toBe('b.pdf');
    expect(screen.getByTestId('tab1-selected').textContent).toBe('root');
    expect(screen.getByTestId('tab2-selected').textContent).toBe('root');
  });
});

// ---------------------------------------------------------------------------
// Story 4.2 supplemental: Isolation tests for remaining tab-scoped actions.
//
// Task 3.2 in the story spec identifies CLEAR_NAV_TARGET, NAV_ERROR,
// DISMISS_NAV_ERROR, NAVIGATE_BACK, and NAVIGATE_FORWARD as all filtering
// by activeTabId. These tests verify that contract at the reducer level.
// ---------------------------------------------------------------------------

describe('4.2 supplemental: CLEAR_NAV_TARGET isolation', () => {
  test('clears pendingNavTarget only on active tab', () => {
    function Inspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      const tab1 = state.tabs.find((t) => t.tabId === 'tab-1');
      const tab2 = state.tabs.find((t) => t.tabId === 'tab-2');
      return (
        <div>
          <span data-testid="tab1-nav">{tab1?.pendingNavTarget ?? 'null'}</span>
          <span data-testid="tab2-nav">{tab2?.pendingNavTarget ?? 'null'}</span>
          <button data-testid="open-tab1" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: catalogNodeA, rootChildren: null } })} />
          <button data-testid="open-tab2" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: catalogNodeB, rootChildren: null } })} />
          <button data-testid="activate-tab1" onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } })} />
          <button data-testid="activate-tab2" onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-2' } })} />
          <button data-testid="nav-to-ref" onClick={() => dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: 'obj:0:7' } })} />
          <button data-testid="clear-nav" onClick={() => dispatch({ type: 'CLEAR_NAV_TARGET' })} />
        </div>
      );
    }

    render(<AppProvider><Inspector /></AppProvider>);

    act(() => screen.getByTestId('open-tab1').click());
    act(() => screen.getByTestId('open-tab2').click());

    // Set pendingNavTarget on both tabs
    act(() => screen.getByTestId('activate-tab1').click());
    act(() => screen.getByTestId('nav-to-ref').click());
    act(() => screen.getByTestId('activate-tab2').click());
    act(() => screen.getByTestId('nav-to-ref').click());

    expect(screen.getByTestId('tab1-nav').textContent).toBe('obj:0:7');
    expect(screen.getByTestId('tab2-nav').textContent).toBe('obj:0:7');

    // Clear only on active tab (tab-2)
    act(() => screen.getByTestId('clear-nav').click());

    expect(screen.getByTestId('tab2-nav').textContent).toBe('null');
    expect(screen.getByTestId('tab1-nav').textContent).toBe('obj:0:7');
  });
});

describe('4.2 supplemental: NAV_ERROR isolation', () => {
  test('sets navError only on active tab', () => {
    function Inspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      const tab1 = state.tabs.find((t) => t.tabId === 'tab-1');
      const tab2 = state.tabs.find((t) => t.tabId === 'tab-2');
      return (
        <div>
          <span data-testid="tab1-nav-error">{tab1?.navError ?? 'null'}</span>
          <span data-testid="tab2-nav-error">{tab2?.navError ?? 'null'}</span>
          <span data-testid="tab1-nav">{tab1?.pendingNavTarget ?? 'null'}</span>
          <button data-testid="open-tab1" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: catalogNodeA, rootChildren: null } })} />
          <button data-testid="open-tab2" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: catalogNodeB, rootChildren: null } })} />
          <button data-testid="activate-tab1" onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } })} />
          <button data-testid="nav-to-ref" onClick={() => dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: 'obj:0:99' } })} />
          <button data-testid="nav-error" onClick={() => dispatch({ type: 'NAV_ERROR', payload: { message: 'node not found' } })} />
        </div>
      );
    }

    render(<AppProvider><Inspector /></AppProvider>);

    act(() => screen.getByTestId('open-tab1').click());
    act(() => screen.getByTestId('open-tab2').click());

    // Set nav on tab-1, then trigger NAV_ERROR on tab-1
    act(() => screen.getByTestId('activate-tab1').click());
    act(() => screen.getByTestId('nav-to-ref').click());
    expect(screen.getByTestId('tab1-nav').textContent).toBe('obj:0:99');

    act(() => screen.getByTestId('nav-error').click());

    expect(screen.getByTestId('tab1-nav-error').textContent).toBe('node not found');
    expect(screen.getByTestId('tab1-nav').textContent).toBe('null');
    expect(screen.getByTestId('tab2-nav-error').textContent).toBe('null');
  });
});

describe('4.2 supplemental: DISMISS_NAV_ERROR isolation', () => {
  test('clears navError only on active tab', () => {
    function Inspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      const tab1 = state.tabs.find((t) => t.tabId === 'tab-1');
      const tab2 = state.tabs.find((t) => t.tabId === 'tab-2');
      return (
        <div>
          <span data-testid="tab1-nav-error">{tab1?.navError ?? 'null'}</span>
          <span data-testid="tab2-nav-error">{tab2?.navError ?? 'null'}</span>
          <button data-testid="open-tab1" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: catalogNodeA, rootChildren: null } })} />
          <button data-testid="open-tab2" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: catalogNodeB, rootChildren: null } })} />
          <button data-testid="activate-tab1" onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } })} />
          <button data-testid="activate-tab2" onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-2' } })} />
          <button data-testid="nav-error" onClick={() => dispatch({ type: 'NAV_ERROR', payload: { message: 'broken ref' } })} />
          <button data-testid="dismiss-nav-error" onClick={() => dispatch({ type: 'DISMISS_NAV_ERROR' })} />
        </div>
      );
    }

    render(<AppProvider><Inspector /></AppProvider>);

    act(() => screen.getByTestId('open-tab1').click());
    act(() => screen.getByTestId('open-tab2').click());

    // Set navError on both tabs
    act(() => screen.getByTestId('activate-tab1').click());
    act(() => screen.getByTestId('nav-error').click());
    act(() => screen.getByTestId('activate-tab2').click());
    act(() => screen.getByTestId('nav-error').click());

    expect(screen.getByTestId('tab1-nav-error').textContent).toBe('broken ref');
    expect(screen.getByTestId('tab2-nav-error').textContent).toBe('broken ref');

    // Dismiss only on active tab (tab-2)
    act(() => screen.getByTestId('dismiss-nav-error').click());

    expect(screen.getByTestId('tab2-nav-error').textContent).toBe('null');
    expect(screen.getByTestId('tab1-nav-error').textContent).toBe('broken ref');
  });
});

describe('4.2 supplemental: NAVIGATE_BACK/FORWARD isolation', () => {
  test('history navigation only affects active tab', () => {
    function Inspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      const tab1 = state.tabs.find((t) => t.tabId === 'tab-1');
      const tab2 = state.tabs.find((t) => t.tabId === 'tab-2');
      return (
        <div>
          <span data-testid="tab1-selected">{tab1?.selectedNodeId ?? 'null'}</span>
          <span data-testid="tab1-history-idx">{tab1?.navHistoryIndex ?? -1}</span>
          <span data-testid="tab2-selected">{tab2?.selectedNodeId ?? 'null'}</span>
          <span data-testid="tab2-history-idx">{tab2?.navHistoryIndex ?? -1}</span>
          <button data-testid="open-tab1" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: catalogNodeA, rootChildren: null } })} />
          <button data-testid="open-tab2" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: catalogNodeB, rootChildren: null } })} />
          <button data-testid="activate-tab1" onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } })} />
          <button data-testid="activate-tab2" onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-2' } })} />
          <button data-testid="select-a" onClick={() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'node-a', label: 'A' } })} />
          <button data-testid="select-b" onClick={() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'node-b', label: 'B' } })} />
          <button data-testid="select-c" onClick={() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'node-c', label: 'C' } })} />
          <button data-testid="nav-back" onClick={() => dispatch({ type: 'NAVIGATE_BACK' })} />
          <button data-testid="nav-forward" onClick={() => dispatch({ type: 'NAVIGATE_FORWARD' })} />
        </div>
      );
    }

    render(<AppProvider><Inspector /></AppProvider>);

    // Open two tabs
    act(() => screen.getByTestId('open-tab1').click());
    act(() => screen.getByTestId('open-tab2').click());

    // Build history on tab-1: A -> B -> C
    act(() => screen.getByTestId('activate-tab1').click());
    act(() => screen.getByTestId('select-a').click());
    act(() => screen.getByTestId('select-b').click());
    act(() => screen.getByTestId('select-c').click());
    expect(screen.getByTestId('tab1-selected').textContent).toBe('node-c');
    expect(screen.getByTestId('tab1-history-idx').textContent).toBe('2');

    // Build history on tab-2: A only
    act(() => screen.getByTestId('activate-tab2').click());
    act(() => screen.getByTestId('select-a').click());
    expect(screen.getByTestId('tab2-selected').textContent).toBe('node-a');
    expect(screen.getByTestId('tab2-history-idx').textContent).toBe('0');

    // Navigate back on tab-2 (already at index 0, should be no-op)
    act(() => screen.getByTestId('nav-back').click());
    expect(screen.getByTestId('tab2-selected').textContent).toBe('node-a');
    expect(screen.getByTestId('tab2-history-idx').textContent).toBe('0');

    // Tab-1 must be unaffected
    expect(screen.getByTestId('tab1-selected').textContent).toBe('node-c');
    expect(screen.getByTestId('tab1-history-idx').textContent).toBe('2');

    // Switch to tab-1 and navigate back
    act(() => screen.getByTestId('activate-tab1').click());
    act(() => screen.getByTestId('nav-back').click());
    expect(screen.getByTestId('tab1-selected').textContent).toBe('node-b');
    expect(screen.getByTestId('tab1-history-idx').textContent).toBe('1');

    // Tab-2 must be unaffected
    expect(screen.getByTestId('tab2-selected').textContent).toBe('node-a');
    expect(screen.getByTestId('tab2-history-idx').textContent).toBe('0');

    // Navigate forward on tab-1
    act(() => screen.getByTestId('nav-forward').click());
    expect(screen.getByTestId('tab1-selected').textContent).toBe('node-c');
    expect(screen.getByTestId('tab1-history-idx').textContent).toBe('2');

    // Tab-2 still unaffected
    expect(screen.getByTestId('tab2-selected').textContent).toBe('node-a');
  });
});
