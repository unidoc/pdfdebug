/**
 * The Images navigator, driven through the main layout's rail: the third rail
 * destination, text rows built from GetImageIndex with a badge strip, the flat
 * and by-page views, sorting, page expansion, the shared selection that opens
 * the existing image preview, the "Show node in tree" menu, empty states and
 * the per-tab cache.
 *
 * Run: cd frontend && npx vitest run src/components/ImagesPanel.test.tsx
 */
import { render, screen, act, fireEvent, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import type { Dispatch, ReactNode } from 'react';
import { AppProvider, useAppDispatch, useAppState, type AppAction, type AppState } from '../hooks/useDocumentState';
import { MainLayout } from './MainLayout';

const RAIL_KEY = 'unidoc-pdf-debugger:left-rail';

const EMPTY_COPY =
  "No image XObjects are referenced from this document's page resources. Inline images (BI/ID/EI) are not listed.";
const PAGE_INDEX_NOTE = 'Page rows cannot be selected: the page index could not be loaded.';

vi.mock('allotment', () => {
  function Pane({ children }: { children: ReactNode }) {
    return <div>{children}</div>;
  }
  function Allotment({ children }: { children: ReactNode }) {
    return <div>{children}</div>;
  }
  Allotment.Pane = Pane;
  return { Allotment };
});

vi.mock('allotment/dist/style.css', () => ({}));

const mockGetImageIndex = vi.fn();
const mockGetImagePages = vi.fn();
const mockGetImagePageGroups = vi.fn();
const mockGetPageIndex = vi.fn();
const mockGetChildren = vi.fn();
const mockGetAncestorPath = vi.fn();
const mockGetObjectSource = vi.fn();
const mockGetObjectDetail = vi.fn();
const mockDescribeImage = vi.fn();
const mockGetImageData = vi.fn();

vi.mock('../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  OpenFile: vi.fn(),
  GetTreeRoot: vi.fn(),
  GetChildren: (...args: unknown[]) => mockGetChildren(...args),
  CloseDocument: vi.fn(),
  OpenFileDialog: vi.fn(),
  GetObjectDetail: (...args: unknown[]) => mockGetObjectDetail(...args),
  GetObjectSource: (...args: unknown[]) => mockGetObjectSource(...args),
  GetContentStream: vi.fn().mockResolvedValue(null),
  GetImageData: (...args: unknown[]) => mockGetImageData(...args),
  DescribeImage: (...args: unknown[]) => mockDescribeImage(...args),
  SaveImageToFile: vi.fn().mockResolvedValue(''),
  GetFontView: vi.fn().mockResolvedValue({ kind: 'neither', detail: null, roster: null }),
  GetReverseRefs: vi.fn().mockResolvedValue([]),
  GetAncestorPath: (...args: unknown[]) => mockGetAncestorPath(...args),
  GetXRefTable: vi.fn().mockResolvedValue({ tabId: '', entries: [] }),
  GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
  GetSignatures: vi.fn().mockResolvedValue([]),
  GetEmbeddedFileBytes: vi.fn().mockResolvedValue(''),
  GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '', warning: '' }),
  SaveBytesToFile: vi.fn().mockResolvedValue(''),
  DiffDocuments: vi.fn().mockResolvedValue({ root: null, summary: {} }),
  GetPageIndex: (...args: unknown[]) => mockGetPageIndex(...args),
  GetImageIndex: (...args: unknown[]) => mockGetImageIndex(...args),
  GetImagePages: (...args: unknown[]) => mockGetImagePages(...args),
  GetImagePageGroups: (...args: unknown[]) => mockGetImagePageGroups(...args),
}));

class MockResizeObserver {
  callback: ResizeObserverCallback;
  constructor(callback: ResizeObserverCallback) {
    this.callback = callback;
  }
  observe(target: Element) {
    this.callback(
      [{ target, contentRect: { width: 300, height: 600 } as DOMRectReadOnly } as ResizeObserverEntry],
      this as unknown as ResizeObserver,
    );
  }
  unobserve() {}
  disconnect() {}
}

interface ImageEntry {
  objNum: number;
  gen: number;
  nodeId: string;
  width: number;
  height: number;
  bitsPerComponent: number;
  colorSpace: string;
  filters: string[];
  imageMask: boolean;
  smask: string | null;
  decode: number[] | null;
  decodeNonDefault: boolean;
  sampleInterpretation: string;
  adobeMarker: string;
  adobeTransform: number | null;
  estimatedBytes: number;
  firstPage: number;
  pageCount: number;
  firstPages: number[];
  warning: string;
  error: string;
}

function img(objNum: number, over: Partial<ImageEntry> = {}): ImageEntry {
  return {
    objNum,
    gen: 0,
    nodeId: `obj:0:${objNum}`,
    width: 2,
    height: 2,
    bitsPerComponent: 8,
    colorSpace: 'DeviceGray',
    filters: [],
    imageMask: false,
    smask: null,
    decode: null,
    decodeNonDefault: false,
    sampleInterpretation: 'Normal (default)',
    adobeMarker: 'not-applicable',
    adobeTransform: null,
    estimatedBytes: 4,
    firstPage: 1,
    pageCount: 1,
    firstPages: [1],
    warning: '',
    error: '',
    ...over,
  };
}

const WALK_STOPPED = 'image walk stopped after 1000000 resource entries at page 412; later pages were not walked';

function errorRowEntry(error = WALK_STOPPED): ImageEntry {
  return img(0, { nodeId: '', width: 0, height: 0, bitsPerComponent: 0, colorSpace: '', firstPage: 0, pageCount: 0, firstPages: [], error, sampleInterpretation: '', adobeMarker: '' });
}

const range = (from: number, to: number) => Array.from({ length: to - from + 1 }, (_, i) => from + i);

