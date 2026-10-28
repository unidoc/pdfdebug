/**
 * EmptyState Open button records each successfully opened file in Open
 * Recent through RecordRecentFile. A file that fails to open is not recorded.
 *
 * Run: cd frontend && npx vitest run src/components/EmptyState.recordRecent.test.tsx
 */
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { AppProvider } from '../hooks/useDocumentState';
import { EmptyState } from './EmptyState';

const mockOpenFileDialog = vi.fn();
const mockOpenFile = vi.fn();
const mockRecordRecentFile = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: (...a: unknown[]) => mockOpenFile(...a),
    GetTreeRoot: vi.fn().mockResolvedValue({ id: 'root', label: 'Catalog' }),
    GetChildren: vi.fn().mockResolvedValue([]),
    CloseDocument: vi.fn().mockResolvedValue(undefined),
    OpenFileDialog: (...a: unknown[]) => mockOpenFileDialog(...a),
    RecordRecentFile: (...a: unknown[]) => mockRecordRecentFile(...a),
  })
);

function docInfo(tabId: string, filePath: string) {
  return {
    tabId,
    fileName: filePath.replace(/^.*[/\\]/, ''),
    filePath,
    pageCount: 1,
    fileSize: 10,
    error: '',
  };
}

function renderEmptyState() {
  return render(
    <AppProvider>
      <EmptyState />
    </AppProvider>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mockRecordRecentFile.mockResolvedValue(undefined);
});

describe('EmptyState records opened files in Open Recent', () => {
  test('a file opened from the Open button is recorded', async () => {
    mockOpenFileDialog.mockResolvedValue(['/docs/a.pdf']);
    mockOpenFile.mockResolvedValue(docInfo('tab-a', '/docs/a.pdf'));
    renderEmptyState();

    fireEvent.click(screen.getByTestId('open-file-button'));

    await waitFor(() => expect(mockRecordRecentFile).toHaveBeenCalledWith('/docs/a.pdf'));
    expect(mockRecordRecentFile).toHaveBeenCalledTimes(1);
  });

  test('a multi-file open records each file that opened and skips the one that failed', async () => {
    mockOpenFileDialog.mockResolvedValue(['/docs/a.pdf', '/docs/bad.pdf', '/docs/c.pdf']);
    mockOpenFile.mockImplementation((path: string) =>
      path === '/docs/bad.pdf'
        ? Promise.reject(new Error('malformed PDF'))
        : Promise.resolve(docInfo(`tab-${path}`, path))
    );
    renderEmptyState();

    fireEvent.click(screen.getByTestId('open-file-button'));

    await waitFor(() => expect(mockRecordRecentFile).toHaveBeenCalledTimes(2));
    expect(mockRecordRecentFile).toHaveBeenCalledWith('/docs/a.pdf');
    expect(mockRecordRecentFile).toHaveBeenCalledWith('/docs/c.pdf');
    expect(mockRecordRecentFile).not.toHaveBeenCalledWith('/docs/bad.pdf');
  });

  test('a cancelled dialog records nothing', async () => {
    mockOpenFileDialog.mockResolvedValue([]);
    renderEmptyState();

    fireEvent.click(screen.getByTestId('open-file-button'));

    await waitFor(() => expect(mockOpenFileDialog).toHaveBeenCalled());
    expect(mockRecordRecentFile).not.toHaveBeenCalled();
  });
});
