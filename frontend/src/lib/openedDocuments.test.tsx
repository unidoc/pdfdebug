/**
 * The open and close paths in lib/openedDocuments: a repeated delivery of one
 * open leaves its tab untouched and closes nothing, a deliberate re-open of a
 * path replaces its tab and closes the earlier backend document, and closing
 * a tab forgets it so no later open closes it a second time.
 *
 * Run: cd frontend && npx vitest run src/lib/openedDocuments.test.tsx
 */
import { render, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import type { Dispatch } from 'react';
import { AppProvider, useAppDispatch, useAppState, type AppAction, type AppState } from '../hooks/useDocumentState';
import { closeDocumentTab, dispatchOpenedDocument, type OpenedDocument } from './openedDocuments';

const mockCloseDocument = vi.fn();

vi.mock('../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  CloseDocument: (...args: unknown[]) => mockCloseDocument(...args),
}));

let state: AppState;
let dispatch: Dispatch<AppAction>;

function Probe() {
  state = useAppState();
  dispatch = useAppDispatch();
  return null;
}

function doc(tabId: string, filePath: string): OpenedDocument {
  return { tabId, fileName: filePath.replace(/^.*\//, ''), filePath, pageCount: 1, rootNode: null, rootChildren: null };
}

function open(d: OpenedDocument) {
  act(() => dispatchOpenedDocument(dispatch, state.tabs, d));
}

beforeEach(() => {
  mockCloseDocument.mockReset().mockResolvedValue(undefined);
  render(
    <AppProvider>
      <Probe />
    </AppProvider>,
  );
});

describe('repeated delivery of one open', () => {
  test('keeps the tab selection and history and closes no document', () => {
    open(doc('t-a', '/repeat/a.pdf'));
    act(() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:5', label: 'Five' } }));
    open(doc('t-b', '/repeat/b.pdf'));
    expect(state.activeTabId).toBe('t-b');

    open(doc('t-a', '/repeat/a.pdf'));

    expect(mockCloseDocument).not.toHaveBeenCalled();
    expect(state.tabs.map((t) => t.tabId)).toEqual(['t-a', 't-b']);
    expect(state.activeTabId).toBe('t-a');
    const tab = state.tabs.find((t) => t.tabId === 't-a')!;
    expect(tab.selectedNodeId).toBe('obj:0:5');
    expect(tab.navHistory).toHaveLength(1);
  });

  test('is recognised from the session record before its tab renders', () => {
    act(() => {
      dispatchOpenedDocument(dispatch, [], doc('t-c', '/repeat/c.pdf'));
      dispatchOpenedDocument(dispatch, [], doc('t-c', '/repeat/c.pdf'));
    });
    expect(mockCloseDocument).not.toHaveBeenCalled();
    expect(state.tabs.map((t) => t.tabId)).toEqual(['t-c']);
  });

  test('a deliberate re-open of the path still replaces the tab in place', () => {
    open(doc('t-d1', '/repeat/d.pdf'));
    act(() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:5' } }));
    open(doc('t-d2', '/repeat/d.pdf'));
    expect(mockCloseDocument).toHaveBeenCalledWith('t-d1');
    expect(state.tabs.map((t) => t.tabId)).toEqual(['t-d2']);
    expect(state.tabs[0].selectedNodeId).toBeNull();
  });
});

describe('closing a tab', () => {
  test('closes its document once, and a later open of the path does not close it again', () => {
    open(doc('t-e1', '/close/e.pdf'));
    act(() => closeDocumentTab(dispatch, 't-e1'));
    expect(state.tabs).toHaveLength(0);
    expect(mockCloseDocument).toHaveBeenCalledTimes(1);
    expect(mockCloseDocument).toHaveBeenCalledWith('t-e1');

    open(doc('t-e2', '/close/e.pdf'));
    expect(mockCloseDocument).toHaveBeenCalledTimes(1);
    expect(state.tabs.map((t) => t.tabId)).toEqual(['t-e2']);
  });

  test('a document already gone is quiet and any other close failure is logged', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    mockCloseDocument.mockRejectedValueOnce(new Error('document not found: tab "t-f"'));
    open(doc('t-f', '/close/f.pdf'));
    await act(async () => closeDocumentTab(dispatch, 't-f'));
    expect(warn).not.toHaveBeenCalled();

    mockCloseDocument.mockRejectedValueOnce(new Error('disk on fire'));
    open(doc('t-g', '/close/g.pdf'));
    await act(async () => closeDocumentTab(dispatch, 't-g'));
    expect(warn).toHaveBeenCalledTimes(1);
    expect(String(warn.mock.calls[0][0])).toContain('disk on fire');
    warn.mockRestore();
  });
});