// A logo on 20 pages (capped first pages), a two-page inverted gray image, a
// facts read that panicked on an image, a stencil mask with a warning, and an
// error row; in the walk's default order.
const logo = img(12, {
  width: 1240,
  height: 800,
  colorSpace: 'DeviceRGB',
  filters: ['DCTDecode'],
  smask: '13 0 R',
  adobeMarker: 'present',
  adobeTransform: 2,
  estimatedBytes: 2976000,
  firstPage: 1,
  pageCount: 20,
  firstPages: range(1, 16),
});
const inverted = img(19, {
  width: 640,
  height: 480,
  filters: ['FlateDecode'],
  decode: [1, 0],
  decodeNonDefault: true,
  sampleInterpretation: 'Inverted by /Decode',
  firstPage: 2,
  pageCount: 2,
  firstPages: [2, 3],
});
const panicked = img(7, { firstPage: 3, pageCount: 1, firstPages: [3], error: 'image facts read failed: boom' });
const stencil = img(23, {
  width: 2480,
  height: 3508,
  bitsPerComponent: 1,
  colorSpace: '',
  imageMask: true,
  firstPage: 3,
  pageCount: 1,
  firstPages: [3],
  warning: 'Width metadata: not an integer',
});
const entries: ImageEntry[] = [logo, inverted, panicked, stencil, errorRowEntry()];

interface PageEntry {
  pageNum: number;
  objNum: number;
  gen: number;
  nodeId: string;
  contentNodeId: string;
  mediaBox: number[];
  rotate: number;
  inherited: number;
  annotCount: number;
  contentLen: number;
  error: string;
}

function pageEntry(pageNum: number): PageEntry {
  return {
    pageNum,
    objNum: 100 + pageNum,
    gen: 0,
    nodeId: `obj:0:${100 + pageNum}`,
    contentNodeId: '',
    mediaBox: [0, 0, 612, 792],
    rotate: 0,
    inherited: 0,
    annotCount: 0,
    contentLen: 0,
    error: '',
  };
}

const pages: PageEntry[] = range(1, 20).map(pageEntry);

const groups = [
  { pageNum: 1, images: [{ objNum: 12, gen: 0, path: ['Im1'] }] },
  { pageNum: 2, images: [{ objNum: 12, gen: 0, path: ['Im1'] }, { objNum: 19, gen: 0, path: ['Fm1', 'Im0'] }] },
  { pageNum: 3, images: [{ objNum: 7, gen: 0, path: ['Im7'] }, { objNum: 23, gen: 0, path: ['Mask'] }] },
  { pageNum: 4, images: [] },
];

const catalogNode = {
  id: 'root',
  label: 'Catalog',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 1,
  iconHint: 'catalog',
  error: '',
};

const rootChildren = [
  {
    id: 'obj:0:2',
    label: 'Pages',
    rawKey: '/Pages',
    nodeType: 'dict',
    valueType: 'reference',
    hasChildren: true,
    childCount: 1,
    iconHint: 'pages',
    error: '',
    objectRef: '2 0 R',
    typeName: 'Pages',
  },
];

function streamDetail(nodeId: string) {
  return {
    nodeId,
    objectRef: nodeId.replace(/^obj:(\d+):(\d+)$/, '$2 $1 R'),
    type: 'stream',
    properties: [],
    elements: [],
    scalarValue: null,
    streamInfo: { filter: 'DCTDecode', length: 1000 },
  };
}

type FullState = AppState & { leftView?: string };
let state: FullState;
let dispatch: Dispatch<AppAction>;

function Probe() {
  state = useAppState() as FullState;
  dispatch = useAppDispatch();
  return null;
}

function renderLayout() {
  return render(
    <AppProvider>
      <Probe />
      <MainLayout />
    </AppProvider>,
  );
}

function openTab(tabId = 'tab-1', pageCount = 20) {
  act(() =>
    dispatch({
      type: 'OPEN_DOCUMENT',
      payload: { tabId, fileName: `${tabId}.pdf`, filePath: `/tmp/${tabId}.pdf`, pageCount, rootNode: catalogNode, rootChildren },
    }),
  );
}

function activeTab() {
  return state.tabs.find((t) => t.tabId === state.activeTabId)!;
}

function rail() {
  const lists = screen.getAllByRole('tablist').filter((l) => l.getAttribute('aria-orientation') === 'vertical');
  expect(lists).toHaveLength(1);
  return lists[0];
}

function tab(name: string) {
  return within(rail()).getByRole('tab', { name });
}

function panelFor(name: string) {
  const panel = document.getElementById(tab(name).getAttribute('aria-controls') ?? '');
  expect(panel).not.toBeNull();
  return panel!;
}

function pressDigit(key: string) {
  fireEvent.keyDown(window, { key, metaKey: true, ctrlKey: true });
}

async function showImages(user: ReturnType<typeof userEvent.setup>) {
  await user.click(tab('Images'));
  const panel = panelFor('Images');
  await waitFor(() => expect(within(panel).getAllByTestId('image-row').length).toBeGreaterThan(0));
  return panel;
}

function imageRows(panel: HTMLElement) {
  return within(panel).queryAllByTestId('image-row');
}

function pageRows(panel: HTMLElement) {
  return within(panel).queryAllByTestId('image-page-row');
}

function imageRow(panel: HTMLElement, text: string | RegExp) {
  const row = imageRows(panel).find((r) =>
    typeof text === 'string' ? (r.textContent ?? '').includes(text) : text.test(r.textContent ?? ''),
  );
  expect(row, `no image row matching ${String(text)}`).toBeDefined();
  return row!;
}

function pageRow(panel: HTMLElement, pageNum: number) {
  const re = new RegExp(`\\bPage ${pageNum}\\b`);
  const row = pageRows(panel).find((r) => re.test(r.textContent ?? ''));
  expect(row, `no page row for page ${pageNum}`).toBeDefined();
  return row!;
}

function hasPageRow(panel: HTMLElement, pageNum: number) {
  const re = new RegExp(`\\bPage ${pageNum}\\b`);
  return pageRows(panel).some((r) => re.test(r.textContent ?? ''));
}

