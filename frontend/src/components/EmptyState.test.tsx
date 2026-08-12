/**
 * 2.4-UNIT-002 [P1]: EmptyState drop zone highlights on drag-over with PDF file.
 *
 * Tests visual feedback for drag-and-drop interactions and the presence of
 * data-file-drop-target attribute required by Wails.
 */
import { render, screen, fireEvent, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { AppProvider } from '../hooks/useDocumentState';
import { EmptyState } from './EmptyState';

// Mock Wails bindings so imports resolve. Path matches what usePDFService.ts
// uses (../../bindings -> frontend/bindings), so vitest's mock registry hits
// when EmptyState clicks the Open File button.
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: vi.fn(),
    CloseDocument: vi.fn(),
    OpenFileDialog: vi.fn(),
  })
);

function renderEmptyState() {
  return render(
    <AppProvider>
      <EmptyState />
    </AppProvider>
  );
}

// Helper to create a DragEvent with items
function makeDragEvent(type: string, mimeType?: string) {
  const items = mimeType
    ? [{ kind: 'file', type: mimeType }]
    : [];
  return new Event(type, { bubbles: true }) as unknown as {
    preventDefault: () => void;
    stopPropagation: () => void;
    dataTransfer: { items: typeof items; files: File[] };
  };
}

describe('EmptyState drop zone', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('data-file-drop-target attribute exists on app container (window-wide drop)', () => {
    // data-file-drop-target is on the root app container (App.jsx),
    // not on the drop zone itself, so the entire window is a drop surface.
    // EmptyState drop zone is the visual indicator only.
    renderEmptyState();
    const dropZone = screen.getByTestId('drop-zone');
    expect(dropZone).not.toHaveAttribute('data-file-drop-target');
  });

  test('drop zone shows blue border on drag-over with PDF', () => {
    renderEmptyState();
    const emptyState = screen.getByTestId('empty-state');
    const dropZone = screen.getByTestId('drop-zone');

    // Simulate drag enter with a PDF file
    fireEvent.dragEnter(emptyState, {
      dataTransfer: {
        items: [{ kind: 'file', type: 'application/pdf' }],
      },
    });

    // During drag-over with PDF, drop zone should have focus border class
    expect(dropZone.className).toContain('border-border-focus');
    expect(dropZone.className).toContain('bg-surface-selected');
  });

  // ---------------------------------------------------------------------------
  // Story 10.8 AC1 (RED PHASE): the per-file invalid flash is REMOVED. The drop
  // zone must NEVER claim pre-drop knowledge it does not have. For ANY drag
  // combination (PDF-only, mixed, non-PDF-only) the hint stays the constant
  // "Drop a PDF file here" in text-text-muted, and the error styling
  // ("PDF files only" / text-error) is never applied.
  //
  // These assertions FAIL against the current EmptyState.tsx because the
  // dragenter/drop handlers still call setIsInvalidFile and the derived
  // hintText/hintColor still branch on it.
  // ---------------------------------------------------------------------------

  test('non-PDF drag does NOT show error flash', () => {
    renderEmptyState();
    const emptyState = screen.getByTestId('empty-state');

    // Drag enter with a non-PDF file
    fireEvent.dragEnter(emptyState, {
      dataTransfer: {
        items: [{ kind: 'file', type: 'image/png' }],
      },
    });

    const hint = screen.getByTestId('drop-zone-hint');
    // Backend is authoritative; the UI must not flash an invalid hint pre-drop.
    expect(hint.textContent).toBe('Drop a PDF file here');
    expect(hint.className).not.toContain('text-error');
    expect(hint.className).toContain('text-text-muted');
  });

  test('mixed-file drag does NOT show error flash', () => {
    renderEmptyState();
    const emptyState = screen.getByTestId('empty-state');

    // Mixed drag: a non-PDF first item with a PDF later. The old per-file
    // inspection only looked at index [0] and would lie either way.
    fireEvent.dragEnter(emptyState, {
      dataTransfer: {
        items: [
          { kind: 'file', type: 'text/plain' },
          { kind: 'file', type: 'application/pdf' },
        ],
      },
    });

    const hint = screen.getByTestId('drop-zone-hint');
    expect(hint.textContent).toBe('Drop a PDF file here');
    expect(hint.className).not.toContain('text-error');
    expect(hint.className).toContain('text-text-muted');
  });

  test('PDF-only drag keeps the standard hint (no error)', () => {
    renderEmptyState();
    const emptyState = screen.getByTestId('empty-state');

    fireEvent.dragEnter(emptyState, {
      dataTransfer: {
        items: [{ kind: 'file', type: 'application/pdf' }],
      },
    });

    const hint = screen.getByTestId('drop-zone-hint');
    expect(hint.textContent).toBe('Drop a PDF file here');
    expect(hint.className).not.toContain('text-error');
  });

  test('dropping a non-PDF file shows no error flash', () => {
    renderEmptyState();
    const emptyState = screen.getByTestId('empty-state');

    const nonPdfFile = new File(['data'], 'image.png', { type: 'image/png' });
    fireEvent.drop(emptyState, {
      dataTransfer: { files: [nonPdfFile] },
    });

    const hint = screen.getByTestId('drop-zone-hint');
    // The drop handler no longer sets the invalid flash; backend warning is the
    // authoritative source after the drop.
    expect(hint.textContent).toBe('Drop a PDF file here');
    expect(hint.className).not.toContain('text-error');
  });

  test('drop zone reverts after drag leave', () => {
    renderEmptyState();
    const emptyState = screen.getByTestId('empty-state');
    const dropZone = screen.getByTestId('drop-zone');

    fireEvent.dragEnter(emptyState, {
      dataTransfer: {
        items: [{ kind: 'file', type: 'application/pdf' }],
      },
    });
    expect(dropZone.className).toContain('border-border-focus');

    fireEvent.dragLeave(emptyState);
    expect(dropZone.className).toContain('border-border');
    expect(dropZone.className).not.toContain('bg-surface-selected');
  });

  test('renders Open File button and shortcut hint', () => {
    renderEmptyState();
    expect(screen.getByTestId('open-file-button')).toBeInTheDocument();
    expect(screen.getByTestId('shortcut-hint')).toBeInTheDocument();
  });

  test('returns null when hasDocument is true', () => {
    const { container } = render(
      <AppProvider>
        <EmptyState hasDocument />
      </AppProvider>
    );
    expect(container.innerHTML).toBe('');
  });
});

