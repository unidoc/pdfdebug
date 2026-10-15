/**
 * Reload through OPEN_DOCUMENT with `reload: true`: the tab is replaced in
 * place exactly like re-opening an open file (fresh content, selection and
 * history cleared, rail view and find case kept), and a reload never touches
 * the batch-open counters or the batch warning.
 *
 * Run: cd frontend && npx vitest run src/hooks/useDocumentState.reload.test.tsx
 */
import { render, act } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import type { Dispatch } from 'react';
import { AppProvider, useAppState, useAppDispatch, type AppAction, type AppState } from './useDocumentState';

let state: AppState;
let dispatch: Dispatch<AppAction>;

function Probe() {
  state = useAppState();
  dispatch = useAppDispatch();
  return null;
}

function send(action: Record<string, unknown>) {
  act(() => dispatch(action as unknown as AppAction));
}

function mount() {
  render(
    <AppProvider>
      <Probe />
    </AppProvider>,
  );
}

const catalogNode = {
  id: 'root',
  label: 'Catalog',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 1,
  iconHint: 'catalog',
  error: '',
};

const pagesNode = { ...catalogNode, id: 'obj:0:2', label: 'Pages', iconHint: 'pages' };

function open(tabId: string, filePath: string, extra: Record<string, unknown> = {}) {
  send({
    type: 'OPEN_DOCUMENT',
    payload: {
      tabId,
      fileName: filePath.split('/').pop(),
      filePath,
      pageCount: 1,
      rootNode: catalogNode,
      rootChildren: [],
      ...extra,
    },
  });
}

function reload(newTabId: string, filePath: string, pageCount = 9) {
  open(newTabId, filePath, { reload: true, pageCount, rootChildren: [pagesNode] });
}

function tab(tabId: string) {
  return state.tabs.find((t) => t.tabId === tabId);
}