// Every title attribute on the row or inside it, joined.
function titles(row: HTMLElement) {
  const own = row.getAttribute('title') ?? '';
  const inner = Array.from(row.querySelectorAll('[title]')).map((el) => el.getAttribute('title') ?? '');
  const item = row.closest('[role="treeitem"]')?.getAttribute('title') ?? '';
  return [own, item, ...inner].join('\n');
}

function button(panel: HTMLElement, name: string) {
  return within(panel).getByRole('button', { name });
}

// Expands a row from the keyboard: arborist opens the focused node on
// ArrowRight, whichever renderer draws it.
async function expand(user: ReturnType<typeof userEvent.setup>, row: HTMLElement) {
  await user.click(row);
  await user.keyboard('{ArrowRight}');
}

beforeEach(() => {
  (globalThis as Record<string, unknown>).ResizeObserver = MockResizeObserver;
  window.localStorage.removeItem(RAIL_KEY);
  mockGetImageIndex.mockReset().mockResolvedValue(entries);
  mockGetImagePages.mockReset().mockImplementation((_tab: string, objNum: number) =>
    Promise.resolve(objNum === 12 ? range(1, 20) : []),
  );
  mockGetImagePageGroups.mockReset().mockResolvedValue(groups);
  mockGetPageIndex.mockReset().mockResolvedValue(pages);
  mockGetChildren.mockReset().mockResolvedValue([]);
  mockGetAncestorPath.mockReset().mockResolvedValue(['root', 'obj:0:12']);
  mockGetObjectSource.mockReset().mockResolvedValue('');
  mockGetObjectDetail.mockReset().mockImplementation((_tab: string, id: string) => Promise.resolve(streamDetail(id)));
  mockDescribeImage.mockReset().mockResolvedValue({ nodeId: '', objectRef: '', width: 1240, height: 800, estimatedBytes: 2976000, warning: '', error: '' });
  mockGetImageData.mockReset().mockResolvedValue({ kind: 'error', error: 'not decoded in this test', base64: '', mimeType: '' });
});

afterEach(() => {
  delete (globalThis as Record<string, unknown>).ResizeObserver;
});

describe('rail registration', () => {
  test('Images is the third rail item, with a visible label and an icon', () => {
    renderLayout();
    const tabs = within(rail()).getAllByRole('tab');
    expect(tabs.map((t) => t.getAttribute('aria-label'))).toEqual(['Structure', 'Pages', 'Images']);
    const images = tabs[2];
    expect(within(images).getByText('Images')).toBeVisible();
    expect(images.querySelector('svg')).not.toBeNull();
  });

  test('Cmd/Ctrl+3 selects Images and 4 is out of range', () => {
    renderLayout();
    pressDigit('3');
    expect(tab('Images')).toHaveAttribute('aria-selected', 'true');
    pressDigit('4');
    expect(tab('Images')).toHaveAttribute('aria-selected', 'true');
  });
});

describe('no document', () => {
  test('says no document is open and does not fetch', async () => {
    const user = userEvent.setup();
    renderLayout();
    await user.click(tab('Images'));
    expect(panelFor('Images').textContent).toContain('No document open.');
    expect(mockGetImageIndex).not.toHaveBeenCalled();
    expect(mockGetImagePageGroups).not.toHaveBeenCalled();
  });
});

describe('flat rows', () => {
  test('first activation fetches the image index and the page index for the tab', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    expect(mockGetImageIndex).not.toHaveBeenCalled();
    await showImages(user);
    expect(mockGetImageIndex).toHaveBeenCalledWith('tab-1');
    expect(mockGetPageIndex).toHaveBeenCalledWith('tab-1');
    expect(mockGetImagePageGroups).not.toHaveBeenCalled();
  });

  test('a row shows the reference, dimensions, filter, colour space and use count', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    const text = imageRow(panel, '12 0 R').textContent ?? '';
    for (const part of ['1240x800', 'DCTDecode', 'DeviceRGB', 'used on 20 pages']) {
      expect(text).toContain(part);
    }
    const two = imageRow(panel, '19 0 R').textContent ?? '';
    expect(two).toContain('640x480');
    expect(two).toContain('FlateDecode');
    expect(two).toContain('DeviceGray');
    expect(two).toContain('used on 2 pages');
  });

  test('a single use is singular, and an empty filter list and colour space read as -', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    const text = imageRow(panel, '23 0 R').textContent ?? '';
    expect(text).toContain('used on 1 page');
    expect(text).not.toContain('used on 1 pages');
    expect(text).toMatch(/2480x3508\s*-\s*-/);
  });

  test('rows are text: no image, canvas or placeholder slot', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    expect(panel.querySelectorAll('img, canvas')).toHaveLength(0);
  });

  test('the badge strip names each fact in text', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    const logoText = imageRow(panel, '12 0 R').textContent ?? '';
    expect(logoText).toContain('SMask');
    expect(logoText).toContain('APP14 t=2');
    expect(logoText).not.toMatch(/\bwarn\b/);
    expect(logoText).not.toMatch(/\bmask\b/);
    expect(logoText).not.toMatch(/\bDecode\b/);

    const invText = imageRow(panel, '19 0 R').textContent ?? '';
    expect(invText).toMatch(/\bDecode\b/);
    expect(invText).not.toContain('SMask');

    const maskText = imageRow(panel, '23 0 R').textContent ?? '';
    expect(maskText).toMatch(/\bmask\b/);
    expect(maskText).toMatch(/\bwarn\b/);
  });

  test('the row title carries the sample interpretation and the warning', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    expect(titles(imageRow(panel, '19 0 R'))).toContain('Inverted by /Decode');
    const maskTitle = titles(imageRow(panel, '23 0 R'));
    expect(maskTitle).toContain('Normal (default)');
    expect(maskTitle).toContain('Width metadata: not an integer');
  });

  test('the header counts images, not error rows', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    expect(panel.textContent).toMatch(/\b4 images\b/);
  });

  test('the default order is the walk order, error row last', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    const text = imageRows(panel).map((r) => r.textContent ?? '');
    expect(text[0]).toContain('12 0 R');
    expect(text[1]).toContain('19 0 R');
    expect(text[2]).toContain('7 0 R');
    expect(text[3]).toContain('23 0 R');
    expect(text[4]).toContain(WALK_STOPPED);
  });
});

