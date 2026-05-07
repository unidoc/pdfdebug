/**
 * @file Progress dialog shown during a multi-file PDF drag-and-drop.
 * Visible when batchOpenTotal > 0; auto-closes on BATCH_OPEN_COMPLETE.
 */
import * as Dialog from '@radix-ui/react-dialog';
import { useAppState } from '../hooks/useDocumentState';

export function BatchOpenDialog() {
  const { batchOpenTotal, batchOpenCompleted } = useAppState();
  const open = batchOpenTotal > 0;
  // Clamp 0..1 even if a stray progress event delivers > total (defensive).
  const ratio = open ? Math.min(1, Math.max(0, batchOpenCompleted / batchOpenTotal)) : 0;
  const percent = Math.round(ratio * 100);

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
            Opened {batchOpenCompleted} of {batchOpenTotal}
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
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
