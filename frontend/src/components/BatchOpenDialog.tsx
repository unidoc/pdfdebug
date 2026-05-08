/**
 * @file Progress dialog shown during a multi-file PDF drag-and-drop.
 * Visible when batchOpenTotal > 0; auto-closes on BATCH_OPEN_COMPLETE.
 * Includes a Cancel button that signals the active loop (JS- or Go-side) to
 * stop after the file currently being opened. Already-opened tabs are kept.
 */
import * as Dialog from '@radix-ui/react-dialog';
import { Events } from '@wailsio/runtime';
import { useAppState, useAppDispatch } from '../hooks/useDocumentState';

export function BatchOpenDialog() {
  const { batchOpenTotal, batchOpenCompleted, batchOpenCancelled } = useAppState();
  const dispatch = useAppDispatch();
  const open = batchOpenTotal > 0;
  const ratio = open ? Math.min(1, Math.max(0, batchOpenCompleted / batchOpenTotal)) : 0;
  const percent = Math.round(ratio * 100);

  const handleCancel = () => {
    if (batchOpenCancelled) return;
    dispatch({ type: 'BATCH_OPEN_CANCEL' });
    // Signal the Go-side loop; the JS-side loop in EmptyState reads the
    // cancel state via ref and does not need this event.
    Events.Emit('document:batch-cancel', null);
  };

  const status = batchOpenCancelled
    ? `Cancelling... opened ${batchOpenCompleted} of ${batchOpenTotal}`
    : `Opened ${batchOpenCompleted} of ${batchOpenTotal}`;

  return (
    <Dialog.Root open={open}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/40" />
        <Dialog.Content
          aria-label="Opening multiple PDFs"
          data-testid="batch-open-dialog"
          // Backend drives close; ignore Escape and outside-click while in flight.
          onEscapeKeyDown={(e) => e.preventDefault()}
          onPointerDownOutside={(e) => e.preventDefault()}
          className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 bg-surface border border-border rounded-md shadow-lg p-4 w-80"
        >
          <Dialog.Title className="text-sm font-ui text-text mb-1">Opening PDFs</Dialog.Title>
          <Dialog.Description className="text-xs text-text-muted mb-3" data-testid="batch-open-status">
            {status}
          </Dialog.Description>
          <div
            className="w-full h-2 rounded bg-surface-hover overflow-hidden"
            role="progressbar"
            aria-valuemin={0}
            aria-valuemax={batchOpenTotal}
            aria-valuenow={batchOpenCompleted}
            data-testid="batch-open-progressbar"
          >
            <div
              className="h-full bg-border-focus transition-[width] duration-150"
              style={{ width: `${percent}%` }}
            />
          </div>
          <div className="mt-4 flex justify-end">
            <button
              data-testid="batch-open-cancel"
              type="button"
              disabled={batchOpenCancelled}
              onClick={handleCancel}
              className="text-sm px-3 py-1 rounded border border-border bg-surface hover:bg-surface-hover cursor-pointer disabled:cursor-not-allowed disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-border-focus focus-visible:outline-none"
            >
              {batchOpenCancelled ? 'Cancelling...' : 'Cancel'}
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