describe('error rows', () => {
  test('an error row shows - for the reference and its error text', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    const row = imageRow(panel, WALK_STOPPED);
    expect(row.textContent).toMatch(/^\s*-/);
    expect(titles(row)).toContain(WALK_STOPPED);
  });

  test('an error row is not selectable and not expandable', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await user.click(imageRow(panel, WALK_STOPPED));
    expect(activeTab().selectedNodeId).toBeNull();
    await user.keyboard('{ArrowRight}');
    expect(pageRows(panel)).toHaveLength(0);
  });

  test('arrow keys do not land the selection on an error row', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await user.click(imageRow(panel, '23 0 R'));
    expect(activeTab().selectedNodeId).toBe('obj:0:23');
    await user.keyboard('{ArrowDown}');
    expect(activeTab().selectedNodeId).toBe('obj:0:23');
  });

  test('an image whose facts read failed shows the error and stays selectable', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    const row = imageRow(panel, '7 0 R');
    expect(row.textContent).toContain('image facts read failed: boom');
    await user.click(row);
    expect(activeTab().selectedNodeId).toBe('obj:0:7');
  });
});

describe('sort', () => {
  test('object-number order reorders without a refetch and keeps the error row last', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await user.click(button(panel, 'Object number'));
    expect(button(panel, 'Object number')).toHaveAttribute('aria-pressed', 'true');
    expect(button(panel, 'Object number')).toHaveClass('bg-surface-armed');
    expect(button(panel, 'First use')).not.toHaveClass('bg-surface-armed');
    const text = imageRows(panel).map((r) => r.textContent ?? '');
    expect(text[0]).toContain('7 0 R');
    expect(text[1]).toContain('12 0 R');
    expect(text[2]).toContain('19 0 R');
    expect(text[3]).toContain('23 0 R');
    expect(text[4]).toContain(WALK_STOPPED);
    expect(mockGetImageIndex).toHaveBeenCalledTimes(1);

    await user.click(button(panel, 'First use'));
    expect(imageRows(panel)[0].textContent).toContain('12 0 R');
    expect(mockGetImageIndex).toHaveBeenCalledTimes(1);
  });
});

describe('flat expansion', () => {
  test('expanding a row lists the pages from firstPages without another fetch', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    expect(pageRows(panel)).toHaveLength(0);
    await expand(user, imageRow(panel, '19 0 R'));
    await waitFor(() => expect(hasPageRow(panel, 3)).toBe(true));
    expect(hasPageRow(panel, 2)).toBe(true);
    expect(pageRows(panel)).toHaveLength(2);
    expect(mockGetImagePages).not.toHaveBeenCalled();
  });

  test('expanding a row past the first-pages cap fetches and lists every page', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await expand(user, imageRow(panel, '12 0 R'));
    await waitFor(() => expect(mockGetImagePages).toHaveBeenCalledWith('tab-1', 12));
    await waitFor(() => expect(hasPageRow(panel, 17)).toBe(true));
  });

  test('clicking a page child selects that page node', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await expand(user, imageRow(panel, '19 0 R'));
    await waitFor(() => expect(hasPageRow(panel, 2)).toBe(true));
    await user.click(pageRow(panel, 2));
    expect(activeTab().selectedNodeId).toBe('obj:0:102');
    expect(activeTab().selectedNodeIconHint).toBe('page');
  });

  test('a slow page fetch that resolves after a tab switch lands on its own tab only', async () => {
    let resolvePages: (v: number[]) => void = () => {};
    mockGetImagePages.mockReset().mockImplementation(
      () =>
        new Promise<number[]>((r) => {
          resolvePages = r;
        }),
    );
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1');
    const first = await showImages(user);
    await expand(user, imageRow(first, '12 0 R'));
    await waitFor(() => expect(mockGetImagePages).toHaveBeenCalledWith('tab-1', 12));

    openTab('tab-2');
    const second = await showImages(user);
    await act(async () => resolvePages(range(1, 20)));
    expect(hasPageRow(second, 17)).toBe(false);

    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } }));
    await waitFor(() => expect(hasPageRow(panelFor('Images'), 17)).toBe(true));
  });
});

