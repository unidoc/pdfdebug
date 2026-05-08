/**
 * BatchOpenDialog appears while a multi-PDF drop is in flight (batchOpenTotal
 * > 0) and disappears on BATCH_OPEN_COMPLETE. Cancel button dispatches
 * BATCH_OPEN_CANCEL and emits document:batch-cancel for the Go-side loop.
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

function Bootstrap() {
  const dispatch = useAppDispatch();
  const { batchOpenCancelled, documentWarning } = useAppState();
  return (
    <div>
      <button data-testid="start-2" onClick={() => dispatch({ type: 'BATCH_OPEN_START', payload: { total: 2 } })}>s</button>
      <button data-testid="prog-1" onClick={() => dispatch({ type: 'BATCH_OPEN_PROGRESS', payload: { completed: 1 } })}>p1</button>
      <button data-testid="prog-2" onClick={() => dispatch({ type: 'BATCH_OPEN_PROGRESS', payload: { completed: 2 } })}>p2</button>
      <button data-testid="done" onClick={() => dispatch({ type: 'BATCH_OPEN_COMPLETE' })}>d</button>
      <button
        data-testid="open-doc"
        onClick={() => dispatch({
          type: 'OPEN_DOCUMENT',
          payload: {
            tabId: 't-drain', fileName: 'drain.pdf', filePath: '/drain.pdf',
            pageCount: 1, rootNode: null, rootChildren: null,
          },
        })}
      >o</button>
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

  test('opens on BATCH_OPEN_START, advances on PROGRESS, closes on COMPLETE', () => {
    render(<AppProvider><Bootstrap /></AppProvider>);

    act(() => screen.getByTestId('start-2').click());
    const status = screen.getByTestId('batch-open-status');
    expect(status.textContent).toMatch(/Opened 0 of 2/);
    expect(screen.getByTestId('batch-open-progressbar')).toHaveAttribute('aria-valuenow', '0');

    act(() => screen.getByTestId('prog-1').click());
    expect(screen.getByTestId('batch-open-status').textContent).toMatch(/Opened 1 of 2/);
    expect(screen.getByTestId('batch-open-progressbar')).toHaveAttribute('aria-valuenow', '1');

    act(() => screen.getByTestId('prog-2').click());
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

    // State flag flipped + Wails event emitted for the Go-side loop.
    expect(screen.getByTestId('cancelled-flag').textContent).toBe('true');
    expect(mockEmit).toHaveBeenCalledWith('document:batch-cancel', null);

    // Button label switches to "Cancelling..." and is disabled.
    expect(cancelBtn).toHaveTextContent('Cancelling...');
    expect(cancelBtn).toBeDisabled();

    // Status copy reflects the cancelling state.
    expect(screen.getByTestId('batch-open-status').textContent).toMatch(/Cancelling/);
  });

  test('cancel button surfaces the warning toast immediately on click', async () => {
    render(<AppProvider><Bootstrap /></AppProvider>);

    act(() => screen.getByTestId('start-2').click());
    act(() => screen.getByTestId('prog-1').click());
    await userEvent.click(screen.getByTestId('batch-open-cancel'));

    // Warning toast must be set on the click itself, not on a later
    // BATCH_OPEN_COMPLETE -- otherwise a fast Go-side loop that completes
    // before the cancel signal arrives would never trigger the toast.
    expect(screen.getByTestId('warning-text').textContent).toBe(
      'Loading cancelled. 1 of 2 files opened.'
    );

    // Subsequent BATCH_OPEN_COMPLETE must not clear the toast.
    act(() => screen.getByTestId('done').click());
    expect(screen.getByTestId('warning-text').textContent).toBe(
      'Loading cancelled. 1 of 2 files opened.'
    );
  });

  test('in-flight OPEN_DOCUMENT during drain preserves the cancel toast', async () => {
    // The Go-side loop has a file already in flight when the user clicks
    // Cancel. That document:opened arrives between the cancel click and the
    // eventual document:batch-complete. OPEN_DOCUMENT normally clears
    // documentWarning, but when batchOpenCancelled is true it must preserve
    // the toast so the user keeps seeing the cancellation feedback.
    render(<AppProvider><Bootstrap /></AppProvider>);
    act(() => screen.getByTestId('start-2').click());
    act(() => screen.getByTestId('prog-1').click());
    await userEvent.click(screen.getByTestId('batch-open-cancel'));

    expect(screen.getByTestId('warning-text').textContent).toMatch(/Loading cancelled/);

    // Simulate the drain: an in-flight document:opened arrives.
    act(() => screen.getByTestId('open-doc').click());

    // Toast must survive.
    expect(screen.getByTestId('warning-text').textContent).toMatch(/Loading cancelled/);
  });

  test('in-flight per-file warning during drain does not overwrite the cancel toast', async () => {
    // Each document:opened payload from Go can carry a per-file warning
    // (structural issues recovered by pdfcpu). App.jsx dispatches
    // SET_DOCUMENT_WARNING for that payload, which would replace the cancel
    // toast unless guarded by batchOpenCancelled.
    render(<AppProvider><Bootstrap /></AppProvider>);
    act(() => screen.getByTestId('start-2').click());
    act(() => screen.getByTestId('prog-1').click());
    await userEvent.click(screen.getByTestId('batch-open-cancel'));

    expect(screen.getByTestId('warning-text').textContent).toMatch(/Loading cancelled/);

    act(() => screen.getByTestId('set-warning').click());

    expect(screen.getByTestId('warning-text').textContent).toMatch(/Loading cancelled/);
  });

  test('completing a non-cancelled batch leaves documentWarning untouched', () => {
    render(<AppProvider><Bootstrap /></AppProvider>);
    act(() => screen.getByTestId('start-2').click());
    act(() => screen.getByTestId('prog-1').click());
    act(() => screen.getByTestId('prog-2').click());
    act(() => screen.getByTestId('done').click());

    expect(screen.getByTestId('warning-text').textContent).toBe('');
  });

  test('cancelled flag persists past BATCH_OPEN_COMPLETE so late events cannot clobber the toast', async () => {
    // Wails alpha.85 dispatches each Go-side Event.Emit in its own
    // goroutine, so document:batch-complete can arrive at JS BEFORE
    // earlier document:opened events. The cancelled flag must stay set
    // until the next batch or an explicit dismiss; otherwise a late
    // OPEN_DOCUMENT (cancelled=false branch) would clear documentWarning.
    render(<AppProvider><Bootstrap /></AppProvider>);

    act(() => screen.getByTestId('start-2').click());
    await userEvent.click(screen.getByTestId('batch-open-cancel'));
    expect(screen.getByTestId('cancelled-flag').textContent).toBe('true');

    act(() => screen.getByTestId('done').click());
    expect(screen.getByTestId('cancelled-flag').textContent).toBe('true');

    // A late OPEN_DOCUMENT arriving after dialog close still preserves the toast.
    act(() => screen.getByTestId('open-doc').click());
    expect(screen.getByTestId('warning-text').textContent).toMatch(/Loading cancelled/);
  });

  test('starting a new batch clears the cancelled flag from a previous run', async () => {
    render(<AppProvider><Bootstrap /></AppProvider>);

    act(() => screen.getByTestId('start-2').click());
    await userEvent.click(screen.getByTestId('batch-open-cancel'));
    act(() => screen.getByTestId('done').click());

    // Flag still true after the prior cancelled batch finished.
    expect(screen.getByTestId('cancelled-flag').textContent).toBe('true');

    // Next batch resets it.
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

    // Simulate the user clicking the X on the warning toast (via direct dispatch).
    await userEvent.click(screen.getByTestId('open-doc')); // first ensure toast still there
    expect(screen.getByTestId('warning-text').textContent).toMatch(/Loading cancelled/);

    // Now dismiss.
    act(() => screen.getByTestId('dismiss-warning').click());
    expect(screen.getByTestId('cancelled-flag').textContent).toBe('false');
    expect(screen.getByTestId('warning-text').textContent).toBe('');
  });
});
