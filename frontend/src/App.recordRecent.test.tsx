/**
 * Cold-start pending opens (file association launch) record each file that
 * opened in Open Recent through RecordRecentFile; a path that fails to open
 * is not recorded.
 *
 * Run: cd frontend && npx vitest run src/App.recordRecent.test.tsx
 */
import { render, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: () => () => {},
    Emit: vi.fn(),
  },
  Screens: { GetAll: vi.fn().mockResolvedValue(null) },
  Window: {
    SetSize: vi.fn().mockResolvedValue(undefined),
    SetPosition: vi.fn().mockResolvedValue(undefined),
    Position: vi.fn().mockResolvedValue({ x: 0, y: 0 }),
    Size: vi.fn().mockResolvedValue({ width: 800, height: 600 }),
  },
}));

const mockConsume = vi.fn();
const mockOpenFile = vi.fn();
const mockRecordRecentFile = vi.fn();

vi.mock('../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  OpenFile: (...a: unknown[]) => mockOpenFile(...a),
  SetWatchedTabs: vi.fn().mockResolvedValue(undefined),
  GetTreeRoot: vi.fn().mockResolvedValue({ id: 'root', label: 'Catalog' }),
  GetChildren: vi.fn().mockResolvedValue([]),
  CloseDocument: vi.fn().mockResolvedValue(undefined),
  OpenFileDialog: vi.fn(),
  GetPageIndex: vi.fn(),
  GetImageIndex: vi.fn(),
  GetImagePages: vi.fn(),
  GetImagePageGroups: vi.fn(),
  ConsumePendingOpenFiles: (...a: unknown[]) => mockConsume(...a),
  RecordRecentFile: (...a: unknown[]) => mockRecordRecentFile(...a),
}));

vi.mock('./components/EmptyState', () => ({
  EmptyState: () => <div data-testid="empty-state">Empty</div>,
}));
vi.mock('./components/MainLayout', () => ({
  MainLayout: () => <div data-testid="main-layout">Layout</div>,
}));
vi.mock('./components/ErrorBanner', () => ({
  ErrorBanner: ({ message }: { message: string }) => (
    <div data-testid="error-banner">{message}</div>
  ),
}));
vi.mock('./components/TabBar', () => ({
  TabBar: () => <div data-testid="tab-bar">Tabs</div>,
}));
vi.mock('./components/UpdateNotifier', () => ({
  UpdateNotifier: () => null,
}));

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

beforeEach(() => {
  mockConsume.mockReset();
  mockOpenFile.mockReset();
  mockRecordRecentFile.mockReset();
  mockRecordRecentFile.mockResolvedValue(undefined);
});

describe('cold-start opens record Open Recent', () => {
  test('each pending path that opens is recorded, a failing one is not', async () => {
    mockConsume.mockResolvedValue(['/docs/a.pdf', '/docs/bad.pdf', '/docs/c.pdf']);
    mockOpenFile.mockImplementation((path: string) =>
      path === '/docs/bad.pdf'
        ? Promise.reject(new Error('malformed PDF'))
        : Promise.resolve(docInfo(`tab-${path}`, path))
    );
    const { default: App } = await import('./App');
    render(<App />);

    await waitFor(() => expect(mockRecordRecentFile).toHaveBeenCalledTimes(2));
    expect(mockRecordRecentFile).toHaveBeenCalledWith('/docs/a.pdf');
    expect(mockRecordRecentFile).toHaveBeenCalledWith('/docs/c.pdf');
    expect(mockRecordRecentFile).not.toHaveBeenCalledWith('/docs/bad.pdf');
  });

  test('no pending paths records nothing', async () => {
    mockConsume.mockResolvedValue([]);
    const { default: App } = await import('./App');
    render(<App />);

    await waitFor(() => expect(mockConsume).toHaveBeenCalled());
    expect(mockRecordRecentFile).not.toHaveBeenCalled();
  });
});