describe('grouping by page', () => {
  test('the view toggle is two pressed-state buttons, flat by default', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    expect(button(panel, 'Flat')).toHaveAttribute('aria-pressed', 'true');
    expect(button(panel, 'By page')).toHaveAttribute('aria-pressed', 'false');
  });

  test('switching to By page fetches the groups once and lists every page', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await user.click(button(panel, 'By page'));
    expect(button(panel, 'By page')).toHaveAttribute('aria-pressed', 'true');
    await waitFor(() => expect(hasPageRow(panel, 3)).toBe(true));
    expect(mockGetImagePageGroups).toHaveBeenCalledWith('tab-1');
    for (const n of [1, 2, 3]) expect(hasPageRow(panel, n)).toBe(true);

    await user.click(button(panel, 'Flat'));
    await user.click(button(panel, 'By page'));
    expect(mockGetImagePageGroups).toHaveBeenCalledTimes(1);
  });

  test('a page with no images is not listed', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await user.click(button(panel, 'By page'));
    await waitFor(() => expect(hasPageRow(panel, 3)).toBe(true));
    expect(hasPageRow(panel, 4)).toBe(false);
  });

  test('an image used by two pages appears under each, with its resource path', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await user.click(button(panel, 'By page'));
    await waitFor(() => expect(hasPageRow(panel, 2)).toBe(true));
    await expand(user, pageRow(panel, 1));
    await expand(user, pageRow(panel, 2));
    await waitFor(() => expect(imageRows(panel).filter((r) => (r.textContent ?? '').includes('12 0 R'))).toHaveLength(2));
    expect(imageRow(panel, '19 0 R').textContent).toContain('Fm1 > Im0');
  });

  test('each view keeps its own expansion across a round trip', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await expand(user, imageRow(panel, '19 0 R'));
    await waitFor(() => expect(hasPageRow(panel, 3)).toBe(true));

    await user.click(button(panel, 'By page'));
    await waitFor(() => expect(hasPageRow(panel, 1)).toBe(true));
    expect(imageRows(panel).some((r) => (r.textContent ?? '').includes('used on'))).toBe(false);

    await user.click(button(panel, 'Flat'));
    await waitFor(() => expect(imageRow(panel, '19 0 R')).toBeDefined());
    expect(hasPageRow(panel, 3)).toBe(true);
    expect(hasPageRow(panel, 4)).toBe(false);
  });

  test('a failed groups fetch shows inline and is retried on the next switch', async () => {
    mockGetImagePageGroups.mockReset().mockRejectedValueOnce(new Error('groups unreadable')).mockResolvedValue(groups);
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await user.click(button(panel, 'By page'));
    await waitFor(() => expect(panel.textContent).toContain('groups unreadable'));
    await user.click(button(panel, 'Flat'));
    await user.click(button(panel, 'By page'));
    await waitFor(() => expect(hasPageRow(panel, 3)).toBe(true));
    expect(mockGetImagePageGroups).toHaveBeenCalledTimes(2);
  });
});

describe('grouping by page across generations', () => {
  test('a use whose reference names another generation shows its entry', async () => {
    mockGetImagePageGroups.mockReset().mockResolvedValue([
      { pageNum: 1, images: [{ objNum: 12, gen: 0, path: ['Im1'] }] },
      { pageNum: 2, images: [{ objNum: 12, gen: 1, path: ['Im1'] }] },
    ]);
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await user.click(button(panel, 'By page'));
    await waitFor(() => expect(hasPageRow(panel, 2)).toBe(true));
    await expand(user, pageRow(panel, 1));
    await expand(user, pageRow(panel, 2));
    await waitFor(() => expect(imageRows(panel).filter((r) => (r.textContent ?? '').includes('12 0 R'))).toHaveLength(2));
    expect(imageRows(panel).some((r) => (r.textContent ?? '').includes('obj:1:12'))).toBe(false);
  });
});

describe('grouping by page with a stopped walk', () => {
  test('the walk\'s error rows follow the pages, so unwalked pages are not read as empty', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await user.click(button(panel, 'By page'));
    await waitFor(() => expect(hasPageRow(panel, 3)).toBe(true));
    const rows = imageRows(panel);
    expect(rows).toHaveLength(1);
    expect(rows[0].textContent).toContain(WALK_STOPPED);
  });
});

describe('grouping by page with an incomplete page', () => {
  test('a page the walk did not finish says so instead of claiming no images', async () => {
    mockGetImagePageGroups.mockReset().mockResolvedValue([
      { pageNum: 1, images: [{ objNum: 12, gen: 0, path: ['Im1'] }], incomplete: false },
      { pageNum: 2, images: [{ objNum: 19, gen: 0, path: ['Fm1', 'Im0'] }], incomplete: true },
      { pageNum: 3, images: [], incomplete: true },
      { pageNum: 4, images: [], incomplete: false },
    ]);
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await user.click(button(panel, 'By page'));
    await waitFor(() => expect(hasPageRow(panel, 3)).toBe(true));
    expect(pageRow(panel, 1).textContent).not.toContain('walk incomplete');
    expect(pageRow(panel, 2).textContent).toContain('(walk incomplete)');
    expect(pageRow(panel, 3).textContent).toContain('(walk incomplete)');
    expect(hasPageRow(panel, 4)).toBe(false);
  });
});

describe('page index unavailable', () => {
  test('image rows still render and select; page rows do not, and the header says why', async () => {
    mockGetPageIndex.mockReset().mockRejectedValue(new Error('the page tree could not be read past page 3'));
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    expect(panel.textContent).toContain(PAGE_INDEX_NOTE);
    await user.click(imageRow(panel, '19 0 R'));
    expect(activeTab().selectedNodeId).toBe('obj:0:19');
    await user.keyboard('{ArrowRight}');
    await waitFor(() => expect(hasPageRow(panel, 2)).toBe(true));
    await user.click(pageRow(panel, 2));
    expect(activeTab().selectedNodeId).toBe('obj:0:19');
    fireEvent.contextMenu(pageRow(panel, 2));
    expect(screen.queryByRole('menu')).toBeNull();
  });

  test('a page whose index entry has no node id is not selectable', async () => {
    mockGetPageIndex.mockReset().mockResolvedValue(pages.map((p) => (p.pageNum === 2 ? { ...p, nodeId: '' } : p)));
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await expand(user, imageRow(panel, '19 0 R'));
    await waitFor(() => expect(hasPageRow(panel, 2)).toBe(true));
    await user.click(pageRow(panel, 2));
    expect(activeTab().selectedNodeId).toBe('obj:0:19');
    await user.click(pageRow(panel, 3));
    expect(activeTab().selectedNodeId).toBe('obj:0:103');
  });
});

