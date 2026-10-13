/**
 * Left-rail state: the navigator each tab shows in the left panel, the
 * app-level collapse flag (persisted under its own localStorage key), the
 * focus request for the Pages jump field, and re-opening an open file in place.
 *
 * Run: cd frontend && npx vitest run src/hooks/useDocumentState.leftRail.test.tsx
 */
import { render, act } from '@testing-library/react';
import { describe, test, expect, beforeEach } from 'vitest';
import type { Dispatch } from 'react';
import { AppProvider, selectActiveLeftView, useAppState, useAppDispatch, type AppAction, type AppState } from './useDocumentState';

const RAIL_KEY = 'unidoc-pdf-debugger:left-rail';

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
  return render(
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

function view() {
  return selectActiveLeftView(state);
}

function tabView(tabId: string) {
  return state.tabs.find((t) => t.tabId === tabId)?.leftView;
}

function openTab(tabId: string, pageCount: number) {
  send({
    type: 'OPEN_DOCUMENT',
    payload: { tabId, fileName: `${tabId}.pdf`, filePath: `/tmp/${tabId}.pdf`, pageCount, rootNode: catalogNode, rootChildren: [] },
  });
}

beforeEach(() => {
  window.localStorage.removeItem(RAIL_KEY);
});

describe('left rail initial state', () => {
  test('starts on Structure, expanded, with no focus request', () => {
    mount();
    expect(view()).toBe('structure');
    expect(state.leftPanelCollapsed).toBe(false);
    expect(state.pagesJumpFocusVersion).toBe(0);
  });

  test('seeds the persisted collapse flag but always starts on Structure', () => {
    window.localStorage.setItem(RAIL_KEY, JSON.stringify({ view: 'pages', collapsed: true }));
    mount();
    expect(view()).toBe('structure');
    expect(state.leftPanelCollapsed).toBe(true);
  });

  test.each([
    ['not JSON', '{nope'],
    ['a JSON string', '"pages"'],
    ['null', 'null'],
    ['a non-boolean collapsed flag', JSON.stringify({ collapsed: 'yes' })],
    ['a missing collapsed flag', JSON.stringify({ view: 'pages' })],
  ])('falls back to Structure, expanded, when the stored value is %s', (_label, raw) => {
    window.localStorage.setItem(RAIL_KEY, raw);
    mount();
    expect(view()).toBe('structure');
    expect(state.leftPanelCollapsed).toBe(false);
  });
});

describe('left rail actions', () => {
  test('SELECT_LEFT_VIEW switches the view and un-collapses the panel', () => {
    mount();
    openTab('tab-1', 3);
    send({ type: 'TOGGLE_LEFT_PANEL' });
    expect(state.leftPanelCollapsed).toBe(true);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    expect(view()).toBe('pages');
    expect(state.leftPanelCollapsed).toBe(false);
  });

  test('TOGGLE_LEFT_PANEL flips the collapse flag and keeps the view', () => {
    mount();
    openTab('tab-1', 3);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    send({ type: 'TOGGLE_LEFT_PANEL' });
    expect(state.leftPanelCollapsed).toBe(true);
    expect(view()).toBe('pages');
    send({ type: 'TOGGLE_LEFT_PANEL' });
    expect(state.leftPanelCollapsed).toBe(false);
  });

  test('the collapse flag is app-level: switching tabs does not change it', () => {
    mount();
    openTab('tab-1', 3);
    openTab('tab-2', 3);
    send({ type: 'TOGGLE_LEFT_PANEL' });
    send({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } });
    expect(state.leftPanelCollapsed).toBe(true);
  });

  test('with no active tab the view is Structure and SELECT_LEFT_VIEW only un-collapses', () => {
    mount();
    expect(state.activeTabId).toBeNull();
    send({ type: 'TOGGLE_LEFT_PANEL' });
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'images' } });
    expect(view()).toBe('structure');
    expect(state.leftPanelCollapsed).toBe(false);
  });
});

