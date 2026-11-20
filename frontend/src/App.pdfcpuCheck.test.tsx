/**
 * The background pdfcpu check: every new tab id that enters app state starts
 * one Validate(tabId, 'pdfcpu') call, and the result shows an overlay banner
 * for the active tab when pdfcpu rejected the file or recorded notices.
 * Details opens the detail panel's Validate tab on the pdfcpu profile and runs
 * it. The banner sits over the content and takes no layout space.
 *
 * The real MainLayout, DetailPanel, ValidateView, TabBar and ErrorBanner
 * render; the pdfservice bindings are mocked.
 *
 * Run: cd frontend && npx vitest run src/App.pdfcpuCheck.test.tsx
 */
import { render, screen, act, fireEvent, within, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import type { ReactNode } from 'react';

type EventHandler = (event: { data: Record<string, unknown> }) => void;
const eventHandlers: Record<string, EventHandler[]> = {};

vi.mock('@wailsio/runtime', async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  Events: {
    On: (name: string, handler: EventHandler) => {
      if (!eventHandlers[name]) eventHandlers[name] = [];
      eventHandlers[name].push(handler);
      return () => {
        const idx = eventHandlers[name]?.indexOf(handler) ?? -1;
        if (idx >= 0) eventHandlers[name].splice(idx, 1);
      };
    },
    Emit: vi.fn(),
  },
  Window: {
    SetSize: vi.fn().mockResolvedValue(undefined),
    SetPosition: vi.fn().mockResolvedValue(undefined),
    Position: vi.fn().mockResolvedValue({ x: 0, y: 0 }),
    Size: vi.fn().mockResolvedValue({ width: 1200, height: 800 }),
  },
  Screens: {
    GetAll: vi.fn().mockResolvedValue([]),
  },
}));

vi.mock('allotment', () => {
  function Pane({ children, visible }: { children: ReactNode; visible?: boolean }) {
    return <div data-testid="allotment-pane" data-visible={visible === false ? 'false' : 'true'}>{children}</div>;
  }
  function Allotment({ children, vertical }: { children: ReactNode; vertical?: boolean }) {
    return <div data-testid={vertical ? 'allotment-vertical' : 'allotment-horizontal'}>{children}</div>;
  }
  Allotment.Pane = Pane;
  return { Allotment };
});

vi.mock('allotment/dist/style.css', () => ({}));

const bindings = vi.hoisted(() => ({
  Validate: vi.fn(),
  OpenFile: vi.fn(),
  GetTreeRoot: vi.fn(),
  GetChildren: vi.fn(),
}));

vi.mock('../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  Validate: (...a: unknown[]) => bindings.Validate(...a),
  OpenFile: (...a: unknown[]) => bindings.OpenFile(...a),
  GetTreeRoot: (...a: unknown[]) => bindings.GetTreeRoot(...a),
  GetChildren: (...a: unknown[]) => bindings.GetChildren(...a),
  SetWatchedTabs: vi.fn().mockResolvedValue(undefined),
  CloseDocument: vi.fn().mockResolvedValue(undefined),
  GetDiskStatus: vi.fn().mockResolvedValue({ state: 'current' }),
  OpenFileDialog: vi.fn(),
  GetObjectDetail: vi.fn().mockResolvedValue(null),
  GetObjectSource: vi.fn().mockResolvedValue(''),
  GetReverseRefs: vi.fn().mockResolvedValue([]),
  GetAncestorPath: vi.fn().mockResolvedValue([]),
  GetXRefTable: vi.fn().mockResolvedValue({ tabId: '', entries: [] }),
  ConsumePendingOpenFiles: vi.fn().mockResolvedValue([]),
  GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
  GetSignatures: vi.fn().mockResolvedValue([]),
  GetEmbeddedFileBytes: vi.fn().mockResolvedValue(''),
  GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '', warning: '' }),
  SaveEmbeddedFileToFile: vi.fn().mockResolvedValue(''),
  DiffDocuments: vi.fn().mockResolvedValue({ root: null, summary: {} }),
  GetPageIndex: vi.fn().mockResolvedValue([]),
  GetImageIndex: vi.fn().mockResolvedValue([]),
  GetImagePages: vi.fn().mockResolvedValue([]),
  GetImagePageGroups: vi.fn().mockResolvedValue([]),
  GetContentStream: vi.fn().mockResolvedValue(null),
}));

