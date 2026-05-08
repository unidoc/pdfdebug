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

// ---------------------------------------------------------------------------
// Story 4.3: Close Document and Tab Management
//
// These tests verify CLOSE_DOCUMENT reducer behavior with 3+ tabs, focus
// transfer logic, and return-to-empty-state. The reducer is already
// implemented (story 4-1). These tests are verification-only.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 4.3-UNIT-001 [P0]: CLOSE_DOCUMENT removes only the target tab; other
// tabs remain.
// AC#1: Document is closed, tab is removed, other tabs unaffected.
//
// Given 3 tabs are open (tab-1, tab-2, tab-3) with tab-3 active,
// When CLOSE_DOCUMENT is dispatched for tab-2 (non-active),
// Then state.tabs.length === 2,
// And tab-1 and tab-3 are still present,
// And tab-2 is gone.
// ---------------------------------------------------------------------------

describe('4.3-UNIT-001: CLOSE_DOCUMENT removes only the target tab', () => {
  test('closing a tab leaves other tabs intact', () => {
    function Inspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      const tabIds = state.tabs.map((t) => t.tabId).join(',');
      return (
        <div>
          <span data-testid="tab-count">{state.tabs.length}</span>
          <span data-testid="tab-ids">{tabIds}</span>
          <span data-testid="active">{state.activeTabId ?? 'null'}</span>
          <button data-testid="open-1" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-2" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-3" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-3', fileName: 'c.pdf', filePath: '/c.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="close-2" onClick={() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-2' } })} />
        </div>
      );
    }

    render(<AppProvider><Inspector /></AppProvider>);

    act(() => screen.getByTestId('open-1').click());
    act(() => screen.getByTestId('open-2').click());
    act(() => screen.getByTestId('open-3').click());
    expect(screen.getByTestId('tab-count').textContent).toBe('3');
    expect(screen.getByTestId('active').textContent).toBe('tab-3');

    // Close tab-2 (non-active)
    act(() => screen.getByTestId('close-2').click());

    expect(screen.getByTestId('tab-count').textContent).toBe('2');
    expect(screen.getByTestId('tab-ids').textContent).toBe('tab-1,tab-3');
  });
});

// ---------------------------------------------------------------------------
// 4.3-UNIT-002 [P0]: Closing active tab moves activeTabId to the next tab
// (or previous if last).
// AC#1: Focus moves to the next tab (or the previous tab if the closed tab
//       was the last one).
//
// (a) Close the last-in-array tab (tab-3): activeTabId -> tab-2 (previous).
// (b) Close the first-in-array tab (tab-1): activeTabId -> tab-2 (next).
// ---------------------------------------------------------------------------

describe('4.3-UNIT-002: Closing active tab transfers focus correctly', () => {
  test('(a) closing last-in-array active tab falls back to previous', () => {
    function Inspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      return (
        <div>
          <span data-testid="active">{state.activeTabId ?? 'null'}</span>
          <span data-testid="tab-count">{state.tabs.length}</span>
          <button data-testid="open-1" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-2" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-3" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-3', fileName: 'c.pdf', filePath: '/c.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="close-3" onClick={() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-3' } })} />
        </div>
      );
    }

    render(<AppProvider><Inspector /></AppProvider>);

    act(() => screen.getByTestId('open-1').click());
    act(() => screen.getByTestId('open-2').click());
    act(() => screen.getByTestId('open-3').click());
    expect(screen.getByTestId('active').textContent).toBe('tab-3');

    // Close tab-3 (active, last in array)
    // Reducer: closedIndex=2, filtered=[tab-1,tab-2], Math.min(2,1)=1 -> tab-2
    act(() => screen.getByTestId('close-3').click());

    expect(screen.getByTestId('active').textContent).toBe('tab-2');
    expect(screen.getByTestId('tab-count').textContent).toBe('2');
  });

  test('(b) closing first-in-array active tab moves to next', () => {
    function Inspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      return (
        <div>
          <span data-testid="active">{state.activeTabId ?? 'null'}</span>
          <span data-testid="tab-count">{state.tabs.length}</span>
          <button data-testid="open-1" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-2" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-3" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-3', fileName: 'c.pdf', filePath: '/c.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="activate-1" onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } })} />
          <button data-testid="close-1" onClick={() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-1' } })} />
        </div>
      );
    }

    render(<AppProvider><Inspector /></AppProvider>);

    act(() => screen.getByTestId('open-1').click());
    act(() => screen.getByTestId('open-2').click());
    act(() => screen.getByTestId('open-3').click());

    // Activate tab-1 so it is active
    act(() => screen.getByTestId('activate-1').click());
    expect(screen.getByTestId('active').textContent).toBe('tab-1');

    // Close tab-1 (active, first in array)
    // Reducer: closedIndex=0, filtered=[tab-2,tab-3], Math.min(0,1)=0 -> tab-2
    act(() => screen.getByTestId('close-1').click());

    expect(screen.getByTestId('active').textContent).toBe('tab-2');
    expect(screen.getByTestId('tab-count').textContent).toBe('2');
  });
});

