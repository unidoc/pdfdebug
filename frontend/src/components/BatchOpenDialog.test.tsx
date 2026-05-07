/**
 * BatchOpenDialog appears while a multi-PDF drop is in flight (batchOpenTotal
 * > 0) and disappears on BATCH_OPEN_COMPLETE.
 */
import { render, screen, act } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import { AppProvider, useAppDispatch } from '../hooks/useDocumentState';
import { BatchOpenDialog } from './BatchOpenDialog';

function Bootstrap() {
  const dispatch = useAppDispatch();
  return (
    <div>
      <button data-testid="start-2" onClick={() => dispatch({ type: 'BATCH_OPEN_START', payload: { total: 2 } })}>s</button>
      <button data-testid="prog-1" onClick={() => dispatch({ type: 'BATCH_OPEN_PROGRESS', payload: { completed: 1 } })}>p1</button>
      <button data-testid="prog-2" onClick={() => dispatch({ type: 'BATCH_OPEN_PROGRESS', payload: { completed: 2 } })}>p2</button>
      <button data-testid="done" onClick={() => dispatch({ type: 'BATCH_OPEN_COMPLETE' })}>d</button>
      <BatchOpenDialog />
    </div>
  );
}

describe('BatchOpenDialog', () => {
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
});
