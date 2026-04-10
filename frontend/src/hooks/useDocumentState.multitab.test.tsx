/**
 * Story 4.1: Tab Bar Component for Multi-Document Management
 *
 * TDD RED PHASE: Tests MUST fail until story 4-1 is implemented.
 *
 * Reducer-level acceptance tests for multi-tab state management:
 *   4.1-UNIT-001 [P0]: OPEN_DOCUMENT appends new tab (does not replace)
 *   4.1-UNIT-002 [P0]: OPEN_DOCUMENT sets activeTabId to the new tab
 *   4.1-UNIT-003 [P0]: ACTIVATE_TAB sets activeTabId without modifying tab state
 *   4.1-UNIT-010 [P2]: Duplicate filePath focuses existing tab instead of opening new
 *
 * Run: cd frontend && npx vitest run src/hooks/useDocumentState.multitab.test.tsx
 */
import { render, screen, act } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import {
  AppProvider,
  useAppState,
  useAppDispatch,
  type AppAction,
} from './useDocumentState';

// --- Test fixtures ---

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

const catalogNode2 = {
  id: 'root',
  label: 'Catalog 2',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 3,
  iconHint: 'catalog',
  error: '',
};

const childNodes2 = [
  {
    id: 'dict:root:Pages',
    label: 'Pages',
    rawKey: '/Pages',
    nodeType: 'dict',
    valueType: 'reference',
    hasChildren: true,
    childCount: 5,
    iconHint: 'page',
    error: '',
  },
];

// --- Helper component that exposes full multi-tab state ---

function MultiTabInspector() {
  const state = useAppState();
  const dispatch = useAppDispatch();
  return (
    <div>
      <span data-testid="active-tab-id">{state.activeTabId ?? 'null'}</span>
      <span data-testid="tab-count">{state.tabs.length}</span>
      {state.tabs.map((tab, i) => (
        <div key={tab.tabId} data-testid={`tab-${i}`}>
          <span data-testid={`tab-${i}-id`}>{tab.tabId}</span>
          <span data-testid={`tab-${i}-filename`}>{tab.fileName}</span>
          <span data-testid={`tab-${i}-filepath`}>{tab.filePath}</span>
          <span data-testid={`tab-${i}-selected-node`}>
            {tab.selectedNodeId ?? 'null'}
          </span>
          <span data-testid={`tab-${i}-root-label`}>
            {tab.rootNode?.label ?? 'null'}
          </span>
        </div>
      ))}
      <button
        data-testid="open-doc-1"
        onClick={() =>
          dispatch({
            type: 'OPEN_DOCUMENT',
            payload: {
              tabId: 'tab-1',
              fileName: 'first.pdf',
              filePath: '/path/to/first.pdf',
              rootNode: catalogNode,
              rootChildren: childNodes,
            },
          } as AppAction)
        }
      />
      <button
        data-testid="open-doc-2"
        onClick={() =>
          dispatch({
            type: 'OPEN_DOCUMENT',
            payload: {
              tabId: 'tab-2',
              fileName: 'second.pdf',
              filePath: '/path/to/second.pdf',
              rootNode: catalogNode2,
              rootChildren: childNodes2,
            },
          } as AppAction)
        }
      />
      <button
        data-testid="open-doc-duplicate"
        onClick={() =>
          dispatch({
            type: 'OPEN_DOCUMENT',
            payload: {
              tabId: 'tab-3',
              fileName: 'first.pdf',
              filePath: '/path/to/first.pdf',
              rootNode: catalogNode,
              rootChildren: childNodes,
            },
          } as AppAction)
        }
      />
      <button
        data-testid="activate-tab-1"
        onClick={() =>
          dispatch({
            type: 'ACTIVATE_TAB',
            payload: { tabId: 'tab-1' },
          } as AppAction)
        }
      />
      <button
        data-testid="activate-tab-2"
        onClick={() =>
          dispatch({
            type: 'ACTIVATE_TAB',
            payload: { tabId: 'tab-2' },
          } as AppAction)
        }
      />
      <button
        data-testid="activate-nonexistent"
        onClick={() =>
          dispatch({
            type: 'ACTIVATE_TAB',
            payload: { tabId: 'tab-999' },
          } as AppAction)
        }
      />
      <button
        data-testid="select-node-tab1"
        onClick={() =>
          dispatch({
            type: 'SELECT_NODE',
            payload: { nodeId: 'obj:0:5', label: 'Pages', rawKey: '/Pages' },
          })
        }
      />
      <button
        data-testid="set-error"
        onClick={() =>
          dispatch({
            type: 'SET_DOCUMENT_ERROR',
            payload: { message: 'encrypted PDF' },
          })
        }
      />
      <span data-testid="document-error">
        {state.documentError ?? 'null'}
      </span>
      <button
        data-testid="close-doc-1"
        onClick={() =>
          dispatch({
            type: 'CLOSE_DOCUMENT',
            payload: { tabId: 'tab-1' },
          })
        }
      />
      <button
        data-testid="close-doc-2"
        onClick={() =>
          dispatch({
            type: 'CLOSE_DOCUMENT',
            payload: { tabId: 'tab-2' },
          })
        }
      />
    </div>
  );
}

