/**
 * BatchOpenDialog appears while a multi-PDF drop is in flight (batchOpenTotal
 * > 0) and disappears on BATCH_OPEN_COMPLETE. Cancel button dispatches
 * BATCH_OPEN_CANCEL and emits document:batch-cancel for the Go-side loop.
 *
 * Progress is driven by OPEN_DOCUMENT itself (atomic with tab-add) so the
 * count in the toast can never lag behind the actual number of opened tabs.
 */
import { render, screen, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { AppProvider, useAppDispatch, useAppState } from '../hooks/useDocumentState';
import { BatchOpenDialog } from './BatchOpenDialog';

// Mock Wails runtime -- the Cancel button calls Events.Emit.
const mockEmit = vi.fn();
vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn(() => vi.fn()),
    Emit: (...args: unknown[]) => mockEmit(...args),
  },
}));

function openDocAction(n: number) {
  return {
    type: 'OPEN_DOCUMENT' as const,
    payload: {
      tabId: `t-${n}`,
      fileName: `f${n}.pdf`,
      filePath: `/f${n}.pdf`,
      pageCount: 1,
      rootNode: null,
      rootChildren: null,
    },
  };
}

function Bootstrap() {
  const dispatch = useAppDispatch();
  const { batchOpenCancelled, documentWarning } = useAppState();
  return (
    <div>
      <button data-testid="start-2" onClick={() => dispatch({ type: 'BATCH_OPEN_START', payload: { total: 2 } })}>s2</button>
      <button data-testid="start-3" onClick={() => dispatch({ type: 'BATCH_OPEN_START', payload: { total: 3 } })}>s3</button>
      <button data-testid="open-1" onClick={() => dispatch(openDocAction(1))}>o1</button>
      <button data-testid="open-2" onClick={() => dispatch(openDocAction(2))}>o2</button>
      <button data-testid="open-3" onClick={() => dispatch(openDocAction(3))}>o3</button>
      <button data-testid="done" onClick={() => dispatch({ type: 'BATCH_OPEN_COMPLETE' })}>d</button>
      <button
        data-testid="set-warning"
        onClick={() => dispatch({
          type: 'SET_DOCUMENT_WARNING',
          payload: { message: 'Per-file structural issue' },
        })}
      >w</button>
      <button data-testid="dismiss-warning" onClick={() => dispatch({ type: 'DISMISS_WARNING' })}>x</button>
      <span data-testid="cancelled-flag">{String(batchOpenCancelled)}</span>
      <span data-testid="warning-text">{documentWarning ?? ''}</span>
      <BatchOpenDialog />
    </div>
  );
}