// ---------------------------------------------------------------------------
// 4.3-UNIT-003 [P0]: Closing the last tab sets activeTabId to null (empty state).
// AC#2: When no documents remain open, the empty state is shown again.
//
// Given 1 tab is open,
// When CLOSE_DOCUMENT is dispatched for that tab,
// Then state.tabs.length === 0,
// And state.activeTabId === null.
// ---------------------------------------------------------------------------

describe('4.3-UNIT-003: Closing last tab returns to empty state', () => {
  test('closing the only tab sets activeTabId to null', () => {
    function Inspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      return (
        <div>
          <span data-testid="active">{state.activeTabId ?? 'null'}</span>
          <span data-testid="tab-count">{state.tabs.length}</span>
          <button data-testid="open" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="close" onClick={() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-1' } })} />
        </div>
      );
    }

    render(<AppProvider><Inspector /></AppProvider>);

    act(() => screen.getByTestId('open').click());
    expect(screen.getByTestId('tab-count').textContent).toBe('1');
    expect(screen.getByTestId('active').textContent).toBe('tab-1');

    act(() => screen.getByTestId('close').click());

    expect(screen.getByTestId('tab-count').textContent).toBe('0');
    expect(screen.getByTestId('active').textContent).toBe('null');
  });
});

// ---------------------------------------------------------------------------
// 4.3-UNIT-007 [P2]: Closing a non-active tab does not change the active tab.
// AC#1: Tab is removed, focus stays on the current active tab.
//
// Given 3 tabs are open with tab-3 active,
// When CLOSE_DOCUMENT is dispatched for tab-1 (non-active),
// Then activeTabId is still 'tab-3',
// And state.tabs.length === 2.
// ---------------------------------------------------------------------------

describe('4.3-UNIT-007: Closing non-active tab preserves activeTabId', () => {
  test('closing a background tab does not change active tab', () => {
    function Inspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      return (
        <div>
          <span data-testid="active">{state.activeTabId ?? 'null'}</span>
          <span data-testid="tab-count">{state.tabs.length}</span>
          <button data-testid="open-1" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-2" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-3" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-3', fileName: 'c.pdf', filePath: '/c.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="close-1" onClick={() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-1' } })} />
        </div>
      );
    }

    render(<AppProvider><Inspector /></AppProvider>);

    act(() => screen.getByTestId('open-1').click());
    act(() => screen.getByTestId('open-2').click());
    act(() => screen.getByTestId('open-3').click());
    expect(screen.getByTestId('active').textContent).toBe('tab-3');

    act(() => screen.getByTestId('close-1').click());

    expect(screen.getByTestId('active').textContent).toBe('tab-3');
    expect(screen.getByTestId('tab-count').textContent).toBe('2');
  });
});

// ---------------------------------------------------------------------------
// 4.3-UNIT-008 [P2]: Rapidly closing multiple tabs does not corrupt state.
// AC#1: Tab management handles rapid sequential closes gracefully.
//
// Given 4 tabs are open (tab-1..tab-4, tab-4 active),
// When CLOSE_DOCUMENT is dispatched for tab-2, tab-3, tab-4 in rapid
//   succession (synchronous dispatches within a single act() block),
// Then state.tabs.length === 1, only tab-1 remains,
// And activeTabId === 'tab-1'.
// ---------------------------------------------------------------------------

