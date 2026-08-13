/**
 * GoToPageDialog validates input, dispatches NAVIGATE_TO_REF on success,
 * surfaces backend errors inline, and closes on Cancel/Escape.
 *
 * Mocks the Wails binding via vi.mock so the test exercises the dialog's
 * validation, async flow, and dispatch wiring without a Go runtime.
 */
import { render, screen, act, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { AppProvider, useAppDispatch, type AppAction, useAppState } from '../hooks/useDocumentState';
import { GoToPageDialog } from './GoToPageDialog';

// Hoist the mock so all imports of usePDFService resolve to it.
const mockGoToPage = vi.hoisted(() => vi.fn<(tabId: string, n: number) => Promise<string>>());
vi.mock('../hooks/usePDFService', () => ({
  goToPage: mockGoToPage,
}));

const catalog = {
  id: 'root', label: 'Catalog', rawKey: '', nodeType: 'dict', valueType: '',
  hasChildren: true, childCount: 0, iconHint: 'catalog', error: '',
};

const openWith = (pageCount: number): AppAction => ({
  type: 'OPEN_DOCUMENT',
  payload: { tabId: 't1', fileName: 'a.pdf', filePath: '/a.pdf', pageCount, rootNode: catalog, rootChildren: [] },
});

function NavTargetProbe() {
  const state = useAppState();
  const tab = state.tabs.find((t) => t.tabId === state.activeTabId) ?? null;
  return <span data-testid="pending-nav-target">{tab?.pendingNavTarget ?? 'null'}</span>;
}

function Bootstrap({ pageCount }: { pageCount: number }) {
  const dispatch = useAppDispatch();
  return (
    <div>
      <button data-testid="bootstrap-open" onClick={() => {
        dispatch(openWith(pageCount));
        dispatch({ type: 'OPEN_GO_TO_PAGE' });
      }}>open</button>
      <NavTargetProbe />
      <GoToPageDialog />
    </div>
  );
}

beforeEach(() => {
  mockGoToPage.mockReset();
});

describe('GoToPageDialog', () => {
  test('renders dialog with the expected page-range label after open', () => {
    render(
      <AppProvider>
        <Bootstrap pageCount={42} />
      </AppProvider>
    );
    act(() => screen.getByTestId('bootstrap-open').click());

    expect(screen.getByTestId('go-to-page-dialog')).toBeInTheDocument();
    expect(screen.getByText(/Page number \(1-42\)/)).toBeInTheDocument();
  });

  test('rejects out-of-range input inline without calling the backend', async () => {
    const user = userEvent.setup();
    render(
      <AppProvider>
        <Bootstrap pageCount={5} />
      </AppProvider>
    );
    act(() => screen.getByTestId('bootstrap-open').click());

    const input = screen.getByTestId('go-to-page-input');
    await user.type(input, '99');
    await user.click(screen.getByTestId('go-to-page-submit'));

    expect(screen.getByTestId('go-to-page-error').textContent).toMatch(/out of range \(1-5\)/);
    expect(mockGoToPage).not.toHaveBeenCalled();
  });

  test('rejects non-integer input inline without calling the backend', async () => {
    const user = userEvent.setup();
    render(
      <AppProvider>
        <Bootstrap pageCount={10} />
      </AppProvider>
    );
    act(() => screen.getByTestId('bootstrap-open').click());

    const input = screen.getByTestId('go-to-page-input');
    // type=number filters most non-numeric, but explicit value-set covers paste paths.
    await user.click(input);
    await user.keyboard('1.5');
    await user.click(screen.getByTestId('go-to-page-submit'));

    // Either "must be an integer" (preferred) or "out of range" (acceptable
    // when the input strips the dot client-side and "15" lands above range).
    expect(screen.getByTestId('go-to-page-error').textContent).not.toBe('');
    expect(mockGoToPage).not.toHaveBeenCalled();
  });

  test('dispatches NAVIGATE_TO_REF with resolved nodeId and closes on success', async () => {
    mockGoToPage.mockResolvedValue('obj:0:42');
    const user = userEvent.setup();
    render(
      <AppProvider>
        <Bootstrap pageCount={50} />
      </AppProvider>
    );
    act(() => screen.getByTestId('bootstrap-open').click());

    await user.type(screen.getByTestId('go-to-page-input'), '7');
    await user.click(screen.getByTestId('go-to-page-submit'));

    await waitFor(() => expect(mockGoToPage).toHaveBeenCalledWith('t1', 7));
    await waitFor(() => expect(screen.queryByTestId('go-to-page-dialog')).not.toBeInTheDocument());
    expect(screen.getByTestId('pending-nav-target').textContent).toBe('obj:0:42');
  });

  test('surfaces backend error inline and keeps dialog open', async () => {
    mockGoToPage.mockRejectedValue(new Error('Page 7 has no content stream.'));
    const user = userEvent.setup();
    render(
      <AppProvider>
        <Bootstrap pageCount={50} />
      </AppProvider>
    );
    act(() => screen.getByTestId('bootstrap-open').click());

    await user.type(screen.getByTestId('go-to-page-input'), '7');
    await user.click(screen.getByTestId('go-to-page-submit'));

    await waitFor(() => expect(screen.getByTestId('go-to-page-error').textContent).toMatch(/no content stream/));
    expect(screen.getByTestId('go-to-page-dialog')).toBeInTheDocument();
    expect(screen.getByTestId('pending-nav-target').textContent).toBe('null');
  });

  test('Escape closes the dialog without dispatching navigation', async () => {
    const user = userEvent.setup();
    render(
      <AppProvider>
        <Bootstrap pageCount={5} />
      </AppProvider>
    );
    act(() => screen.getByTestId('bootstrap-open').click());

    await user.keyboard('{Escape}');

    await waitFor(() => expect(screen.queryByTestId('go-to-page-dialog')).not.toBeInTheDocument());
    expect(mockGoToPage).not.toHaveBeenCalled();
    expect(screen.getByTestId('pending-nav-target').textContent).toBe('null');
  });
});