describe('BatchOpenDialog', () => {
  beforeEach(() => {
    mockEmit.mockClear();
  });

  test('hidden initially when no batch is active', () => {
    render(<AppProvider><Bootstrap /></AppProvider>);
    expect(screen.queryByTestId('batch-open-dialog')).not.toBeInTheDocument();
  });

  test('opens on BATCH_OPEN_START, advances on OPEN_DOCUMENT, closes on COMPLETE', () => {
    render(<AppProvider><Bootstrap /></AppProvider>);

    act(() => screen.getByTestId('start-2').click());
    expect(screen.getByTestId('batch-open-status').textContent).toMatch(/Opened 0 of 2/);
    expect(screen.getByTestId('batch-open-progressbar')).toHaveAttribute('aria-valuenow', '0');

    act(() => screen.getByTestId('open-1').click());
    expect(screen.getByTestId('batch-open-status').textContent).toMatch(/Opened 1 of 2/);
    expect(screen.getByTestId('batch-open-progressbar')).toHaveAttribute('aria-valuenow', '1');

    act(() => screen.getByTestId('open-2').click());
    expect(screen.getByTestId('batch-open-status').textContent).toMatch(/Opened 2 of 2/);

    act(() => screen.getByTestId('done').click());
    expect(screen.queryByTestId('batch-open-dialog')).not.toBeInTheDocument();
  });

  test('cancel button dispatches BATCH_OPEN_CANCEL and emits document:batch-cancel', async () => {
    render(<AppProvider><Bootstrap /></AppProvider>);
    act(() => screen.getByTestId('start-2').click());

    const cancelBtn = screen.getByTestId('batch-open-cancel');
    expect(cancelBtn).toHaveTextContent('Cancel');
    expect(cancelBtn).not.toBeDisabled();
    expect(screen.getByTestId('cancelled-flag').textContent).toBe('false');

    await userEvent.click(cancelBtn);

    expect(screen.getByTestId('cancelled-flag').textContent).toBe('true');
    expect(mockEmit).toHaveBeenCalledWith('document:batch-cancel', null);

    expect(cancelBtn).toHaveTextContent('Cancelling...');
    expect(cancelBtn).toBeDisabled();
    expect(screen.getByTestId('batch-open-status').textContent).toMatch(/Cancelling/);
  });

  test('cancel toast count reflects OPEN_DOCUMENT-driven completion', async () => {
    render(<AppProvider><Bootstrap /></AppProvider>);

    act(() => screen.getByTestId('start-2').click());
    act(() => screen.getByTestId('open-1').click());
    await userEvent.click(screen.getByTestId('batch-open-cancel'));

    expect(screen.getByTestId('warning-text').textContent).toBe(
      'Loading cancelled. 1 of 2 files opened.'
    );

    act(() => screen.getByTestId('done').click());
    expect(screen.getByTestId('warning-text').textContent).toBe(
      'Loading cancelled. 1 of 2 files opened.'
    );
  });

  test('in-flight OPEN_DOCUMENT during drain refreshes the cancel toast count', async () => {
    // Cancel after 0 opens (worst case), then a drain OPEN_DOCUMENT arrives
    // (Wails goroutine race: file actually finished but the event is
    // delivered late). The toast must refresh to the new count so the user
    // sees the real total.
    render(<AppProvider><Bootstrap /></AppProvider>);
    act(() => screen.getByTestId('start-3').click());
    act(() => screen.getByTestId('open-1').click());
    await userEvent.click(screen.getByTestId('batch-open-cancel'));

    expect(screen.getByTestId('warning-text').textContent).toBe(
      'Loading cancelled. 1 of 3 files opened.'
    );

    // Late OPEN_DOCUMENT lands -- count updates.
    act(() => screen.getByTestId('open-2').click());
    expect(screen.getByTestId('warning-text').textContent).toBe(
      'Loading cancelled. 2 of 3 files opened.'
    );
  });

  test('cancel toast is suppressed when late drain ends up landing every file', async () => {
    // The user clicks Cancel but the loop was already on its last file --
    // every file ends up opened. Showing "Loading cancelled. N of N" would
    // be misleading; treat the cancel as a no-op and clear the toast.
    render(<AppProvider><Bootstrap /></AppProvider>);
    act(() => screen.getByTestId('start-2').click());
    act(() => screen.getByTestId('open-1').click());
    await userEvent.click(screen.getByTestId('batch-open-cancel'));
    expect(screen.getByTestId('warning-text').textContent).toMatch(/Loading cancelled/);

    // The last in-flight open lands -- count reaches total, toast cleared.
    act(() => screen.getByTestId('open-2').click());
    expect(screen.getByTestId('warning-text').textContent).toBe('');
    expect(screen.getByTestId('cancelled-flag').textContent).toBe('false');
  });

  test('OPEN_DOCUMENT after BATCH_OPEN_COMPLETE still refreshes the cancel toast count', async () => {
    // Specifically the user-reported race: BATCH_OPEN_COMPLETE arrives at JS
    // before the last document:opened. Total/completed must stay alive past
    // COMPLETE so the late event can still bump the displayed count.
    render(<AppProvider><Bootstrap /></AppProvider>);
    act(() => screen.getByTestId('start-3').click());
    act(() => screen.getByTestId('open-1').click());
    await userEvent.click(screen.getByTestId('batch-open-cancel'));
    act(() => screen.getByTestId('done').click()); // dialog closes early
    act(() => screen.getByTestId('open-2').click()); // late open (still under total)

    expect(screen.getByTestId('warning-text').textContent).toBe(
      'Loading cancelled. 2 of 3 files opened.'
    );
  });

  test('in-flight per-file warning during drain does not overwrite the cancel toast', async () => {
    render(<AppProvider><Bootstrap /></AppProvider>);
    act(() => screen.getByTestId('start-2').click());
    act(() => screen.getByTestId('open-1').click());
    await userEvent.click(screen.getByTestId('batch-open-cancel'));

    expect(screen.getByTestId('warning-text').textContent).toMatch(/Loading cancelled/);

    act(() => screen.getByTestId('set-warning').click());

    expect(screen.getByTestId('warning-text').textContent).toMatch(/Loading cancelled/);
  });

  test('completing a non-cancelled batch leaves documentWarning untouched', () => {
    render(<AppProvider><Bootstrap /></AppProvider>);
    act(() => screen.getByTestId('start-2').click());
    act(() => screen.getByTestId('open-1').click());
    act(() => screen.getByTestId('open-2').click());
    act(() => screen.getByTestId('done').click());

    expect(screen.getByTestId('warning-text').textContent).toBe('');
  });

  test('cancelled flag persists past BATCH_OPEN_COMPLETE so late events cannot clobber the toast', async () => {
    render(<AppProvider><Bootstrap /></AppProvider>);

    act(() => screen.getByTestId('start-2').click());
    await userEvent.click(screen.getByTestId('batch-open-cancel'));
    expect(screen.getByTestId('cancelled-flag').textContent).toBe('true');

    act(() => screen.getByTestId('done').click());
    expect(screen.getByTestId('cancelled-flag').textContent).toBe('true');

    act(() => screen.getByTestId('open-3').click());
    expect(screen.getByTestId('warning-text').textContent).toMatch(/Loading cancelled/);
  });

  test('starting a new batch clears the cancelled flag from a previous run', async () => {
    render(<AppProvider><Bootstrap /></AppProvider>);

    act(() => screen.getByTestId('start-2').click());
    await userEvent.click(screen.getByTestId('batch-open-cancel'));
    act(() => screen.getByTestId('done').click());
    expect(screen.getByTestId('cancelled-flag').textContent).toBe('true');

    act(() => screen.getByTestId('start-2').click());
    expect(screen.getByTestId('cancelled-flag').textContent).toBe('false');
    expect(screen.getByTestId('batch-open-cancel')).not.toBeDisabled();
  });

  test('dismissing the warning clears the cancelled flag', async () => {
    render(<AppProvider><Bootstrap /></AppProvider>);

    act(() => screen.getByTestId('start-2').click());
    await userEvent.click(screen.getByTestId('batch-open-cancel'));
    act(() => screen.getByTestId('done').click());
    expect(screen.getByTestId('cancelled-flag').textContent).toBe('true');
    expect(screen.getByTestId('warning-text').textContent).toMatch(/Loading cancelled/);

    act(() => screen.getByTestId('dismiss-warning').click());
    expect(screen.getByTestId('cancelled-flag').textContent).toBe('false');
    expect(screen.getByTestId('warning-text').textContent).toBe('');
  });
});