describe('selection', () => {
  test('clicking an image row selects it with the image icon hint, and the detail panel requests the image', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await user.click(imageRow(panel, '12 0 R'));
    expect(activeTab().selectedNodeId).toBe('obj:0:12');
    expect(activeTab().selectedNodeIconHint).toBe('image');
    expect(activeTab().selectedNodeLabel).toBe('Image 12 0 R');
    await waitFor(() => expect(mockGetObjectDetail).toHaveBeenCalledWith('tab-1', 'obj:0:12'));
    await waitFor(() => expect(mockDescribeImage).toHaveBeenCalledWith('tab-1', 'obj:0:12'));
  });

  test('a selection made elsewhere is not re-dispatched by the Images panel', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await showImages(user);
    const before = activeTab().navHistory.length;
    act(() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:12', label: 'Im1', rawKey: '/Im1', iconHint: 'image' } }));
    await waitFor(() => expect(activeTab().selectedNodeId).toBe('obj:0:12'));
    await new Promise((r) => setTimeout(r, 20));
    expect(activeTab().selectedNodeLabel).toBe('Im1');
    expect(activeTab().navHistory.length).toBe(before + 1);
  });

  test('a page selected elsewhere does not expand an image row in the flat view', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    expect(pageRows(panel)).toHaveLength(0);
    act(() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:102', label: 'Page 2', rawKey: '', iconHint: 'page' } }));
    await waitFor(() => expect(activeTab().selectedNodeId).toBe('obj:0:102'));
    await new Promise((r) => setTimeout(r, 20));
    expect(pageRows(panel)).toHaveLength(0);

    await expand(user, imageRow(panel, '19 0 R'));
    await waitFor(() => expect(hasPageRow(panel, 2)).toBe(true));
    act(() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:103', label: 'Page 3', rawKey: '', iconHint: 'page' } }));
    await waitFor(() => {
      const selected = panel.querySelectorAll('[role="treeitem"][aria-selected="true"]');
      expect(selected).toHaveLength(1);
      expect(selected[0].contains(pageRow(panel, 3))).toBe(true);
    });
  });

  test('re-selecting a page picked here under a since-collapsed row does not expand it', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await expand(user, imageRow(panel, '19 0 R'));
    await waitFor(() => expect(hasPageRow(panel, 2)).toBe(true));
    await user.click(pageRow(panel, 2));
    expect(activeTab().selectedNodeId).toBe('obj:0:102');
    const chevron = imageRow(panel, '19 0 R').querySelector('span');
    expect(chevron?.textContent).toBe('v');
    await user.click(chevron!);
    await waitFor(() => expect(pageRows(panel)).toHaveLength(0));

    act(() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:103', label: 'Page 3', rawKey: '', iconHint: 'page' } }));
    await waitFor(() => expect(activeTab().selectedNodeId).toBe('obj:0:103'));
    act(() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:102', label: 'Page 2', rawKey: '', iconHint: 'page' } }));
    await waitFor(() => expect(activeTab().selectedNodeId).toBe('obj:0:102'));
    await new Promise((r) => setTimeout(r, 20));
    expect(pageRows(panel)).toHaveLength(0);
  });

  test('expanding a row by its chevron highlights a page selected elsewhere', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    act(() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:102', label: 'Page 2', rawKey: '', iconHint: 'page' } }));
    await waitFor(() => expect(activeTab().selectedNodeId).toBe('obj:0:102'));
    const chevron = imageRow(panel, '19 0 R').querySelector('span');
    expect(chevron?.textContent).toBe('>');
    await user.click(chevron!);
    await waitFor(() => expect(hasPageRow(panel, 2)).toBe(true));
    expect(activeTab().selectedNodeId).toBe('obj:0:102');
    await waitFor(() => {
      const selected = panel.querySelectorAll('[role="treeitem"][aria-selected="true"]');
      expect(selected).toHaveLength(1);
      expect(selected[0].contains(pageRow(panel, 2))).toBe(true);
    });
  });

  test('an image listed under two pages keeps the highlight on the listing clicked', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await user.click(button(panel, 'By page'));
    await waitFor(() => expect(hasPageRow(panel, 2)).toBe(true));
    await expand(user, pageRow(panel, 1));
    await expand(user, pageRow(panel, 2));
    await waitFor(() => expect(imageRows(panel).filter((r) => (r.textContent ?? '').includes('12 0 R'))).toHaveLength(2));
    const second = imageRows(panel).filter((r) => (r.textContent ?? '').includes('12 0 R'))[1];
    await user.click(second);
    expect(activeTab().selectedNodeId).toBe('obj:0:12');
    await waitFor(() => {
      const selected = panel.querySelectorAll('[role="treeitem"][aria-selected="true"]');
      expect(selected).toHaveLength(1);
      expect(selected[0].contains(second)).toBe(true);
    });
  });
});

describe('Show node in tree', () => {
  test('right-click on an image row opens one labelled menu item and suppresses the WebView menu', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    const ev = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
    act(() => {
      imageRow(panel, '12 0 R').dispatchEvent(ev);
    });
    expect(ev.defaultPrevented).toBe(true);
    const menu = screen.getByRole('menu', { name: 'Image row actions' });
    expect(within(menu).getAllByRole('menuitem').map((m) => m.textContent)).toEqual(['Show node in tree']);
  });

  test('activating it switches the rail to Structure and reveals the image', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    fireEvent.contextMenu(imageRow(panel, '12 0 R'));
    await user.click(screen.getByRole('menuitem', { name: 'Show node in tree' }));
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByRole('menu')).toBeNull();
    await waitFor(() => expect(mockGetAncestorPath).toHaveBeenCalledWith('tab-1', 'obj:0:12'));
  });

  test('a page child row offers the menu for its /Page node', async () => {
    mockGetAncestorPath.mockReset().mockResolvedValue(['root', 'obj:0:2', 'obj:0:102']);
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await expand(user, imageRow(panel, '19 0 R'));
    await waitFor(() => expect(hasPageRow(panel, 2)).toBe(true));
    fireEvent.contextMenu(pageRow(panel, 2));
    await user.click(screen.getByRole('menuitem', { name: 'Show node in tree' }));
    await waitFor(() => expect(mockGetAncestorPath).toHaveBeenCalledWith('tab-1', 'obj:0:102'));
  });

  test('Shift+F10 on a focused row opens the menu, and Escape closes it returning focus', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    const row = imageRow(panel, '12 0 R');
    await user.click(row);
    await user.keyboard('{Shift>}{F10}{/Shift}');
    expect(screen.getByRole('menu')).toBeInTheDocument();
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('menu')).toBeNull();
    expect(row.contains(document.activeElement) || row.closest('[role="treeitem"]')?.contains(document.activeElement)).toBe(true);
  });

  test('an error row opens no menu', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    fireEvent.contextMenu(imageRow(panel, WALK_STOPPED));
    expect(screen.queryByRole('menu')).toBeNull();
  });

  test('the Pages navigator keeps its own menu label', async () => {
    mockGetPageIndex.mockReset().mockResolvedValue(pages.slice(0, 3));
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1', 3);
    await user.click(tab('Pages'));
    const pagesPanel = panelFor('Pages');
    await waitFor(() => expect(within(pagesPanel).getAllByTestId('tree-node').length).toBeGreaterThan(0));
    fireEvent.contextMenu(within(pagesPanel).getAllByTestId('tree-node')[0]);
    expect(screen.getByRole('menu', { name: 'Page row actions' })).toBeInTheDocument();
    await user.keyboard('{Escape}');
    await user.click(tab('Images'));
    const panel = panelFor('Images');
    await waitFor(() => expect(imageRows(panel).length).toBeGreaterThan(0));
    fireEvent.contextMenu(imageRow(panel, '12 0 R'));
    expect(screen.getByRole('menu', { name: 'Image row actions' })).toBeInTheDocument();
  });
});