describe('the rail view is per tab', () => {
  test('each tab keeps its own view across tab switches', () => {
    mount();
    openTab('a', 2);
    openTab('b', 3);
    send({ type: 'ACTIVATE_TAB', payload: { tabId: 'a' } });
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'images' } });
    expect(view()).toBe('images');
    send({ type: 'ACTIVATE_TAB', payload: { tabId: 'b' } });
    expect(view()).toBe('structure');
    send({ type: 'ACTIVATE_TAB', payload: { tabId: 'a' } });
    expect(view()).toBe('images');
  });

  test('selecting a view on one tab never changes another tab', () => {
    mount();
    openTab('a', 2);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'images' } });
    openTab('b', 3);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    expect(tabView('a')).toBe('images');
    expect(tabView('b')).toBe('pages');
  });

  test('FOCUS_PAGES_JUMP changes only the active tab', () => {
    mount();
    openTab('a', 2);
    openTab('b', 3);
    send({ type: 'FOCUS_PAGES_JUMP' });
    expect(tabView('b')).toBe('pages');
    expect(tabView('a')).toBe('structure');
  });

  test('NAVIGATE_TO_REF changes only the tab it targets', () => {
    mount();
    openTab('a', 2);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    openTab('b', 3);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'images' } });
    send({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: 'obj:0:3' } });
    expect(tabView('b')).toBe('structure');
    expect(tabView('a')).toBe('pages');
  });
});

describe('FOCUS_PAGES_JUMP', () => {
  test('is a no-op without an active tab', () => {
    mount();
    send({ type: 'TOGGLE_LEFT_PANEL' });
    send({ type: 'FOCUS_PAGES_JUMP' });
    expect(view()).toBe('structure');
    expect(state.leftPanelCollapsed).toBe(true);
    expect(state.pagesJumpFocusVersion).toBe(0);
  });

  test('selects Pages, un-collapses and bumps the focus version', () => {
    mount();
    openTab('tab-1', 3);
    send({ type: 'TOGGLE_LEFT_PANEL' });
    send({ type: 'FOCUS_PAGES_JUMP' });
    expect(view()).toBe('pages');
    expect(state.leftPanelCollapsed).toBe(false);
    expect(state.pagesJumpFocusVersion).toBe(1);
    send({ type: 'FOCUS_PAGES_JUMP' });
    expect(state.pagesJumpFocusVersion).toBe(2);
  });

  test('works on a tab whose page count is 0', () => {
    mount();
    openTab('tab-1', 0);
    send({ type: 'FOCUS_PAGES_JUMP' });
    expect(view()).toBe('pages');
    expect(state.pagesJumpFocusVersion).toBe(1);
  });
});

