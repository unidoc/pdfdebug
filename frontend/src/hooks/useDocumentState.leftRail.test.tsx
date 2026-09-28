/**
 * App-level left-rail state: which navigator fills the left panel, whether the
 * panel is collapsed, and the focus request for the Pages jump field. The
 * state is shared by every tab and persists under its own localStorage key.
 *
 * Run: cd frontend && npx vitest run src/hooks/useDocumentState.leftRail.test.tsx
 */
import { render, act } from '@testing-library/react';
import { describe, test, expect, beforeEach } from 'vitest';
import type { Dispatch } from 'react';
import { AppProvider, useAppState, useAppDispatch, type AppAction, type AppState } from './useDocumentState';

const RAIL_KEY = 'unidoc-pdf-debugger:left-rail';

type RailState = AppState & {
  leftView?: string;
  leftPanelCollapsed?: boolean;
  pagesJumpFocusVersion?: number;
};

let state: RailState;
let dispatch: Dispatch<AppAction>;

function Probe() {
  state = useAppState() as RailState;
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
    expect(state.leftView).toBe('structure');
    expect(state.leftPanelCollapsed).toBe(false);
    expect(state.pagesJumpFocusVersion).toBe(0);
  });

  test('seeds the persisted collapse flag but always starts on Structure', () => {
    window.localStorage.setItem(RAIL_KEY, JSON.stringify({ view: 'pages', collapsed: true }));
    mount();
    expect(state.leftView).toBe('structure');
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
    expect(state.leftView).toBe('structure');
    expect(state.leftPanelCollapsed).toBe(false);
  });
});

describe('left rail actions', () => {
  test('SELECT_LEFT_VIEW switches the view and un-collapses the panel', () => {
    mount();
    send({ type: 'TOGGLE_LEFT_PANEL' });
    expect(state.leftPanelCollapsed).toBe(true);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    expect(state.leftView).toBe('pages');
    expect(state.leftPanelCollapsed).toBe(false);
  });

  test('TOGGLE_LEFT_PANEL flips the collapse flag and keeps the view', () => {
    mount();
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    send({ type: 'TOGGLE_LEFT_PANEL' });
    expect(state.leftPanelCollapsed).toBe(true);
    expect(state.leftView).toBe('pages');
    send({ type: 'TOGGLE_LEFT_PANEL' });
    expect(state.leftPanelCollapsed).toBe(false);
  });

  test('the rail state is app-level: switching tabs does not change it', () => {
    mount();
    openTab('tab-1', 3);
    openTab('tab-2', 3);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    send({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } });
    expect(state.leftView).toBe('pages');
  });
});

describe('FOCUS_PAGES_JUMP', () => {
  test('is a no-op without an active tab', () => {
    mount();
    send({ type: 'TOGGLE_LEFT_PANEL' });
    send({ type: 'FOCUS_PAGES_JUMP' });
    expect(state.leftView).toBe('structure');
    expect(state.leftPanelCollapsed).toBe(true);
    expect(state.pagesJumpFocusVersion).toBe(0);
  });

  test('selects Pages, un-collapses and bumps the focus version', () => {
    mount();
    openTab('tab-1', 3);
    send({ type: 'TOGGLE_LEFT_PANEL' });
    send({ type: 'FOCUS_PAGES_JUMP' });
    expect(state.leftView).toBe('pages');
    expect(state.leftPanelCollapsed).toBe(false);
    expect(state.pagesJumpFocusVersion).toBe(1);
    send({ type: 'FOCUS_PAGES_JUMP' });
    expect(state.pagesJumpFocusVersion).toBe(2);
  });

  test('works on a tab whose page count is 0', () => {
    mount();
    openTab('tab-1', 0);
    send({ type: 'FOCUS_PAGES_JUMP' });
    expect(state.leftView).toBe('pages');
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
    expect(state.leftView).toBe('structure');
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
    expect(state.leftView).toBe('pages');
    send({ type: 'NAVIGATE_FORWARD' });
    expect(state.leftView).toBe('pages');
  });
});

describe('the retired Go to Page modal state', () => {
  test('there is no modal open flag any more', () => {
    mount();
    expect('goToPageOpen' in state).toBe(false);
  });
});

describe('opening a file starts on Structure', () => {
  test('a new tab switches the view back to Structure and keeps the collapse flag', () => {
    mount();
    openTab('a', 2);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    send({ type: 'TOGGLE_LEFT_PANEL' });
    openTab('b', 3);
    expect(state.activeTabId).toBe('b');
    expect(state.leftView).toBe('structure');
    expect(state.leftPanelCollapsed).toBe(true);
  });

  test('re-opening an already open file switches the view back to Structure', () => {
    mount();
    openTab('a', 2);
    openTab('b', 3);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    send({ type: 'OPEN_DOCUMENT', payload: { tabId: 'dup', fileName: 'a.pdf', filePath: '/tmp/a.pdf', pageCount: 2, rootNode: catalogNode, rootChildren: [] } });
    expect(state.activeTabId).toBe('a');
    expect(state.leftView).toBe('structure');
  });

  test('switching tabs keeps the current view', () => {
    mount();
    openTab('a', 2);
    openTab('b', 3);
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    send({ type: 'ACTIVATE_TAB', payload: { tabId: 'a' } });
    expect(state.leftView).toBe('pages');
  });
});

describe('left rail persistence', () => {
  test('writes only the collapse flag on change', () => {
    mount();
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    expect(JSON.parse(window.localStorage.getItem(RAIL_KEY) ?? 'null')).toEqual({ collapsed: false });
    send({ type: 'TOGGLE_LEFT_PANEL' });
    expect(JSON.parse(window.localStorage.getItem(RAIL_KEY) ?? 'null')).toEqual({ collapsed: true });
  });

  test('survives a remount', () => {
    const first = mount();
    send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
    send({ type: 'TOGGLE_LEFT_PANEL' });
    first.unmount();
    mount();
    expect(state.leftView).toBe('structure');
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
      expect(state.leftView).toBe('structure');
      send({ type: 'SELECT_LEFT_VIEW', payload: { view: 'pages' } });
      expect(state.leftView).toBe('pages');
    } finally {
      Object.defineProperty(window, 'localStorage', { value: original, configurable: true, writable: true });
    }
  });
});