describe('empty states', () => {
  test('a document with no images shows the explicit empty state', async () => {
    mockGetImageIndex.mockReset().mockResolvedValue([]);
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Images'));
    const panel = panelFor('Images');
    await waitFor(() => expect(panel.textContent).toContain(EMPTY_COPY));
    expect(imageRows(panel)).toHaveLength(0);
  });

  test('a document with no pages and no images shows the empty state in the by-page view too', async () => {
    mockGetImageIndex.mockReset().mockResolvedValue([]);
    mockGetImagePageGroups.mockReset().mockResolvedValue([]);
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Images'));
    const panel = panelFor('Images');
    await waitFor(() => expect(panel.textContent).toContain(EMPTY_COPY));
    await user.click(button(panel, 'By page'));
    await waitFor(() => expect(mockGetImagePageGroups).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(panel.textContent).not.toContain('Loading pages...'));
    expect(panel.textContent).toContain(EMPTY_COPY);
    expect(pageRows(panel)).toHaveLength(0);
  });

  test('an index holding only an error row lists it under 0 images, without the empty copy', async () => {
    mockGetImageIndex.mockReset().mockResolvedValue([errorRowEntry()]);
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    expect(panel.textContent).toMatch(/\b0 images\b/);
    expect(imageRow(panel, WALK_STOPPED)).toBeDefined();
    expect(panel.textContent).not.toContain('No image XObjects are referenced');
  });

  test('a failed fetch shows the error inline', async () => {
    mockGetImageIndex.mockReset().mockRejectedValue(new Error('image walk failed'));
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Images'));
    await waitFor(() => expect(panelFor('Images').textContent).toContain('image walk failed'));
  });

  test('a failure with an empty message shows the error banner without the loading line', async () => {
    mockGetImageIndex.mockReset().mockRejectedValue(new Error(''));
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Images'));
    const panel = panelFor('Images');
    await waitFor(() => expect(panel.textContent).toContain('Could not load the image index:'));
    expect(panel.textContent).not.toContain('Loading images...');
  });

  test('a fetch that failed is retried on the next activation', async () => {
    mockGetImageIndex.mockReset().mockRejectedValueOnce(new Error('image walk failed')).mockResolvedValue(entries);
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Images'));
    await waitFor(() => expect(panelFor('Images').textContent).toContain('image walk failed'));
    await user.click(tab('Structure'));
    await showImages(user);
    expect(mockGetImageIndex).toHaveBeenCalledTimes(2);
    expect(panelFor('Images').textContent).not.toContain('image walk failed');
  });
});

describe('per-tab state', () => {
  test('the view and sort persist per tab, and a new tab starts flat in first-use order', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1');
    let panel = await showImages(user);
    await user.click(button(panel, 'Object number'));
    await user.click(button(panel, 'By page'));
    await waitFor(() => expect(hasPageRow(panel, 3)).toBe(true));

    openTab('tab-2');
    panel = await showImages(user);
    expect(button(panel, 'Flat')).toHaveAttribute('aria-pressed', 'true');
    expect(button(panel, 'First use')).toHaveAttribute('aria-pressed', 'true');

    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } }));
    panel = panelFor('Images');
    await waitFor(() => expect(button(panel, 'By page')).toHaveAttribute('aria-pressed', 'true'));
    await user.click(button(panel, 'Flat'));
    expect(button(panel, 'Object number')).toHaveAttribute('aria-pressed', 'true');
    expect(button(panel, 'Object number')).toHaveClass('bg-surface-armed');
    expect(button(panel, 'First use')).not.toHaveClass('bg-surface-armed');
    expect(mockGetImagePageGroups.mock.calls.filter((c) => c[0] === 'tab-1')).toHaveLength(1);
  });

  test('switching back to a tab does not refetch; closing it evicts', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1');
    await showImages(user);
    openTab('tab-2');
    await showImages(user);
    act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-1' } }));
    await waitFor(() => expect(imageRows(panelFor('Images')).length).toBeGreaterThan(0));
    expect(mockGetImageIndex.mock.calls.filter((c) => c[0] === 'tab-1')).toHaveLength(1);

    act(() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-1' } }));
    openTab('tab-1');
    await showImages(user);
    expect(mockGetImageIndex.mock.calls.filter((c) => c[0] === 'tab-1')).toHaveLength(2);
  });
});