vi.mock('./components/UpdateNotifier', () => ({
  UpdateNotifier: () => null,
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

function emitEvent(name: string, data: Record<string, unknown> = {}) {
  for (const handler of [...(eventHandlers[name] ?? [])]) handler({ data });
}

const catalogNode = {
  id: 'root',
  label: 'Catalog',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 0,
  iconHint: 'catalog',
  error: '',
};

const DISCLAIMER = 'structural checks only - not full conformance; use veraPDF for authoritative PDF/A / PDF/UA validation';
const RULES = [
  { ruleId: 'pdfcpu-validator', specRef: 'pdfcpu', severity: 'error', checks: 'relaxed validator accepts the document', evaluated: true, findings: 0 },
  { ruleId: 'pdfcpu-notices', specRef: 'pdfcpu', severity: 'warning', checks: 'repairs pdfcpu records', evaluated: true, findings: 0 },
];
const FATAL_MESSAGE = 'catalog: StructTreeRoot: structure tree root K obj#12: circular structure tree';

function problem(ruleId: string, severity: string, message: string, objNum: number) {
  return { ruleId, profile: 'pdfcpu', severity, message, objRef: `${objNum} 0 R`, objNodeId: `obj:0:${objNum}`, specRef: 'pdfcpu' };
}

function result(problems: ReturnType<typeof problem>[]) {
  const errors = problems.filter((p) => p.severity === 'error').length;
  const warnings = problems.filter((p) => p.severity === 'warning').length;
  const info = problems.filter((p) => p.severity === 'info').length;
  return { profile: 'pdfcpu', summary: { errors, warnings, info }, problems, disclaimer: DISCLAIMER, scope: 'separate parse', rules: RULES };
}

const emptyResult = result([]);
const fatalResult = result([problem('pdfcpu-validator', 'error', FATAL_MESSAGE, 12)]);
const notice = (n: number) => problem('pdfcpu-notices', 'warning', `digested during validate: notice ${n}`, 5 + n);
const oneNoticeResult = result([notice(1)]);
const twoNoticeResult = result([notice(1), notice(2)]);
const degradedResult = {
  ...result([
    { ...problem('pdfcpu-validator', 'info', 'rule "pdfcpu-validator" could not be evaluated: boom', 0), objRef: '', objNodeId: '' },
    { ...problem('pdfcpu-notices', 'info', 'rule "pdfcpu-notices" could not be evaluated: boom', 0), objRef: '', objNodeId: '' },
  ]),
  rules: RULES.map((r) => ({ ...r, evaluated: false })),
};

// openedDocuments.ts keeps module-level maps of released ids and paths, so
// every test uses its own ids and paths.
let n = 0;
function ids() {
  n += 1;
  return { a: `pc-a${n}`, b: `pc-b${n}`, fresh: `pc-fresh${n}`, pathA: `/pc/${n}/a.pdf`, pathB: `/pc/${n}/b.pdf` };
}

/** Validate results keyed by tab id; a tab without an entry gets the empty result. */
let results: Record<string, unknown> = {};

function deliverOpen(tabId: string, filePath: string) {
  act(() => {
    emitEvent('document:opened', {
      tabId,
      fileName: filePath.split('/').pop(),
      filePath,
      pageCount: 1,
      rootNode: catalogNode,
      rootChildren: [],
    });
  });
}

async function flush() {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}

function validateCallsFor(tabId: string) {
  return bindings.Validate.mock.calls.filter((c) => c[0] === tabId);
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

async function renderApp() {
  const { default: App } = await import('./App');
  render(<App />);
}

beforeEach(() => {
  for (const key of Object.keys(eventHandlers)) delete eventHandlers[key];
  (globalThis as Record<string, unknown>).ResizeObserver = MockResizeObserver;
  results = {};
  bindings.Validate.mockReset().mockImplementation((tabId: string) => {
    const r = results[tabId];
    if (r instanceof Error) return Promise.reject(r);
    return Promise.resolve(r ?? emptyResult);
  });
  bindings.OpenFile.mockReset();
  bindings.GetTreeRoot.mockReset().mockResolvedValue(catalogNode);
  bindings.GetChildren.mockReset().mockResolvedValue([]);
});

afterEach(() => {
  delete (globalThis as Record<string, unknown>).ResizeObserver;
});

describe('background pdfcpu check trigger', () => {
  test('an opened tab id starts one pdfcpu check; a repeated delivery starts none', async () => {
    const t = ids();
    await renderApp();
    deliverOpen(t.a, t.pathA);
    await waitFor(() => expect(validateCallsFor(t.a)).toHaveLength(1));
    expect(validateCallsFor(t.a)[0]).toEqual([t.a, 'pdfcpu']);

    deliverOpen(t.a, t.pathA);
    await flush();
    expect(validateCallsFor(t.a)).toHaveLength(1);
  });

  test('a background tab gets its check too', async () => {
    const t = ids();
    await renderApp();
    deliverOpen(t.a, t.pathA);
    deliverOpen(t.b, t.pathB);
    await waitFor(() => {
      expect(validateCallsFor(t.a)).toHaveLength(1);
      expect(validateCallsFor(t.b)).toHaveLength(1);
    });
  });

  test('a reload starts one check for the new tab id and a released tab id starts none', async () => {
    const t = ids();
    await renderApp();
    deliverOpen(t.a, t.pathA);
    await waitFor(() => expect(validateCallsFor(t.a)).toHaveLength(1));

    bindings.OpenFile.mockResolvedValue({ tabId: t.fresh, fileName: 'a.pdf', filePath: t.pathA, pageCount: 1, fileSize: 10, error: '' });
    act(() => emitEvent('document:reload-active'));
    await waitFor(() => expect(validateCallsFor(t.fresh)).toHaveLength(1));
    expect(validateCallsFor(t.fresh)[0]).toEqual([t.fresh, 'pdfcpu']);

    // t.a was released by the reload; a late delivery of it is dropped.
    deliverOpen(t.a, t.pathA);
    await flush();
    expect(validateCallsFor(t.a)).toHaveLength(1);
  });

  test('a result that arrives after its tab closed changes nothing on screen', async () => {
    const t = ids();
    const late = deferred<unknown>();
    await renderApp();
    deliverOpen(t.a, t.pathA);
    bindings.Validate.mockImplementation((tabId: string) => (tabId === t.b ? late.promise : Promise.resolve(emptyResult)));
    deliverOpen(t.b, t.pathB);
    await waitFor(() => expect(validateCallsFor(t.b)).toHaveLength(1));

    fireEvent.click(screen.getByTestId(`tab-close-${t.b}`));
    await waitFor(() => expect(screen.queryByTestId(`tab-${t.b}`)).toBeNull());

    late.resolve(fatalResult);
    await flush();
    expect(screen.queryByTestId('pdfcpu-banner')).toBeNull();
    expect(screen.queryByTestId(`tab-${t.b}`)).toBeNull();
  });
});

describe('pdfcpu banner', () => {
  test('a fatal problem shows "pdfcpu rejects this file:" with the full text in a title', async () => {
    const t = ids();
    results[t.a] = fatalResult;
    await renderApp();
    deliverOpen(t.a, t.pathA);
    const banner = await screen.findByTestId('pdfcpu-banner');
    const full = `pdfcpu rejects this file: ${FATAL_MESSAGE}`;
    expect(banner.textContent).toContain('pdfcpu rejects this file:');
    expect(within(banner).getByTestId('error-banner-message').textContent).toBe(full);
    expect(Array.from(banner.querySelectorAll('[title]')).map((e) => e.getAttribute('title'))).toContain(full);
    // A background notice is a polite status, so tab switches do not re-announce it as an alert.
    expect(banner).toHaveAttribute('role', 'status');
  });

  test('a fatal problem with notices still shows the rejects text', async () => {
    const t = ids();
    results[t.a] = result([notice(1), problem('pdfcpu-validator', 'error', FATAL_MESSAGE, 12), notice(2)]);
    await renderApp();
    deliverOpen(t.a, t.pathA);
    const banner = await screen.findByTestId('pdfcpu-banner');
    expect(within(banner).getByTestId('error-banner-message').textContent).toBe(`pdfcpu rejects this file: ${FATAL_MESSAGE}`);
  });

  test('one notice shows the singular text', async () => {
    const t = ids();
    results[t.a] = oneNoticeResult;
    await renderApp();
    deliverOpen(t.a, t.pathA);
    const banner = await screen.findByTestId('pdfcpu-banner');
    expect(within(banner).getByTestId('error-banner-message').textContent).toBe('pdfcpu repaired or accepted 1 problem in this file');
  });

  test('two notices show the plural text', async () => {
    const t = ids();
    results[t.a] = twoNoticeResult;
    await renderApp();
    deliverOpen(t.a, t.pathA);
    const banner = await screen.findByTestId('pdfcpu-banner');
    expect(within(banner).getByTestId('error-banner-message').textContent).toBe('pdfcpu repaired or accepted 2 problems in this file');
  });

  test.each([
    ['an empty result', emptyResult],
    ['a degraded result with only info problems', degradedResult],
    ['a rejected call', new Error('file changed on disk')],
  ])('no banner for %s', async (_name, r) => {
    const t = ids();
    results[t.a] = r;
    await renderApp();
    deliverOpen(t.a, t.pathA);
    await waitFor(() => expect(validateCallsFor(t.a)).toHaveLength(1));
    await flush();
    expect(screen.queryByTestId('pdfcpu-banner')).toBeNull();
  });

  test('the banner overlays the content container and takes no layout space', async () => {
    const t = ids();
    results[t.a] = fatalResult;
    await renderApp();
    deliverOpen(t.a, t.pathA);
    const banner = await screen.findByTestId('pdfcpu-banner');

    const column = document.querySelector('[data-file-drop-target]') as HTMLElement;
    expect(column).not.toBeNull();
    const content = banner.closest('.flex-1.min-h-0') as HTMLElement | null;
    expect(content, 'banner must sit inside the content container').not.toBeNull();
    expect(content!.parentElement).toBe(column);

    const overlay = banner.closest('.absolute') as HTMLElement | null;
    expect(overlay, 'banner must be absolutely positioned').not.toBeNull();
    expect(content!.contains(overlay)).toBe(true);
    expect(Array.from(column.children)).not.toContain(banner);
    expect(Array.from(column.children)).not.toContain(overlay);
  });

  test('dismiss hides the banner for that tab only', async () => {
    const t = ids();
    results[t.a] = fatalResult;
    results[t.b] = oneNoticeResult;
    await renderApp();
    deliverOpen(t.a, t.pathA);
    deliverOpen(t.b, t.pathB);
    const banner = await screen.findByTestId('pdfcpu-banner');
    expect(banner.textContent).toContain('1 problem');

    fireEvent.click(within(banner).getByTestId('error-banner-dismiss'));
    expect(screen.queryByTestId('pdfcpu-banner')).toBeNull();

    fireEvent.mouseDown(screen.getByTestId(`tab-${t.a}`));
    const other = await screen.findByTestId('pdfcpu-banner');
    expect(other.textContent).toContain('pdfcpu rejects this file:');

    fireEvent.mouseDown(screen.getByTestId(`tab-${t.b}`));
    await flush();
    expect(screen.queryByTestId('pdfcpu-banner')).toBeNull();
  });

  test('the banner follows the active tab', async () => {
    const t = ids();
    results[t.a] = fatalResult;
    await renderApp();
    deliverOpen(t.a, t.pathA);
    deliverOpen(t.b, t.pathB);
    await waitFor(() => expect(validateCallsFor(t.b)).toHaveLength(1));
    await flush();
    expect(screen.queryByTestId('pdfcpu-banner')).toBeNull();

    fireEvent.mouseDown(screen.getByTestId(`tab-${t.a}`));
    expect(await screen.findByTestId('pdfcpu-banner')).toBeInTheDocument();
  });

  test('after a reload the new tab id shows the banner again when its result still has problems', async () => {
    const t = ids();
    results[t.a] = fatalResult;
    results[t.fresh] = fatalResult;
    await renderApp();
    deliverOpen(t.a, t.pathA);
    const banner = await screen.findByTestId('pdfcpu-banner');
    fireEvent.click(within(banner).getByTestId('error-banner-dismiss'));
    expect(screen.queryByTestId('pdfcpu-banner')).toBeNull();

    bindings.OpenFile.mockResolvedValue({ tabId: t.fresh, fileName: 'a.pdf', filePath: t.pathA, pageCount: 1, fileSize: 10, error: '' });
    act(() => emitEvent('document:reload-active'));
    await waitFor(() => expect(validateCallsFor(t.fresh)).toHaveLength(1));
    expect(await screen.findByTestId('pdfcpu-banner')).toBeInTheDocument();
  });

  test('Details opens the Validate tab on the pdfcpu profile and runs it', async () => {
    const t = ids();
    results[t.a] = fatalResult;
    await renderApp();
    deliverOpen(t.a, t.pathA);
    const banner = await screen.findByTestId('pdfcpu-banner');

    fireEvent.click(within(banner).getByRole('button', { name: 'Details' }));

    await waitFor(() => expect(screen.getByTestId('detail-tab-validate')).toHaveAttribute('data-state', 'active'));
    expect((screen.getByTestId('validate-profile') as HTMLSelectElement).value).toBe('pdfcpu');
    await waitFor(() => expect(validateCallsFor(t.a).length).toBeGreaterThanOrEqual(2));
    for (const call of validateCallsFor(t.a)) expect(call).toEqual([t.a, 'pdfcpu']);
    const rows = await screen.findAllByTestId('validate-problem');
    expect(rows.map((r) => r.textContent).join('\n')).toContain(FATAL_MESSAGE);
  });
});