describe('NAVIGATE_TO_REF makes the reveal visible', () => {
  test('switches the view to Structure and un-collapses the panel', () => {
    mount();
    openTab('tab-1', 3);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    send({ type: 'TOGGLE_LEFT_PANEL' });
    send({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: 'obj:0:3' } });
    expect(view()).toBe('structure');
    expect(state.leftPanelCollapsed).toBe(false);
    expect(state.tabs[0].pendingNavTarget).toBe('obj:0:3');
  });

  test('Back and Forward do not switch the view', () => {
    mount();
    openTab('tab-1', 3);
    send({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:3', label: 'Page' } });
    send({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:4', label: 'Page' } });
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    send({ type: 'NAVIGATE_BACK' });
    expect(state.tabs[0].selectedNodeId).toBe('obj:0:3');
    expect(view()).toBe('pages');
    send({ type: 'NAVIGATE_FORWARD' });
    expect(view()).toBe('pages');
  });
});

describe('the retired Go to Page modal state', () => {
  test('there is no modal open flag any more', () => {
    mount();
    expect('goToPageOpen' in state).toBe(false);
  });
});

describe('opening a file', () => {
  test('a new tab starts on Structure, the previous tab keeps its view, the collapse flag stays', () => {
    mount();
    openTab('a', 2);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'images' } });
    send({ type: 'TOGGLE_LEFT_PANEL' });
    openTab('b', 3);
    expect(state.activeTabId).toBe('b');
    expect(view()).toBe('structure');
    expect(tabView('a')).toBe('images');
    expect(state.leftPanelCollapsed).toBe(true);
  });

  test('re-opening an open file replaces that tab in place and keeps its view', () => {
    mount();
    openTab('a', 2);
    openTab('b', 3);
    send({ type: 'ACTIVATE_TAB', payload: { tabId: 'a' } });
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'images' } });
    send({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:3', label: 'Page' } });
    send({ type: 'PUSH_RECENT_JUMP', payload: { tabId: 'a', entry: { objNum: 3, gen: 0, typeName: 'Page', nodeId: 'obj:0:3' } } });
    send({ type: 'SET_FIND_CASE_SENSITIVE', payload: { tabId: 'a', value: true } });
    send({ type: 'ACTIVATE_TAB', payload: { tabId: 'b' } });
    const versionBefore = state.tabActivationVersion;
    const newRoot = { ...catalogNode, id: 'root', label: 'Catalog v2' };
    send({ type: 'OPEN_DOCUMENT', payload: { tabId: 'a2', fileName: 'a-renamed.pdf', filePath: '/tmp/a.pdf', pageCount: 7, rootNode: newRoot, rootChildren: [] } });

    expect(state.tabs.map((t) => t.tabId)).toEqual(['a2', 'b']);
    expect(state.activeTabId).toBe('a2');
    expect(state.tabActivationVersion).toBe(versionBefore + 1);
    const replaced = state.tabs[0];
    expect(replaced.leftView).toBe('images');
    expect(replaced.fileName).toBe('a-renamed.pdf');
    expect(replaced.pageCount).toBe(7);
    expect(replaced.rootNode).toBe(newRoot);
    expect(replaced.selectedNodeId).toBeNull();
    expect(replaced.navHistory).toEqual([]);
    expect(replaced.navHistoryIndex).toBe(-1);
    expect(replaced.pendingNavTarget).toBeNull();
    expect(replaced.recentJumps).toEqual([]);
    expect(replaced.findCaseSensitive).toBe(true);
    expect(tabView('b')).toBe('structure');
  });

  test('re-opening an open file still counts toward a batch', () => {
    mount();
    openTab('a', 2);
    send({ type: 'BATCH_OPEN_START', payload: { total: 2 } });
    send({ type: 'OPEN_DOCUMENT', payload: { tabId: 'a2', fileName: 'a.pdf', filePath: '/tmp/a.pdf', pageCount: 2, rootNode: catalogNode, rootChildren: [] } });
    openTab('b', 3);
    expect(state.batchOpenCompleted).toBe(2);
    expect(state.tabs).toHaveLength(2);
  });
});

describe('left rail persistence', () => {
  test('writes only the collapse flag on change', () => {
    mount();
    openTab('tab-1', 3);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    expect(JSON.parse(window.localStorage.getItem(RAIL_KEY) ?? 'null')).toEqual({ collapsed: false });
    send({ type: 'TOGGLE_LEFT_PANEL' });
    expect(JSON.parse(window.localStorage.getItem(RAIL_KEY) ?? 'null')).toEqual({ collapsed: true });
  });

  test('survives a remount', () => {
    const first = mount();
    openTab('tab-1', 3);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    send({ type: 'TOGGLE_LEFT_PANEL' });
    first.unmount();
    mount();
    expect(view()).toBe('structure');
    expect(state.leftPanelCollapsed).toBe(true);
  });

  test('a storage that throws falls back to the defaults and does not break dispatch', () => {
    const original = window.localStorage;
    const throwing = {
      getItem: () => { throw new Error('blocked'); },
      setItem: () => { throw new Error('blocked'); },
      removeItem: () => {},
      clear: () => {},
      key: () => null,
      length: 0,
    } as Storage;
    Object.defineProperty(window, 'localStorage', { value: throwing, configurable: true, writable: true });
    try {
      mount();
      expect(view()).toBe('structure');
      openTab('tab-1', 3);
      send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
      expect(view()).toBe('pages');
    } finally {
      Object.defineProperty(window, 'localStorage', { value: original, configurable: true, writable: true });
    }
  });
});