// ---------------------------------------------------------------------------
// Single-file loading state: when OPENING_START fires (Go-side
// document:load-start event or the EmptyState button's pre-await dispatch),
// the drop zone is replaced with a spinner + "Opening <filename>..." line.
// ---------------------------------------------------------------------------

import { useAppDispatch as _useAppDispatch } from '../hooks/useDocumentState';

function LoadingHarness({ fileName }: { fileName: string }) {
  const dispatch = _useAppDispatch();
  return (
    <>
      <button
        data-testid="bootstrap-opening"
        onClick={() => dispatch({ type: 'OPENING_START', payload: { fileName } })}
      >
        start
      </button>
      <EmptyState />
    </>
  );
}

describe('EmptyState loading variant', () => {
  test('renders spinner + "Opening <name>..." instead of the drop zone when isOpening is true', () => {
    render(
      <AppProvider>
        <LoadingHarness fileName="big-report.pdf" />
      </AppProvider>
    );

    act(() => screen.getByTestId('bootstrap-opening').click());

    expect(screen.getByTestId('empty-state-spinner')).toBeInTheDocument();
    expect(screen.getByTestId('empty-state-loading').textContent).toBe('Opening big-report.pdf...');
    expect(screen.queryByTestId('drop-zone')).not.toBeInTheDocument();
    expect(screen.queryByTestId('open-file-button')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Multi-select via the Open File dialog: when the picker returns >1 path,
// the BatchOpenDialog must appear, each path is opened sequentially via
// OPEN_DOCUMENT, progress advances, and the dialog closes on completion.
// Mirrors the drag-drop multi-file flow so users get parity between the two
// gestures.
// ---------------------------------------------------------------------------
import { OpenFile, GetTreeRoot, GetChildren, OpenFileDialog as _OpenFileDialog } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { useAppState } from '../hooks/useDocumentState';
import { BatchOpenDialog } from './BatchOpenDialog';

function MultiOpenHarness() {
  const state = useAppState();
  return (
    <>
      <span data-testid="tab-count">{state.tabs.length}</span>
      <span data-testid="batch-total">{state.batchOpenTotal}</span>
      <EmptyState />
      <BatchOpenDialog />
    </>
  );
}

describe('Open File dialog: multi-select', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('two-path selection opens 2 tabs, drives BATCH_OPEN_*, closes dialog when done', async () => {
    // Mock the dialog returning two paths and OpenFile + GetTreeRoot per path.
    (_OpenFileDialog as unknown as ReturnType<typeof vi.fn>).mockResolvedValue(['/a.pdf', '/b.pdf']);
    let openCall = 0;
    (OpenFile as unknown as ReturnType<typeof vi.fn>).mockImplementation(async (path: string) => {
      openCall += 1;
      return {
        tabId: `t${openCall}`,
        fileName: path.replace(/^\//, ''),
        filePath: path,
        pageCount: 1,
        fileSize: 100,
        error: '',
      };
    });
    (GetTreeRoot as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      id: 'root', label: 'Catalog', rawKey: '', nodeType: 'dict', valueType: '',
      hasChildren: true, childCount: 0, iconHint: 'catalog', error: '',
    });
    (GetChildren as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([]);

    render(<AppProvider><MultiOpenHarness /></AppProvider>);

    act(() => screen.getByTestId('open-file-button').click());

    // Dialog appears once batch starts; await async work (open + state updates).
    await act(async () => { /* yield */ });
    await act(async () => { /* yield */ });
    await act(async () => { /* yield */ });

    // Both opens completed; dialog closed; two tabs in state.
    expect(screen.getByTestId('tab-count').textContent).toBe('2');
    expect(screen.getByTestId('batch-total').textContent).toBe('0');
    expect(screen.queryByTestId('batch-open-dialog')).not.toBeInTheDocument();
  });

  test('single-path selection bypasses the batch dialog', async () => {
    (_OpenFileDialog as unknown as ReturnType<typeof vi.fn>).mockResolvedValue(['/only.pdf']);
    (OpenFile as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      tabId: 't1', fileName: 'only.pdf', filePath: '/only.pdf',
      pageCount: 1, fileSize: 100, error: '',
    });
    (GetTreeRoot as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      id: 'root', label: 'Catalog', rawKey: '', nodeType: 'dict', valueType: '',
      hasChildren: true, childCount: 0, iconHint: 'catalog', error: '',
    });
    (GetChildren as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([]);

    render(<AppProvider><MultiOpenHarness /></AppProvider>);
    act(() => screen.getByTestId('open-file-button').click());
    await act(async () => {});
    await act(async () => {});

    expect(screen.getByTestId('tab-count').textContent).toBe('1');
    // Single-file path never invoked the batch state machine.
    expect(screen.getByTestId('batch-total').textContent).toBe('0');
  });

  test('empty selection (cancel) is a no-op', async () => {
    (_OpenFileDialog as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([]);
    render(<AppProvider><MultiOpenHarness /></AppProvider>);
    act(() => screen.getByTestId('open-file-button').click());
    await act(async () => {});
    expect(screen.getByTestId('tab-count').textContent).toBe('0');
    expect(OpenFile as unknown as ReturnType<typeof vi.fn>).not.toHaveBeenCalled();
  });
});