describe('4.1 Multi-Tab Reducer Tests', () => {
  /**
   * 4.1-UNIT-001 [P0]: OPEN_DOCUMENT appends new tab to tabs array (does not replace).
   *
   * RED PHASE: Currently OPEN_DOCUMENT replaces all tabs with `tabs: [newTab]`.
   * After implementation, it must append: `tabs: [...state.tabs, newTab]`.
   */
  test('4.1-UNIT-001 [P0]: OPEN_DOCUMENT appends new tab to tabs array', () => {
    render(
      <AppProvider>
        <MultiTabInspector />
      </AppProvider>
    );

    // Open first document
    act(() => screen.getByTestId('open-doc-1').click());
    expect(screen.getByTestId('tab-count').textContent).toBe('1');
    expect(screen.getByTestId('tab-0-id').textContent).toBe('tab-1');
    expect(screen.getByTestId('tab-0-filename').textContent).toBe('first.pdf');

    // Open second document -- must APPEND, not replace
    act(() => screen.getByTestId('open-doc-2').click());
    expect(screen.getByTestId('tab-count').textContent).toBe('2');
    expect(screen.getByTestId('tab-0-id').textContent).toBe('tab-1');
    expect(screen.getByTestId('tab-0-filename').textContent).toBe('first.pdf');
    expect(screen.getByTestId('tab-1-id').textContent).toBe('tab-2');
    expect(screen.getByTestId('tab-1-filename').textContent).toBe('second.pdf');
  });

  /**
   * 4.1-UNIT-002 [P0]: OPEN_DOCUMENT sets activeTabId to the new tab.
   *
   * RED PHASE: This already works for single-tab mode, but must continue
   * to work when appending (activeTabId = newly opened tab).
   */
  test('4.1-UNIT-002 [P0]: OPEN_DOCUMENT sets activeTabId to the new tab', () => {
    render(
      <AppProvider>
        <MultiTabInspector />
      </AppProvider>
    );

    act(() => screen.getByTestId('open-doc-1').click());
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');

    act(() => screen.getByTestId('open-doc-2').click());
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2');
  });

  /**
   * 4.1-UNIT-003 [P0]: ACTIVATE_TAB sets activeTabId without modifying
   * any tab's internal state.
   *
   * RED PHASE: ACTIVATE_TAB action does not exist yet in AppAction union.
   * The test dispatches it via `as AppAction` cast. The reducer's exhaustive
   * switch will throw or the cast will fail at compile time once strict
   * checking is enforced.
   */
  test('4.1-UNIT-003 [P0]: ACTIVATE_TAB sets activeTabId without modifying tab state', () => {
    render(
      <AppProvider>
        <MultiTabInspector />
      </AppProvider>
    );

    // Open two tabs
    act(() => screen.getByTestId('open-doc-1').click());
    act(() => screen.getByTestId('open-doc-2').click());
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2');

    // Select a node while tab-2 is active
    act(() => screen.getByTestId('select-node-tab1').click());
    expect(screen.getByTestId('tab-1-selected-node').textContent).toBe('obj:0:5');

    // Activate tab-1
    act(() => screen.getByTestId('activate-tab-1').click());
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');

    // Tab-2's selected node must be unchanged (still 'obj:0:5')
    expect(screen.getByTestId('tab-1-selected-node').textContent).toBe('obj:0:5');

    // Tab-1's state must also be untouched (no selectedNodeId)
    expect(screen.getByTestId('tab-0-selected-node').textContent).toBe('null');

    // Both tabs' root data must be intact
    expect(screen.getByTestId('tab-0-root-label').textContent).toBe('Catalog');
    expect(screen.getByTestId('tab-1-root-label').textContent).toBe('Catalog 2');
  });

  /**
   * 4.1-UNIT-003 supplemental: ACTIVATE_TAB with nonexistent tabId is a no-op.
   */
  test('4.1-UNIT-003 supplemental: ACTIVATE_TAB with invalid tabId is a no-op', () => {
    render(
      <AppProvider>
        <MultiTabInspector />
      </AppProvider>
    );

    act(() => screen.getByTestId('open-doc-1').click());
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');

    // Activate a tab that does not exist -- state must not change
    act(() => screen.getByTestId('activate-nonexistent').click());
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');
    expect(screen.getByTestId('tab-count').textContent).toBe('1');
  });

  /**
   * 4.1-UNIT-010 [P2]: Opening a PDF that is already open in a tab focuses
   * that tab instead of opening a duplicate.
   *
   * RED PHASE: No duplicate detection exists. OPEN_DOCUMENT always creates
   * a new tab. After implementation, filePath-based dedup must prevent
   * duplicate tabs and focus the existing one.
   */
  test('4.1-UNIT-010 [P2]: duplicate filePath focuses existing tab instead of opening new', () => {
    render(
      <AppProvider>
        <MultiTabInspector />
      </AppProvider>
    );

    // Open first document
    act(() => screen.getByTestId('open-doc-1').click());
    expect(screen.getByTestId('tab-count').textContent).toBe('1');
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');

    // Open second document (different file)
    act(() => screen.getByTestId('open-doc-2').click());
    expect(screen.getByTestId('tab-count').textContent).toBe('2');
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2');

    // Open duplicate of first document (same filePath, different tabId)
    act(() => screen.getByTestId('open-doc-duplicate').click());

    // Should NOT create a third tab -- should focus the existing tab-1
    expect(screen.getByTestId('tab-count').textContent).toBe('2');
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');
  });

  /**
   * 4.1-UNIT-001 supplemental: filePath is stored in TabState.
   *
   * RED PHASE: TabState does not have filePath field yet.
   */
  test('4.1-UNIT-001 supplemental: OPEN_DOCUMENT stores filePath in TabState', () => {
    render(
      <AppProvider>
        <MultiTabInspector />
      </AppProvider>
    );

    act(() => screen.getByTestId('open-doc-1').click());
    expect(screen.getByTestId('tab-0-filepath').textContent).toBe(
      '/path/to/first.pdf'
    );

    act(() => screen.getByTestId('open-doc-2').click());
    expect(screen.getByTestId('tab-1-filepath').textContent).toBe(
      '/path/to/second.pdf'
    );
  });

  /**
   * Supplemental: SET_DOCUMENT_ERROR preserves existing tabs in multi-tab mode.
   *
   * The reducer was changed from clearing all tabs on error to preserving them.
   * This test ensures that opening a bad PDF as a second document does not
   * destroy the first document's tab.
   */
  test('SET_DOCUMENT_ERROR preserves existing tabs in multi-tab mode', () => {
    render(
      <AppProvider>
        <MultiTabInspector />
      </AppProvider>
    );

    // Open two documents
    act(() => screen.getByTestId('open-doc-1').click());
    act(() => screen.getByTestId('open-doc-2').click());
    expect(screen.getByTestId('tab-count').textContent).toBe('2');

    // Trigger a document error (e.g., third open attempt fails)
    act(() => screen.getByTestId('set-error').click());

    // Both tabs must still exist
    expect(screen.getByTestId('tab-count').textContent).toBe('2');
    expect(screen.getByTestId('tab-0-id').textContent).toBe('tab-1');
    expect(screen.getByTestId('tab-1-id').textContent).toBe('tab-2');
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2');
    expect(screen.getByTestId('document-error').textContent).toBe('encrypted PDF');
  });

  /**
   * Supplemental: CLOSE_DOCUMENT on active middle tab focuses the next adjacent tab.
   *
   * When closing the active tab that is not the last in the array, the reducer
   * should activate the tab to the right (next), not the last tab in the array.
   */
  test('CLOSE_DOCUMENT on active middle tab focuses the next adjacent tab', () => {
    render(
      <AppProvider>
        <MultiTabInspector />
      </AppProvider>
    );

    // Open two tabs: tab-1, tab-2
    act(() => screen.getByTestId('open-doc-1').click());
    act(() => screen.getByTestId('open-doc-2').click());

    // Activate tab-1 (the first/middle-ish tab)
    act(() => screen.getByTestId('activate-tab-1').click());
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');

    // Close tab-1 (the active tab, at index 0)
    act(() => screen.getByTestId('close-doc-1').click());

    // tab-2 should become active (the next tab in the array)
    expect(screen.getByTestId('tab-count').textContent).toBe('1');
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2');
  });

  /**
   * Supplemental: CLOSE_DOCUMENT on non-active tab does not change activeTabId.
   */
  test('CLOSE_DOCUMENT on non-active tab does not change activeTabId', () => {
    render(
      <AppProvider>
        <MultiTabInspector />
      </AppProvider>
    );

    // Open two tabs: tab-1, tab-2. tab-2 is active.
    act(() => screen.getByTestId('open-doc-1').click());
    act(() => screen.getByTestId('open-doc-2').click());
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2');

    // Close tab-1 (non-active)
    act(() => screen.getByTestId('close-doc-1').click());

    expect(screen.getByTestId('tab-count').textContent).toBe('1');
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2');
    expect(screen.getByTestId('tab-0-id').textContent).toBe('tab-2');
  });
});
