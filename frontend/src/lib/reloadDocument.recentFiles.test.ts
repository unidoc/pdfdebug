/**
 * Reload from disk opens the file again but is not a user open: it never
 * records the file in Open Recent, so a reload cannot add a duplicate entry or
 * reorder the list.
 *
 * Run: cd frontend && npx vitest run src/lib/reloadDocument.recentFiles.test.ts
 */
import { describe, test, expect, vi, beforeEach } from 'vitest';
import type { TabState } from '../hooks/useDocumentState';
import { reloadTab } from './reloadDocument';

const mockOpenFile = vi.fn();
const mockRecordRecentFile = vi.fn();
vi.mock('../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  OpenFile: (...a: unknown[]) => mockOpenFile(...a),
  GetTreeRoot: vi.fn().mockResolvedValue({ id: 'root', label: 'Catalog' }),
  GetChildren: vi.fn().mockResolvedValue([]),
  CloseDocument: vi.fn().mockResolvedValue(undefined),
  OpenFileDialog: vi.fn(),
  GetDiskStatus: vi.fn().mockResolvedValue({ state: 'unchanged' }),
  RecordRecentFile: (...a: unknown[]) => mockRecordRecentFile(...a),
}));

const tab = { tabId: 'tab-1', fileName: 'a.pdf', filePath: '/docs/a.pdf' } as unknown as TabState;

beforeEach(() => {
  vi.clearAllMocks();
});

describe('reload and Open Recent', () => {
  test('a successful reload does not record the file', async () => {
    mockOpenFile.mockResolvedValue({
      tabId: 'tab-2',
      fileName: 'a.pdf',
      filePath: '/docs/a.pdf',
      pageCount: 1,
      fileSize: 10,
      error: '',
    });
    const dispatch = vi.fn();

    await reloadTab(dispatch, () => [tab], 'tab-1');

    expect(mockOpenFile).toHaveBeenCalledWith('/docs/a.pdf');
    expect(dispatch).toHaveBeenCalled();
    expect(mockRecordRecentFile).not.toHaveBeenCalled();
  });

  test('a failed reload does not record the file', async () => {
    mockOpenFile.mockRejectedValue(new Error('file not found'));
    const dispatch = vi.fn();

    await reloadTab(dispatch, () => [tab], 'tab-1');

    expect(mockRecordRecentFile).not.toHaveBeenCalled();
  });
});
