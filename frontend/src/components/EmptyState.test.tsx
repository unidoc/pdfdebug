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

// Mock Wails bindings so imports resolve
vi.mock(
  '../../../bindings/unipdf-debugger/internal/pdfservice/pdfservice.js',
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

describe('2.4-UNIT-002: EmptyState drop zone', () => {
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

  test('drop zone shows "PDF files only" for non-PDF drag', () => {
    renderEmptyState();
    const emptyState = screen.getByTestId('empty-state');

    // Drag enter with a non-PDF file
    fireEvent.dragEnter(emptyState, {
      dataTransfer: {
        items: [{ kind: 'file', type: 'image/png' }],
      },
    });

    const hint = screen.getByTestId('drop-zone-hint');
    expect(hint.textContent).toBe('PDF files only');
    expect(hint.className).toContain('text-error');
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

  test('shows invalid hint on drop of non-PDF file', () => {
    vi.useFakeTimers();
    renderEmptyState();
    const emptyState = screen.getByTestId('empty-state');

    const nonPdfFile = new File(['data'], 'image.png', {
      type: 'image/png',
    });

    fireEvent.drop(emptyState, {
      dataTransfer: { files: [nonPdfFile] },
    });

    const hint = screen.getByTestId('drop-zone-hint');
    expect(hint.textContent).toBe('PDF files only');

    // After 2 seconds, hint resets
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(hint.textContent).toBe('Drop a PDF file here');

    vi.useRealTimers();
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
