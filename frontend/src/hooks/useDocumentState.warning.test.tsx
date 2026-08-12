/**
 * 2.9-UNIT: useDocumentState warning reducer tests.
 * TDD RED PHASE -- these tests MUST fail until Story 2-9 is implemented.
 *
 * Tests SET_DOCUMENT_WARNING, DISMISS_WARNING actions and documentWarning state.
 */
import { render, screen, act } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import { AppProvider, useAppState, useAppDispatch, type AppAction } from './useDocumentState';

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

function WarningInspector() {
  const state = useAppState();
  const dispatch = useAppDispatch();
  return (
    <div>
      <span data-testid="document-warning">{state.documentWarning ?? 'null'}</span>
      <span data-testid="tab-count">{state.tabs.length}</span>
      <span data-testid="active-tab-id">{state.activeTabId ?? 'null'}</span>
      <button
        data-testid="set-warning"
        onClick={() =>
          dispatch({
            type: 'SET_DOCUMENT_WARNING',
            payload: { message: 'This PDF has structural errors.' },
          })
        }
      />
      <button
        data-testid="dismiss-warning"
        onClick={() => dispatch({ type: 'DISMISS_WARNING' })}
      />
      <button
        data-testid="open-doc"
        onClick={() =>
          dispatch({
            type: 'OPEN_DOCUMENT',
            payload: {
              tabId: 'tab-1',
              fileName: 'test.pdf',
              filePath: '/path/to/test.pdf',
              rootNode: catalogNode,
              rootChildren: childNodes,
            },
          })
        }
      />
      <button
        data-testid="set-error"
        onClick={() =>
          dispatch({
            type: 'SET_DOCUMENT_ERROR',
            payload: { message: 'fatal error' },
          })
        }
      />
      <button
        data-testid="close-doc"
        onClick={() =>
          dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-1' } })
        }
      />
    </div>
  );
}

describe('appReducer warning state', () => {
  test('SET_DOCUMENT_WARNING sets documentWarning on state', () => {
    render(
      <AppProvider>
        <WarningInspector />
      </AppProvider>
    );

    expect(screen.getByTestId('document-warning').textContent).toBe('null');

    act(() => screen.getByTestId('set-warning').click());

    expect(screen.getByTestId('document-warning').textContent).toBe(
      'This PDF has structural errors.'
    );
  });

  test('SET_DOCUMENT_WARNING does NOT wipe tabs or activeTabId', () => {
    render(
      <AppProvider>
        <WarningInspector />
      </AppProvider>
    );

    // Open a document first
    act(() => screen.getByTestId('open-doc').click());
    expect(screen.getByTestId('tab-count').textContent).toBe('1');
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');

    // Set warning -- tabs must survive
    act(() => screen.getByTestId('set-warning').click());

    expect(screen.getByTestId('document-warning').textContent).toBe(
      'This PDF has structural errors.'
    );
    expect(screen.getByTestId('tab-count').textContent).toBe('1');
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');
  });

  test('DISMISS_WARNING clears documentWarning', () => {
    render(
      <AppProvider>
        <WarningInspector />
      </AppProvider>
    );

    act(() => screen.getByTestId('set-warning').click());
    expect(screen.getByTestId('document-warning').textContent).toBe(
      'This PDF has structural errors.'
    );

    act(() => screen.getByTestId('dismiss-warning').click());
    expect(screen.getByTestId('document-warning').textContent).toBe('null');
  });

  test('OPEN_DOCUMENT clears previous documentWarning', () => {
    render(
      <AppProvider>
        <WarningInspector />
      </AppProvider>
    );

    act(() => screen.getByTestId('set-warning').click());
    expect(screen.getByTestId('document-warning').textContent).toBe(
      'This PDF has structural errors.'
    );

    act(() => screen.getByTestId('open-doc').click());
    expect(screen.getByTestId('document-warning').textContent).toBe('null');
  });

  test('SET_DOCUMENT_ERROR clears documentWarning', () => {
    render(
      <AppProvider>
        <WarningInspector />
      </AppProvider>
    );

    // Open a doc then set a warning
    act(() => screen.getByTestId('open-doc').click());
    act(() => screen.getByTestId('set-warning').click());
    expect(screen.getByTestId('document-warning').textContent).toBe(
      'This PDF has structural errors.'
    );

    // Fatal error should clear warning
    act(() => screen.getByTestId('set-error').click());
    expect(screen.getByTestId('document-warning').textContent).toBe('null');
  });

  test('CLOSE_DOCUMENT clears documentWarning', () => {
    render(
      <AppProvider>
        <WarningInspector />
      </AppProvider>
    );

    // Open a doc then set a warning
    act(() => screen.getByTestId('open-doc').click());
    act(() => screen.getByTestId('set-warning').click());
    expect(screen.getByTestId('document-warning').textContent).toBe(
      'This PDF has structural errors.'
    );

    // Closing the document should clear warning
    act(() => screen.getByTestId('close-doc').click());
    expect(screen.getByTestId('document-warning').textContent).toBe('null');
  });
});