describe('4.3-UNIT-008: Rapid sequential closes do not corrupt state', () => {
  test('closing 3 of 4 tabs in rapid succession leaves correct state', () => {
    function Inspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      const tabIds = state.tabs.map((t) => t.tabId).join(',');
      return (
        <div>
          <span data-testid="active">{state.activeTabId ?? 'null'}</span>
          <span data-testid="tab-count">{state.tabs.length}</span>
          <span data-testid="tab-ids">{tabIds}</span>
          <button data-testid="open-1" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-2" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-3" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-3', fileName: 'c.pdf', filePath: '/c.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-4" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-4', fileName: 'd.pdf', filePath: '/d.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="close-2" onClick={() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-2' } })} />
          <button data-testid="close-3" onClick={() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-3' } })} />
          <button data-testid="close-4" onClick={() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-4' } })} />
        </div>
      );
    }

    render(<AppProvider><Inspector /></AppProvider>);

    act(() => screen.getByTestId('open-1').click());
    act(() => screen.getByTestId('open-2').click());
    act(() => screen.getByTestId('open-3').click());
    act(() => screen.getByTestId('open-4').click());
    expect(screen.getByTestId('tab-count').textContent).toBe('4');
    expect(screen.getByTestId('active').textContent).toBe('tab-4');

    // Rapid close: tab-2 (non-active), tab-3 (non-active), tab-4 (active)
    act(() => {
      screen.getByTestId('close-2').click();
      screen.getByTestId('close-3').click();
      screen.getByTestId('close-4').click();
    });

    expect(screen.getByTestId('tab-count').textContent).toBe('1');
    expect(screen.getByTestId('tab-ids').textContent).toBe('tab-1');
    expect(screen.getByTestId('active').textContent).toBe('tab-1');
  });
});

// ---------------------------------------------------------------------------
// 4.3-UNIT-002 supplemental: Closing middle active tab prefers the next tab.
// AC#1: Focus moves to the next tab (or the previous tab if the closed tab
//       was the last one).
//
// Given 3 tabs are open with tab-2 active (middle),
// When CLOSE_DOCUMENT is dispatched for tab-2,
// Then activeTabId === 'tab-3' (next tab, not previous).
// Reducer: closedIndex=1, filtered=[tab-1,tab-3], Math.min(1,1)=1 -> tab-3.
// ---------------------------------------------------------------------------

describe('4.3-UNIT-002 supplemental: Closing middle active tab prefers next', () => {
  test('closing middle active tab activates the next tab to the right', () => {
    function Inspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      return (
        <div>
          <span data-testid="active">{state.activeTabId ?? 'null'}</span>
          <span data-testid="tab-count">{state.tabs.length}</span>
          <button data-testid="open-1" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-2" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-3" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-3', fileName: 'c.pdf', filePath: '/c.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="activate-2" onClick={() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-2' } })} />
          <button data-testid="close-2" onClick={() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-2' } })} />
        </div>
      );
    }

    render(<AppProvider><Inspector /></AppProvider>);

    act(() => screen.getByTestId('open-1').click());
    act(() => screen.getByTestId('open-2').click());
    act(() => screen.getByTestId('open-3').click());

    // Activate tab-2 (middle)
    act(() => screen.getByTestId('activate-2').click());
    expect(screen.getByTestId('active').textContent).toBe('tab-2');

    // Close tab-2: closedIndex=1, filtered=[tab-1,tab-3], Math.min(1,1)=1 -> tab-3
    act(() => screen.getByTestId('close-2').click());

    expect(screen.getByTestId('active').textContent).toBe('tab-3');
    expect(screen.getByTestId('tab-count').textContent).toBe('2');
  });
});

// ---------------------------------------------------------------------------
// 4.3 supplemental: CLOSE_DOCUMENT clears documentError only when closing
// the active tab. When closing a non-active tab, documentError is preserved.
// AC#1: Resources are freed; state cleanup is correct.
//
// Reducer logic: documentError: closingActive ? null : state.documentError
// ---------------------------------------------------------------------------

describe('4.3 supplemental: CLOSE_DOCUMENT clears documentError conditionally', () => {
  test('closing active tab clears documentError; closing non-active preserves it', () => {
    function Inspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      return (
        <div>
          <span data-testid="active">{state.activeTabId ?? 'null'}</span>
          <span data-testid="tab-count">{state.tabs.length}</span>
          <span data-testid="error">{state.documentError ?? 'null'}</span>
          <button data-testid="open-1" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/a.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-2" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-2', fileName: 'b.pdf', filePath: '/b.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="open-3" onClick={() => dispatch({ type: 'OPEN_DOCUMENT', payload: { tabId: 'tab-3', fileName: 'c.pdf', filePath: '/c.pdf', rootNode: null, rootChildren: null } })} />
          <button data-testid="set-error" onClick={() => dispatch({ type: 'SET_DOCUMENT_ERROR', payload: { message: 'corrupt PDF' } })} />
          <button data-testid="close-1" onClick={() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-1' } })} />
          <button data-testid="close-3" onClick={() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-3' } })} />
        </div>
      );
    }

    render(<AppProvider><Inspector /></AppProvider>);

    act(() => screen.getByTestId('open-1').click());
    act(() => screen.getByTestId('open-2').click());
    act(() => screen.getByTestId('open-3').click());
    expect(screen.getByTestId('active').textContent).toBe('tab-3');

    // Set an error
    act(() => screen.getByTestId('set-error').click());
    expect(screen.getByTestId('error').textContent).toBe('corrupt PDF');

    // Close tab-1 (non-active) -- error should be preserved
    act(() => screen.getByTestId('close-1').click());
    expect(screen.getByTestId('error').textContent).toBe('corrupt PDF');
    expect(screen.getByTestId('tab-count').textContent).toBe('2');

    // Close tab-3 (active) -- error should be cleared
    act(() => screen.getByTestId('close-3').click());
    expect(screen.getByTestId('error').textContent).toBe('null');
    expect(screen.getByTestId('tab-count').textContent).toBe('1');
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

// ---------------------------------------------------------------------------
// Multi-PDF drop reducer flows: distinct paths produce N tabs, same paths
// dedup, and BATCH_OPEN_* actions drive the progress dialog state.
// ---------------------------------------------------------------------------

describe('multi-PDF drop: reducer integrates with backend events', () => {
  test('three OPEN_DOCUMENT dispatches with distinct paths produce 3 tabs, last active', () => {
    const dispatchActions: AppAction[] = [
      { type: 'OPEN_DOCUMENT', payload: { tabId: 't1', fileName: 'a.pdf', filePath: '/a.pdf', pageCount: 1, rootNode: catalogNode, rootChildren: childNodes } },
      { type: 'OPEN_DOCUMENT', payload: { tabId: 't2', fileName: 'b.pdf', filePath: '/b.pdf', pageCount: 1, rootNode: catalogNode, rootChildren: childNodes } },
      { type: 'OPEN_DOCUMENT', payload: { tabId: 't3', fileName: 'c.pdf', filePath: '/c.pdf', pageCount: 1, rootNode: catalogNode, rootChildren: childNodes } },
    ];
    function MultiInspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      return (
        <div>
          <span data-testid="tab-count">{state.tabs.length}</span>
          <span data-testid="active-tab-id">{state.activeTabId ?? 'null'}</span>
          <button data-testid="dispatch-all" onClick={() => dispatchActions.forEach(dispatch)}>go</button>
        </div>
      );
    }
    render(<AppProvider><MultiInspector /></AppProvider>);
    act(() => screen.getByTestId('dispatch-all').click());
    expect(screen.getByTestId('tab-count').textContent).toBe('3');
    expect(screen.getByTestId('active-tab-id').textContent).toBe('t3');
  });

  test('three OPEN_DOCUMENT dispatches with the SAME path dedup to 1 tab', () => {
    const dup: AppAction = {
      type: 'OPEN_DOCUMENT',
      payload: { tabId: 't1', fileName: 'a.pdf', filePath: '/same.pdf', pageCount: 1, rootNode: catalogNode, rootChildren: childNodes },
    };
    function DupInspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      return (
        <div>
          <span data-testid="tab-count">{state.tabs.length}</span>
          <button data-testid="dispatch-all" onClick={() => {
            dispatch(dup);
            dispatch({ ...dup, payload: { ...dup.payload, tabId: 't2' } });
            dispatch({ ...dup, payload: { ...dup.payload, tabId: 't3' } });
          }}>go</button>
        </div>
      );
    }
    render(<AppProvider><DupInspector /></AppProvider>);
    act(() => screen.getByTestId('dispatch-all').click());
    expect(screen.getByTestId('tab-count').textContent).toBe('1');
  });

  test('BATCH_OPEN_* actions sequence: start -> OPEN_DOCUMENT bumps completed -> complete', () => {
    function BatchInspector() {
      const state = useAppState();
      const dispatch = useAppDispatch();
      return (
        <div>
          <span data-testid="batch-total">{state.batchOpenTotal}</span>
          <span data-testid="batch-completed">{state.batchOpenCompleted}</span>
          <button data-testid="start" onClick={() => dispatch({ type: 'BATCH_OPEN_START', payload: { total: 5 } })}>s</button>
          <button data-testid="open" onClick={() => dispatch({
            type: 'OPEN_DOCUMENT',
            payload: {
              tabId: `t-${state.batchOpenCompleted + 1}`,
              fileName: `f${state.batchOpenCompleted + 1}.pdf`,
              filePath: `/f${state.batchOpenCompleted + 1}.pdf`,
              pageCount: 1, rootNode: null, rootChildren: null,
            },
          })}>o</button>
          <button data-testid="done" onClick={() => dispatch({ type: 'BATCH_OPEN_COMPLETE' })}>d</button>
        </div>
      );
    }
    render(<AppProvider><BatchInspector /></AppProvider>);
    expect(screen.getByTestId('batch-total').textContent).toBe('0');

    act(() => screen.getByTestId('start').click());
    expect(screen.getByTestId('batch-total').textContent).toBe('5');
    expect(screen.getByTestId('batch-completed').textContent).toBe('0');

    // OPEN_DOCUMENT bumps batchOpenCompleted (atomic with tab-add) so the
    // count never lags behind the actual number of opened tabs.
    act(() => screen.getByTestId('open').click());
    expect(screen.getByTestId('batch-completed').textContent).toBe('1');
    act(() => screen.getByTestId('open').click());
    expect(screen.getByTestId('batch-completed').textContent).toBe('2');
    act(() => screen.getByTestId('open').click());
    expect(screen.getByTestId('batch-completed').textContent).toBe('3');

    act(() => screen.getByTestId('done').click());
    expect(screen.getByTestId('batch-total').textContent).toBe('0');
    expect(screen.getByTestId('batch-completed').textContent).toBe('0');
  });
});

// ---------------------------------------------------------------------------
// 9.4-UNIT-001: OPEN_GO_TO_PAGE / CLOSE_GO_TO_PAGE reducer paths
// ---------------------------------------------------------------------------

function GoToPageInspector({ pageCount, openAction }: { pageCount: number; openAction: AppAction }) {
  const state = useAppState();
  const dispatch = useAppDispatch();
  return (
    <div>
      <span data-testid="goto-open">{String(state.goToPageOpen)}</span>
      <button data-testid="open-doc" onClick={() => dispatch(openAction)}>open</button>
      <button data-testid="open-goto" onClick={() => dispatch({ type: 'OPEN_GO_TO_PAGE' })}>open-goto</button>
      <button data-testid="close-goto" onClick={() => dispatch({ type: 'CLOSE_GO_TO_PAGE' })}>close-goto</button>
      <span data-testid="page-count">{state.tabs[0]?.pageCount ?? -1}</span>
      <span data-testid="page-count-fallback">{pageCount}</span>
    </div>
  );
}

describe('9.4-UNIT-001: Go to Page dialog state', () => {
  test('initial state has goToPageOpen = false', () => {
    render(
      <AppProvider>
        <GoToPageInspector pageCount={0} openAction={{ type: 'CLOSE_GO_TO_PAGE' }} />
      </AppProvider>
    );
    expect(screen.getByTestId('goto-open').textContent).toBe('false');
  });

  test('OPEN_GO_TO_PAGE is a no-op when no document is loaded', () => {
    render(
      <AppProvider>
        <GoToPageInspector pageCount={0} openAction={{ type: 'CLOSE_GO_TO_PAGE' }} />
      </AppProvider>
    );
    act(() => screen.getByTestId('open-goto').click());
    expect(screen.getByTestId('goto-open').textContent).toBe('false');
  });

  test('OPEN_GO_TO_PAGE is a no-op when active tab has pageCount = 0', () => {
    const open: AppAction = {
      type: 'OPEN_DOCUMENT',
      payload: { tabId: 't1', fileName: 'a.pdf', filePath: '/a.pdf', pageCount: 0, rootNode: catalogNode, rootChildren: childNodes },
    };
    render(
      <AppProvider>
        <GoToPageInspector pageCount={0} openAction={open} />
      </AppProvider>
    );
    act(() => screen.getByTestId('open-doc').click());
    expect(screen.getByTestId('page-count').textContent).toBe('0');
    act(() => screen.getByTestId('open-goto').click());
    expect(screen.getByTestId('goto-open').textContent).toBe('false');
  });

  test('OPEN_GO_TO_PAGE flips goToPageOpen to true when a document with pages is active; CLOSE flips it back', () => {
    const open: AppAction = {
      type: 'OPEN_DOCUMENT',
      payload: { tabId: 't1', fileName: 'a.pdf', filePath: '/a.pdf', pageCount: 5, rootNode: catalogNode, rootChildren: childNodes },
    };
    render(
      <AppProvider>
        <GoToPageInspector pageCount={5} openAction={open} />
      </AppProvider>
    );
    act(() => screen.getByTestId('open-doc').click());
    expect(screen.getByTestId('page-count').textContent).toBe('5');

    act(() => screen.getByTestId('open-goto').click());
    expect(screen.getByTestId('goto-open').textContent).toBe('true');

    act(() => screen.getByTestId('close-goto').click());
    expect(screen.getByTestId('goto-open').textContent).toBe('false');
  });
});
