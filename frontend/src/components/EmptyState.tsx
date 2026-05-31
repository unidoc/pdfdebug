/**
 * @file Landing screen shown when no document is open.
 * Provides drag-and-drop PDF import and an "Open File" button.
 */
import { useState, useRef, useCallback, useEffect } from 'react';
import { getShortcutHint } from '../lib/platform';
import { useAppDispatch, useAppState } from '../hooks/useDocumentState';
import { openPDFFile, openFileDialog, mapErrorMessage } from '../hooks/usePDFService';

/** Props for {@link EmptyState}. */
export interface EmptyStateProps {
  hasDocument?: boolean;
  onOpenFile?: () => void;
}

/**
 * Empty-state landing screen with a drag-and-drop zone and open-file button.
 * The drop zone shows a static hint; the backend filters and validates dropped
 * files and surfaces an advisory warning for unsupported ones after the drop.
 */
export function EmptyState({ hasDocument, onOpenFile }: EmptyStateProps) {
  const [isDragOver, setIsDragOver] = useState(false);
  // dragCounter tracks nested dragenter/dragleave pairs to avoid
  // premature reset when dragging over child elements.
  const dragCounter = useRef<number>(0);

  const handleDragEnter = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    dragCounter.current += 1;
    setIsDragOver(true);
  }, []);

  const handleDragOver = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    // Required to allow drop -- do NOT increment counter or update state
    e.preventDefault();
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    e.stopPropagation();
    dragCounter.current -= 1;
    // Guard against negative counter from unpaired dragleave events
    if (dragCounter.current < 0) {
      dragCounter.current = 0;
    }
    if (dragCounter.current === 0) {
      setIsDragOver(false);
    }
  }, []);

  const handleDrop = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    // Only preventDefault to stop navigation, but do NOT stopPropagation --
    // the Wails runtime registers its own drop handler on the window that
    // uses WebView2's postMessageWithAdditionalObjects to send file paths
    // to the Go backend. Stopping propagation blocks that handler on Windows.
    e.preventDefault();
    dragCounter.current = 0;
    setIsDragOver(false);
    // The dropped files are processed by the Wails runtime's native drop
    // handler (main.go), which filters .pdf files and emits per-batch results.
    // The backend's "N unsupported files could not be opened" advisory is the
    // authoritative source of validation feedback -- the UI no longer guesses
    // pre-drop, so no per-file flash is shown here.
  }, []);

  const dispatch = useAppDispatch();
  const { batchOpenCancelled, isOpening, openingFileName } = useAppState();
  // Mirror cancel state into a ref so the async loop below sees fresh
  // values without re-running on every state change.
  const cancelledRef = useRef(false);
  useEffect(() => {
    cancelledRef.current = batchOpenCancelled;
  }, [batchOpenCancelled]);

  // Opens the native file dialog and loads each selected PDF. For multi-file
  // selections, drives BatchOpenDialog progress state and checks cancelledRef
  // between iterations so Cancel can stop the loop.
  const handleOpenFileClick = useCallback(async () => {
    if (onOpenFile) {
      onOpenFile();
      return;
    }
    try {
      const paths = await openFileDialog();
      if (paths.length === 0) return;

      const isBatch = paths.length > 1;
      if (isBatch) {
        cancelledRef.current = false;
        dispatch({ type: 'BATCH_OPEN_START', payload: { total: paths.length } });
      } else {
        // Single-file open via the button: surface the inline EmptyState
        // loading state before the openPDFFile await blocks.
        const fileName = paths[0].replace(/^.*[/\\]/, '');
        dispatch({ type: 'OPENING_START', payload: { fileName } });
      }

      let lastWarning: string | null = null;
      try {
        for (let i = 0; i < paths.length; i++) {
          if (isBatch && cancelledRef.current) break;
          try {
            const result = await openPDFFile(paths[i]);
            dispatch({
              type: 'OPEN_DOCUMENT',
              payload: {
                tabId: result.tabId,
                fileName: result.fileName,
                filePath: result.filePath,
                pageCount: result.pageCount,
                rootNode: result.rootNode,
                rootChildren: result.rootChildren,
              },
            });
            if (result.warning) lastWarning = result.warning;
          } catch (err: unknown) {
            // Surface and keep iterating so one bad file does not block the rest.
            const msg = err instanceof Error ? err.message : String(err);
            dispatch({ type: 'SET_DOCUMENT_ERROR', payload: { message: mapErrorMessage(msg) } });
          }
        }
      } finally {
        if (isBatch) dispatch({ type: 'BATCH_OPEN_COMPLETE' });
        // SET_DOCUMENT_WARNING is a no-op when batchOpenCancelled is true,
        // so the cancellation toast survives this dispatch.
        if (lastWarning !== null) {
          dispatch({ type: 'SET_DOCUMENT_WARNING', payload: { message: lastWarning } });
        }
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      dispatch({ type: 'SET_DOCUMENT_ERROR', payload: { message: mapErrorMessage(msg) } });
    }
  }, [onOpenFile, dispatch]);

  if (hasDocument) {
    return null;
  }

  // Loading variant: an open is in flight. Replace the drop zone + button
  // with a spinner + filename so the user gets immediate feedback during
  // the (potentially slow) pdfcpu xref walk on large PDFs.
  if (isOpening) {
    return (
      <div
        data-testid="empty-state"
        className="flex flex-col items-center justify-center h-full"
      >
        <h1 className="text-xl font-semibold text-text">UniDoc PDF Debugger</h1>
        <p className="text-sm text-text-secondary mt-2">Inspect PDF internal structure</p>
        <div className="mt-8 flex flex-col items-center gap-3">
          <div
            data-testid="empty-state-spinner"
            aria-hidden="true"
            className="animate-spin w-8 h-8 rounded-full border-2 border-text-muted border-t-transparent"
          />
          <p
            data-testid="empty-state-loading"
            aria-live="polite"
            className="text-sm text-text-muted"
          >
            Opening {openingFileName ?? 'document'}...
          </p>
        </div>
      </div>
    );
  }

  // Drop zone border and background classes based on drag state
  // Note: prefers-reduced-motion is handled by the global CSS rule in style.css
  // which overrides transition-duration with !important -- no separate
  // motion-reduce: Tailwind variants needed.
  const dropZoneBorder = isDragOver
    ? 'border-border-focus bg-surface-selected'
    : 'border-border';

  // Hint text content and color -- constant; the backend is authoritative for
  // post-drop validation, so the UI never claims pre-drop knowledge.
  const hintText = 'Drop a PDF file here';
  const hintColor = 'text-text-muted';

  return (
    <div
      data-testid="empty-state"
      className="flex flex-col items-center justify-center h-full"
      onDragEnter={handleDragEnter}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      <h1
        data-testid="empty-state-title"
        className="text-xl font-semibold text-text"
      >
        UniDoc PDF Debugger
      </h1>

      <p
        data-testid="empty-state-subtitle"
        className="text-sm text-text-secondary mt-2"
      >
        Inspect PDF internal structure
      </p>

      <div
        data-testid="drop-zone"
        role="region"
        aria-label="File drop zone"
        className={`mt-8 px-12 py-10 border-2 border-dashed rounded-lg transition-colors duration-150 ${dropZoneBorder}`}
      >
        <p
          data-testid="drop-zone-hint"
          aria-live="polite"
          className={`text-sm ${hintColor}`}
        >
          {hintText}
        </p>
      </div>

      <p className="text-text-muted text-sm my-4">or</p>

      {/* bg-border-focus intentionally reuses Blue 500 as button-primary -- no dedicated token exists yet */}
      <button
        data-testid="open-file-button"
        className="bg-border-focus text-white rounded px-4 py-2 font-medium cursor-pointer hover:opacity-90 focus-visible:ring-2 focus-visible:ring-border-focus focus-visible:outline-none"
        onClick={handleOpenFileClick}
      >
        Open File...
      </button>

      <p
        data-testid="shortcut-hint"
        className="text-xs text-text-muted mt-2"
      >
        {getShortcutHint('O')}
      </p>
    </div>
  );
}
