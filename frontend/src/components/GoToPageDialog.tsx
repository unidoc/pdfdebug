/**
 * @file Modal dialog for "Go to Page N". Resolves the page number to a content
 * stream node ID via pdfservice.GoToPage and dispatches NAVIGATE_TO_REF so the
 * existing tree-expand + scroll + flash flow takes over.
 */
import { useEffect, useRef, useState } from 'react';
import * as Dialog from '@radix-ui/react-dialog';
import { useAppState, useAppDispatch } from '../hooks/useDocumentState';
import { goToPage } from '../hooks/usePDFService';

export function GoToPageDialog() {
  const { tabs, activeTabId, goToPageOpen } = useAppState();
  const dispatch = useAppDispatch();
  const activeTab = tabs.find((t) => t.tabId === activeTabId) ?? null;
  const pageCount = activeTab?.pageCount ?? 0;

  const [value, setValue] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const inputRef = useRef<HTMLInputElement | null>(null);

  // Reset transient state every time the dialog re-opens so re-entry never
  // shows a stale error or pre-filled value.
  useEffect(() => {
    if (goToPageOpen) {
      setValue('');
      setError(null);
      setSubmitting(false);
    }
  }, [goToPageOpen]);

  const handleOpenChange = (open: boolean) => {
    if (!open) dispatch({ type: 'CLOSE_GO_TO_PAGE' });
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (activeTab === null) return;
    const trimmed = value.trim();
    if (trimmed === '') {
      setError('Enter a page number.');
      return;
    }
    const n = Number.parseInt(trimmed, 10);
    if (!Number.isFinite(n) || String(n) !== trimmed) {
      setError('Page number must be an integer.');
      return;
    }
    if (n < 1 || n > pageCount) {
      setError(`Page number out of range (1-${pageCount}).`);
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const nodeId = await goToPage(activeTab.tabId, n);
      dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: nodeId } });
      dispatch({ type: 'CLOSE_GO_TO_PAGE' });
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(msg);
    } finally {
      setSubmitting(false);
    }
  };

  const submitDisabled = submitting || value.trim() === '';

  return (
    <Dialog.Root open={goToPageOpen} onOpenChange={handleOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/40" />
        <Dialog.Content
          aria-label="Go to page"
          data-testid="go-to-page-dialog"
          className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 bg-surface border border-border rounded-md shadow-lg p-4 w-80"
          onOpenAutoFocus={(e) => {
            // Focus the input rather than the first focusable button.
            e.preventDefault();
            inputRef.current?.focus();
          }}
        >
          <Dialog.Title className="text-sm font-ui text-text mb-2">Go to Page</Dialog.Title>
          <form onSubmit={handleSubmit} noValidate>
            <label className="block text-xs text-text-muted mb-1" htmlFor="go-to-page-input">
              Page number (1-{pageCount})
            </label>
            <input
              id="go-to-page-input"
              ref={inputRef}
              type="text"
              inputMode="numeric"
              autoComplete="off"
              data-testid="go-to-page-input"
              className="w-full border border-border rounded px-2 py-1 text-sm font-ui bg-surface text-text"
              value={value}
              onChange={(e) => {
                setValue(e.target.value);
                if (error !== null) setError(null);
              }}
              disabled={submitting}
            />
            {error !== null && (
              <p
                role="alert"
                data-testid="go-to-page-error"
                className="mt-1 text-xs text-error"
              >
                {error}
              </p>
            )}
            <div className="mt-3 flex justify-end gap-2">
              <Dialog.Close asChild>
                <button
                  type="button"
                  className="px-3 py-1 text-sm font-ui rounded border border-border hover:bg-surface-hover"
                >
                  Cancel
                </button>
              </Dialog.Close>
              <button
                type="submit"
                data-testid="go-to-page-submit"
                disabled={submitDisabled}
                className="px-3 py-1 text-sm font-ui rounded bg-accent text-on-accent disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Go
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