describe('reload replaces the tab like a new open', () => {
  test('new tab id at the same strip position with fresh content', () => {
    mount();
    open('a1', '/docs/a.pdf');
    open('b1', '/docs/b.pdf');
    send({ type: 'ACTIVATE_TAB', payload: { tabId: 'a1' } });

    reload('a2', '/docs/a.pdf', 9);

    expect(state.tabs.map((t) => t.tabId)).toEqual(['a2', 'b1']);
    expect(state.activeTabId).toBe('a2');
    expect(tab('a2')?.pageCount).toBe(9);
    expect(tab('a2')?.rootChildren).toEqual([pagesNode]);
  });

  test('selection, history, pending nav and recent jumps are cleared; rail view and find case are kept', () => {
    mount();
    open('a1', '/docs/a.pdf');
    send({ type: 'SET_FIND_CASE_SENSITIVE', payload: { tabId: 'a1', value: true } });
    send({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:2', label: 'Pages' } });
    send({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:3', label: 'Page' } });
    send({ type: 'PUSH_RECENT_JUMP', payload: { tabId: 'a1', entry: { objNum: 3, gen: 0, typeName: 'Page', nodeId: 'obj:0:3' } } });
    send({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: 'obj:0:4' } });
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    expect(tab('a1')?.pendingNavTarget).toBe('obj:0:4');

    reload('a2', '/docs/a.pdf');

    const t = tab('a2');
    expect(t?.leftView).toBe('pages');
    expect(t?.findCaseSensitive).toBe(true);
    expect(t?.selectedNodeId).toBeNull();
    expect(t?.navHistory).toEqual([]);
    expect(t?.navHistoryIndex).toBe(-1);
    expect(t?.recentJumps).toEqual([]);
    expect(t?.pendingNavTarget).toBeNull();
    expect(t?.navError).toBeNull();
  });

  test('other tabs keep their entries and rail view', () => {
    mount();
    open('b1', '/docs/b.pdf');
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'images' } });
    send({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:7', label: 'Img' } });
    open('a1', '/docs/a.pdf');
    const before = tab('b1');

    reload('a2', '/docs/a.pdf');

    expect(tab('b1')).toEqual(before);
  });

  test('clears the document error and bumps the activation version', () => {
    mount();
    open('a1', '/docs/a.pdf');
    send({ type: 'SET_DOCUMENT_ERROR', payload: { message: 'Could not reload a.pdf: file not found' } });
    const version = state.tabActivationVersion;

    reload('a2', '/docs/a.pdf');

    expect(state.documentError).toBeNull();
    expect(state.tabActivationVersion).toBe(version + 1);
  });

  test('a reload that lands after the user switched tabs leaves the other tab active', () => {
    mount();
    open('a1', '/docs/a.pdf');
    open('b1', '/docs/b.pdf');
    const version = state.tabActivationVersion;

    reload('a2', '/docs/a.pdf');

    expect(state.activeTabId).toBe('b1');
    expect(state.tabs.map((t) => t.tabId)).toEqual(['a2', 'b1']);
    expect(state.tabActivationVersion).toBe(version);
  });

  test('a reload of the active tab keeps it active under the new id', () => {
    mount();
    open('a1', '/docs/a.pdf');
    open('b1', '/docs/b.pdf');
    send({ type: 'ACTIVATE_TAB', payload: { tabId: 'a1' } });

    reload('a2', '/docs/a.pdf');

    expect(state.activeTabId).toBe('a2');
  });
});

describe('reload leaves batch-open bookkeeping alone', () => {
  test('a reload during a batch open does not count toward the batch', () => {
    mount();
    open('a1', '/docs/a.pdf');
    send({ type: 'BATCH_OPEN_START', payload: { total: 3 } });
    open('c1', '/docs/c.pdf');
    expect(state.batchOpenCompleted).toBe(1);

    reload('a2', '/docs/a.pdf');

    expect(state.batchOpenCompleted).toBe(1);
    expect(state.batchOpenTotal).toBe(3);
    expect(state.batchOpenCancelled).toBe(false);
  });

  test('a reload keeps the cancelled-batch warning text and counters', () => {
    mount();
    open('a1', '/docs/a.pdf');
    send({ type: 'BATCH_OPEN_START', payload: { total: 4 } });
    open('c1', '/docs/c.pdf');
    send({ type: 'BATCH_OPEN_CANCEL' });
    send({ type: 'BATCH_OPEN_COMPLETE' });
    open('d1', '/docs/d.pdf');
    const warning = state.documentWarning;
    expect(warning).toBe('Loading cancelled. 2 of 4 files opened.');
    const completed = state.batchOpenCompleted;

    reload('a2', '/docs/a.pdf');

    expect(state.documentWarning).toBe(warning);
    expect(state.batchOpenCompleted).toBe(completed);
    expect(state.batchOpenCancelled).toBe(true);
  });

  test('a clean reload of the active tab clears its structural warning', () => {
    mount();
    open('a1', '/docs/a.pdf');
    send({ type: 'SET_DOCUMENT_WARNING', payload: { message: 'This PDF has structural errors. Some objects may display incorrectly.' } });

    reload('a2', '/docs/a.pdf');

    expect(state.documentWarning).toBeNull();
  });

  test('a reload of the active tab shows the warning of the new parse', () => {
    mount();
    open('a1', '/docs/a.pdf');

    open('a2', '/docs/a.pdf', { reload: true, warning: 'This PDF has structural errors. Some objects may display incorrectly.' });

    expect(state.documentWarning).toBe('This PDF has structural errors. Some objects may display incorrectly.');
  });

  test('a reload in the background keeps the active tab\'s warning', () => {
    mount();
    open('a1', '/docs/a.pdf');
    open('b1', '/docs/b.pdf');
    send({ type: 'SET_DOCUMENT_WARNING', payload: { message: 'b has a warning' } });

    reload('a2', '/docs/a.pdf');

    expect(state.documentWarning).toBe('b has a warning');
  });

  test('a reload leaves an open in progress alone', () => {
    mount();
    open('a1', '/docs/a.pdf');
    send({ type: 'OPENING_START', payload: { fileName: 'x.pdf' } });
    expect(state.isOpening).toBe(true);

    reload('a2', '/docs/a.pdf');

    expect(state.isOpening).toBe(true);
    expect(state.openingFileName).toBe('x.pdf');
  });

  test('a failed reload sets only the error banner', () => {
    mount();
    open('a1', '/docs/a.pdf');
    send({ type: 'SET_DOCUMENT_WARNING', payload: { message: 'This PDF has structural errors. Some objects may display incorrectly.' } });
    send({ type: 'OPENING_START', payload: { fileName: 'x.pdf' } });

    send({ type: 'SET_DOCUMENT_ERROR', payload: { message: 'Could not reload a.pdf: file not found', reload: true } });

    expect(state.documentError).toBe('Could not reload a.pdf: file not found');
    expect(state.documentWarning).toBe('This PDF has structural errors. Some objects may display incorrectly.');
    expect(state.isOpening).toBe(true);
    expect(state.openingFileName).toBe('x.pdf');
  });

  test('a real open of the same file still runs batch bookkeeping', () => {
    mount();
    open('a1', '/docs/a.pdf');
    send({ type: 'BATCH_OPEN_START', payload: { total: 2 } });

    open('a2', '/docs/a.pdf');

    expect(state.batchOpenCompleted).toBe(1);
  });
});
