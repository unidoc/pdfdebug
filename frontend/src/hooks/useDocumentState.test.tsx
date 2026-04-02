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

  test('SET_DOCUMENT_ERROR sets error and clears tabs/activeTabId', () => {
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
