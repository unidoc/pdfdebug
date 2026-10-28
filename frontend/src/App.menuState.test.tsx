/**
 * App reports the open document count to the backend with the navigation
 * state, so the native menu can disable document items with no document
 * open and the next/previous document items with fewer than two.
 *
 * Run: cd frontend && npx vitest run src/App.menuState.test.tsx
 */
import { render, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { Events } from '@wailsio/runtime';
import { NavigateStateChanged } from '../bindings/events/index.js';

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
  RecordRecentFile: vi.fn().mockResolvedValue(undefined),
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

function docInfo(path: string) {
  return {
    tabId: `tab-${path}`,
    fileName: path.replace(/^.*[/\\]/, ''),
    filePath: path,
    pageCount: 1,
    fileSize: 10,
    error: '',
  };
}

function reportedCounts(): number[] {
  return vi.mocked(Events.Emit).mock.calls
    .filter(([name]) => name === NavigateStateChanged)
    .map(([, data]) => (data as { documentCount: number }).documentCount);
}

beforeEach(() => {
  vi.mocked(Events.Emit).mockClear();
  mockConsume.mockReset();
  mockOpenFile.mockReset();
  mockOpenFile.mockImplementation((path: string) => Promise.resolve(docInfo(path)));
});

describe('menu state reports the open document count', () => {
  test('nothing is reported while no document is open', async () => {
    mockConsume.mockResolvedValue([]);
    const { default: App } = await import('./App');
    const { findByTestId } = render(<App />);
    await findByTestId('empty-state');
    expect(reportedCounts()).toEqual([]);
  });

  test('one open document is reported as one', async () => {
    mockConsume.mockResolvedValue(['/docs/a.pdf']);
    const { default: App } = await import('./App');
    render(<App />);
    await waitFor(() => expect(reportedCounts().at(-1)).toBe(1));
  });

  test('two open documents are reported as two', async () => {
    mockConsume.mockResolvedValue(['/docs/a.pdf', '/docs/b.pdf']);
    const { default: App } = await import('./App');
    render(<App />);
    await waitFor(() => expect(reportedCounts().at(-1)).toBe(2));
  });
});