describe('closed-tab eviction of pending requests', () => {
  test('an index fetch still pending when its tab closes does not block the tab reopened under that id', async () => {
    mockGetImageIndex.mockReset().mockReturnValueOnce(new Promise(() => {})).mockResolvedValue(entries);
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1');
    await user.click(tab('Images'));
    await waitFor(() => expect(mockGetImageIndex).toHaveBeenCalledTimes(1));

    act(() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-1' } }));
    openTab('tab-1');
    await showImages(user);
    expect(mockGetImageIndex).toHaveBeenCalledTimes(2);
  });

  test('a page fetch from a closed tab does not land on the tab reopened under that id', async () => {
    const pending: ((v: number[]) => void)[] = [];
    mockGetImagePages.mockReset().mockImplementation(
      () =>
        new Promise<number[]>((r) => {
          pending.push(r);
        }),
    );
    const user = userEvent.setup();
    renderLayout();
    openTab('tab-1');
    let panel = await showImages(user);
    await expand(user, imageRow(panel, '12 0 R'));
    await waitFor(() => expect(pending).toHaveLength(1));

    act(() => dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: 'tab-1' } }));
    openTab('tab-1');
    panel = await showImages(user);
    await expand(user, imageRow(panel, '12 0 R'));
    await waitFor(() => expect(pending).toHaveLength(2));

    await act(async () => pending[1](range(1, 20)));
    await waitFor(() => expect(hasPageRow(panel, 17)).toBe(true));
    await act(async () => pending[0]([1]));
    expect(hasPageRow(panel, 17)).toBe(true);
  });
});

describe('virtualization', () => {
  test('2000 images render a window of rows, not all of them', async () => {
    mockGetImageIndex.mockReset().mockResolvedValue(Array.from({ length: 2000 }, (_, i) => img(10 + i)));
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    expect(panel.textContent).toMatch(/\b2000 images\b/);
    const rendered = imageRows(panel).length;
    expect(rendered).toBeGreaterThan(0);
    expect(rendered).toBeLessThan(100);
  });
});

describe('page list fetch failures and progress', () => {
  test('a failed full page fetch leaves the row empty and expandable, and re-expanding refetches', async () => {
    mockGetImagePages.mockReset().mockRejectedValueOnce(new Error('walk gone')).mockResolvedValue(range(1, 20));
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await expand(user, imageRow(panel, '12 0 R'));
    await waitFor(() => expect(mockGetImagePages).toHaveBeenCalledTimes(1));
    expect(pageRows(panel)).toHaveLength(0);

    await user.keyboard('{ArrowLeft}');
    await user.keyboard('{ArrowRight}');
    await waitFor(() => expect(mockGetImagePages).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(hasPageRow(panel, 20)).toBe(true));
  });

  test('a slow full page fetch pulses the row chevron until it answers', async () => {
    let resolvePages: (v: number[]) => void = () => {};
    mockGetImagePages.mockReset().mockImplementation(
      () =>
        new Promise<number[]>((r) => {
          resolvePages = r;
        }),
    );
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await expand(user, imageRow(panel, '12 0 R'));
    const pulsing = () => imageRow(panel, '12 0 R').querySelector('.animate-pulse');
    expect(pulsing()).toBeNull();
    await waitFor(() => expect(pulsing()).not.toBeNull());
    await act(async () => resolvePages(range(1, 20)));
    expect(pulsing()).toBeNull();
    expect(hasPageRow(panel, 20)).toBe(true);
  });
});

describe('grouping by page with a use the index does not list', () => {
  test('the use shows its node id and selects it by that id', async () => {
    mockGetImagePageGroups.mockReset().mockResolvedValue([{ pageNum: 1, images: [{ objNum: 99, gen: 0, path: ['Ghost'] }] }]);
    // The detail panel cancels the image decode it starts when the layout unmounts.
    mockGetImageData.mockReset().mockImplementation(() =>
      Object.assign(Promise.resolve({ kind: 'error', error: 'not decoded in this test', base64: '', mimeType: '' }), { cancel: () => {} }),
    );
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await user.click(button(panel, 'By page'));
    await waitFor(() => expect(hasPageRow(panel, 1)).toBe(true));
    await expand(user, pageRow(panel, 1));
    const ghost = await waitFor(() => imageRow(panel, 'obj:0:99'));
    await user.click(ghost);
    expect(activeTab().selectedNodeId).toBe('obj:0:99');
    expect(activeTab().selectedNodeIconHint).toBe('image');
  });
});

describe('null-tolerant responses', () => {
  test('a null image index is the empty state', async () => {
    mockGetImageIndex.mockReset().mockResolvedValue(null);
    const user = userEvent.setup();
    renderLayout();
    openTab();
    await user.click(tab('Images'));
    const panel = panelFor('Images');
    expect(await within(panel).findByText(EMPTY_COPY)).toBeInTheDocument();
    expect(within(panel).getByText('0 images')).toBeInTheDocument();
  });

  test('null entries in the index and the groups are dropped', async () => {
    mockGetImageIndex.mockReset().mockResolvedValue([null, inverted, null]);
    mockGetImagePageGroups.mockReset().mockResolvedValue([null, { pageNum: 2, images: [{ objNum: 19, gen: 0, path: ['Fm1', 'Im0'] }] }]);
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    expect(imageRows(panel)).toHaveLength(1);
    expect(within(panel).getByText('1 image')).toBeInTheDocument();
    await user.click(button(panel, 'By page'));
    await waitFor(() => expect(hasPageRow(panel, 2)).toBe(true));
    expect(pageRows(panel)).toHaveLength(1);
  });
});

describe('ContextMenu key', () => {
  test('the ContextMenu key on a focused row opens the menu', async () => {
    const user = userEvent.setup();
    renderLayout();
    openTab();
    const panel = await showImages(user);
    await user.click(imageRow(panel, '19 0 R'));
    await user.keyboard('{ContextMenu}');
    expect(screen.getByRole('menu', { name: 'Image row actions' })).toBeInTheDocument();
  });
});
