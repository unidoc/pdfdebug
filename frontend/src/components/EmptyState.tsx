/**
 * @file Landing screen shown when no document is open.
 * Provides drag-and-drop PDF import and an "Open File" button.
 */
import { useState, useRef, useCallback, useEffect } from 'react';
import { getShortcutHint } from '../lib/platform';
import { useAppDispatch } from '../hooks/useDocumentState';
import { openPDFFile, openFileDialog, mapErrorMessage } from '../hooks/usePDFService';

/** Props for {@link EmptyState}. */
export interface EmptyStateProps {
  hasDocument?: boolean;
  onOpenFile?: () => void;
}

/**
 * Empty-state landing screen with a drag-and-drop zone and open-file button.
 * Validates that only PDF files are accepted during drag and on drop.
 */
export function EmptyState({ hasDocument, onOpenFile }: EmptyStateProps) {
  const [isDragOver, setIsDragOver] = useState(false);
  const [isInvalidFile, setIsInvalidFile] = useState(false);
  // dragCounter tracks nested dragenter/dragleave pairs to avoid
  // premature reset when dragging over child elements.
  const dragCounter = useRef<number>(0);
  const invalidTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Clean up pending timeout on unmount to avoid state updates after unmount
  useEffect(() => {
    return () => {
      if (invalidTimerRef.current !== null) {
        clearTimeout(invalidTimerRef.current);
      }
    };
  }, []);

  const handleDragEnter = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    dragCounter.current += 1;
    setIsDragOver(true);

    // Reset invalid state from a previous rejected drop whose 2-second timer
    // may still be pending -- a new drag should start with a clean slate
    setIsInvalidFile(false);
    if (invalidTimerRef.current !== null) {
      clearTimeout(invalidTimerRef.current);
      invalidTimerRef.current = null;
    }

    // Check dataTransfer.items for file type if available
    if (e.dataTransfer.items && e.dataTransfer.items.length > 0) {
      const hasKnownType = Array.from(e.dataTransfer.items).some(
        (item) => item.type !== ''
      );
      if (hasKnownType) {
        const hasPdf = Array.from(e.dataTransfer.items).some(
          (item) => item.type === 'application/pdf'
        );
        if (!hasPdf) {
          setIsInvalidFile(true);
        }
      }
      // If no known types exposed, default to valid state (blue highlight)
    }
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
      setIsInvalidFile(false);
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

    const file = e.dataTransfer.files[0];
    if (file && file.name.toLowerCase().endsWith('.pdf')) {
      setIsInvalidFile(false);
      if (invalidTimerRef.current !== null) {
        clearTimeout(invalidTimerRef.current);
        invalidTimerRef.current = null;
      }
      // File is processed by the Wails runtime's native drop handler
      // which emits WindowFilesDropped to the Go backend (main.go).
    } else {
      if (invalidTimerRef.current !== null) {
        clearTimeout(invalidTimerRef.current);
      }
      setIsInvalidFile(true);
      invalidTimerRef.current = setTimeout(() => {
        setIsInvalidFile(false);
        invalidTimerRef.current = null;
      }, 2000);
    }
  }, []);

  const dispatch = useAppDispatch();

  // Opens the native file dialog, loads each selected PDF, and dispatches to
  // state. Multi-select parity with drag-drop: when more than one file is
  // chosen, drives the BatchOpenDialog progress state machine the same way
  // the backend file-drop handler does.
  // Dedup cleanup (freeing backend state for duplicate tabIds) is handled in
  // App.jsx and does not apply here -- EmptyState only renders when no tabs
  // exist, so the cross-tab dedup path can't trigger.
  const handleOpenFileClick = useCallback(async () => {
    if (onOpenFile) {
      onOpenFile();
      return;
    }
    try {
      const paths = await openFileDialog();
      if (paths.length === 0) return;

      const isBatch = paths.length > 1;
      if (isBatch) dispatch({ type: 'BATCH_OPEN_START', payload: { total: paths.length } });

      let lastWarning: string | null = null;
      try {
        for (let i = 0; i < paths.length; i++) {
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
            // Surface the failure but keep iterating; otherwise one bad file
            // blocks the rest of the batch.
            const msg = err instanceof Error ? err.message : String(err);
            dispatch({ type: 'SET_DOCUMENT_ERROR', payload: { message: mapErrorMessage(msg) } });
          }
          if (isBatch) dispatch({ type: 'BATCH_OPEN_PROGRESS', payload: { completed: i + 1 } });
        }
      } finally {
        if (isBatch) dispatch({ type: 'BATCH_OPEN_COMPLETE' });
      }
      if (lastWarning !== null) {
        dispatch({ type: 'SET_DOCUMENT_WARNING', payload: { message: lastWarning } });
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      dispatch({ type: 'SET_DOCUMENT_ERROR', payload: { message: mapErrorMessage(msg) } });
    }
  }, [onOpenFile, dispatch]);

  if (hasDocument) {
    return null;
  }

  // Drop zone border and background classes based on drag state
  // Note: prefers-reduced-motion is handled by the global CSS rule in style.css
  // which overrides transition-duration with !important -- no separate
  // motion-reduce: Tailwind variants needed.
  const dropZoneBorder =
    isDragOver && !isInvalidFile
      ? 'border-border-focus bg-surface-selected'
      : isInvalidFile && isDragOver
        ? 'border-border-focus'
        : 'border-border';

  // Hint text content and color
  const hintText = isInvalidFile ? 'PDF files only' : 'Drop a PDF file here';
  const hintColor = isInvalidFile ? 'text-error' : 'text-text-muted';

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
