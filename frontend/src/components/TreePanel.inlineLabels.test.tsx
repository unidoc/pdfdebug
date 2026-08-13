/**
 * Story 9-8: TreePanel inline object-ref + /T:Type suffix rendering.
 *
 * Source contract (backend, see internal/pdfcore/objectindex_test.go):
 *   - TreeNode.objectRef: "<num> <gen> R" for indirect objects, "" otherwise
 *   - TreeNode.typeName:  literal /Type value (e.g. "Pages", "Page", "Font"),
 *                         "" when the dict has no /Type key
 *
 * Render contract (dedup rule):
 *   - Append `[objectRef]` after the semantic label when objectRef !== ""
 *   - Append `/T:typeName` after the ref when typeName !== "" AND
 *     typeName is NOT already encoded in the semantic label. The label
 *     "Pages" already encodes /Type /Pages, so /T:Pages is suppressed.
 *     Font nodes use semantic label "Font: <BaseFont>" which prefixes
 *     "Font", so /T:Font is also suppressed.
 *
 * clicking a tree row still dispatches SELECT_NODE (existing behavior);
 *      the inline label is read-only display, never NAVIGATE_TO_REF.
 *
 * Run: cd frontend && npx vitest run \
 *      src/components/TreePanel.inlineLabels.test.tsx
 */
import { render, screen, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  AppProvider,
  useAppDispatch,
  useAppState,
  type AppAction,
} from '../hooks/useDocumentState';
import { TreePanel } from './TreePanel';

// --- Mocks (mirrors TreePanel.test.tsx setup) ---

vi.mock('allotment', () => {
  function Pane({ children }: { children: React.ReactNode }) { return <div>{children}</div>; }
  function Allotment({ children }: { children: React.ReactNode }) { return <div>{children}</div>; }
  Allotment.Pane = Pane;
  return { Allotment };
});
vi.mock('allotment/dist/style.css', () => ({}));

const mockGetChildren = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: (...args: unknown[]) => mockGetChildren(...args),
    CloseDocument: vi.fn(),
    OpenFileDialog: vi.fn(),
    GetObjectDetail: vi.fn(),
    GetAncestorPath: vi.fn(),
  }),
);

class MockResizeObserver {
  callback: ResizeObserverCallback;
  constructor(callback: ResizeObserverCallback) { this.callback = callback; }
  observe(target: Element) {
    this.callback(
      [{
        target,
        contentRect: { width: 300, height: 600 } as DOMRectReadOnly,
        borderBoxSize: [], contentBoxSize: [], devicePixelContentBoxSize: [],
      } as ResizeObserverEntry],
      this,
    );
  }
  unobserve() {}
  disconnect() {}
}

// --- Fixtures: TreeNodes the backend will emit once 9-8 lands. The
// `objectRef` / `typeName` fields are the new additions. We assume the
// existing TreeNode TS type will be extended; cast through unknown so the

type AnyNode = Record<string, unknown>;

const catalogNode: AnyNode = {
  id: 'root', label: 'Catalog', rawKey: '', nodeType: 'dict', valueType: '',
  hasChildren: true, childCount: 3, iconHint: 'catalog', error: '',
  objectRef: '', typeName: '',
};

// Indirect object with semantic label "Pages" already encoding /Type /Pages
// -- /T:Pages must be suppressed dedup rule.
const pagesIndirect: AnyNode = {
  id: 'obj:0:2', label: 'Pages', rawKey: '/Pages', nodeType: 'dict',
  valueType: 'reference', hasChildren: true, childCount: 2, iconHint: 'pages',
  error: '', objectRef: '2 0 R', typeName: 'Pages',
};

// Indirect object with /Type /Font and BaseFont -- semantic label is
// "Font: Helvetica"; /T:Font is suppressed dedup rule.
const fontIndirect: AnyNode = {
  id: 'obj:0:5', label: 'Font: Helvetica', rawKey: '/F1', nodeType: 'dict',
  valueType: 'reference', hasChildren: true, childCount: 5, iconHint: 'font',
  error: '', objectRef: '5 0 R', typeName: 'Font',
};

// Indirect object with /Type set to something the semantic label does NOT
// already encode -- e.g. /Type /Metadata appearing under a non-keyword
// dict key. /T:Metadata MUST be rendered.
const metadataIndirect: AnyNode = {
  id: 'obj:0:7', label: 'F2', rawKey: '/F2', nodeType: 'dict',
  valueType: 'reference', hasChildren: true, childCount: 1, iconHint: 'default',
  error: '', objectRef: '7 0 R', typeName: 'Metadata',
};

// Inline scalar (/Type entry on the catalog) -- no objectRef, no typeName.
// Inline labels must NOT render either suffix.
const inlineTypeScalar: AnyNode = {
  id: 'dict:root:Type', label: 'Catalog', rawKey: '/Type', nodeType: 'scalar',
  valueType: 'name', hasChildren: false, childCount: 0, iconHint: 'default',
  error: '', objectRef: '', typeName: '',
};

const openAction: AppAction = {
  type: 'OPEN_DOCUMENT',
  payload: {
    tabId: 'tab-1',
    fileName: 'test.pdf',
    filePath: '/test.pdf',
    rootNode: catalogNode as unknown as AppAction['payload']['rootNode'],
    rootChildren: [pagesIndirect, fontIndirect, metadataIndirect, inlineTypeScalar] as unknown as AppAction['payload']['rootChildren'],
  },
};

function DispatchAndRender({ action, children }: { action: AppAction; children: React.ReactNode }) {
  const dispatch = useAppDispatch();
  return (
    <div>
      <button data-testid="dispatch" onClick={() => dispatch(action)} />
      {children}
    </div>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  (globalThis as Record<string, unknown>).ResizeObserver = MockResizeObserver;
});

afterEach(() => {
  vi.useRealTimers();
  delete (globalThis as Record<string, unknown>).ResizeObserver;
});

// ---------------------------------------------------------------------------
// Inline [N G R] on indirect objects
// ---------------------------------------------------------------------------

describe('inline object ref suffix', () => {
  test('indirect object node renders [N G R] suffix after the semantic label', async () => {
    const user = userEvent.setup();
    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>,
    );
    await user.click(screen.getByTestId('dispatch'));

    // The pages row should be visible with the ref suffix.
    const pagesRow = await screen.findByText(/Pages/);
    expect(pagesRow.closest('[data-testid="tree-node"]')!.textContent).toMatch(/\[2 0 R\]/);
  });

  test('every indirect-object child row contains its [N G R] suffix', async () => {
    const user = userEvent.setup();
    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>,
    );
    await user.click(screen.getByTestId('dispatch'));

    // Collect text of every tree-node row, then assert each indirect-object
    // row contains its ref in the bracketed form.
    const rows = await screen.findAllByTestId('tree-node');
    const textAll = rows.map((r) => r.textContent ?? '').join('|');
    expect(textAll).toMatch(/\[2 0 R\]/);
    expect(textAll).toMatch(/\[5 0 R\]/);
    expect(textAll).toMatch(/\[7 0 R\]/);
  });
});

// ---------------------------------------------------------------------------
// /T:<TypeName> suffix with dedup rule
// ---------------------------------------------------------------------------

describe('/T:<TypeName> suffix and dedup', () => {
  test('/T:Pages is SUPPRESSED when semantic label already says "Pages"', async () => {
    const user = userEvent.setup();
    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>,
    );
    await user.click(screen.getByTestId('dispatch'));

    const rows = await screen.findAllByTestId('tree-node');
    const pagesRow = rows.find((el) => el.textContent?.includes('[2 0 R]'));
    expect(pagesRow).toBeDefined();
    expect(pagesRow!.textContent).not.toMatch(/\/T:Pages/i);
  });

  test('/T:Font is SUPPRESSED when semantic label starts with "Font:"', async () => {
    const user = userEvent.setup();
    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>,
    );
    await user.click(screen.getByTestId('dispatch'));

    const rows = await screen.findAllByTestId('tree-node');
    const fontRow = rows.find((el) => el.textContent?.includes('[5 0 R]'));
    expect(fontRow).toBeDefined();
    expect(fontRow!.textContent).not.toMatch(/\/T:Font/i);
  });

  test('/T:<TypeName> IS rendered when semantic label does not encode it', async () => {
    const user = userEvent.setup();
    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>,
    );
    await user.click(screen.getByTestId('dispatch'));

    const rows = await screen.findAllByTestId('tree-node');
    const metaRow = rows.find((el) => el.textContent?.includes('[7 0 R]'));
    expect(metaRow).toBeDefined();
    expect(metaRow!.textContent).toMatch(/\/T:Metadata/);
  });
});

// ---------------------------------------------------------------------------
// Inline label is read-only; click still selects, never NAVIGATE_TO_REF
// ---------------------------------------------------------------------------

describe('inline label is read-only display', () => {
  test('clicking the row whose label contains [2 0 R] does NOT set pendingNavTarget', async () => {
    function NavProbe() {
      const state = useAppState();
      const tab = state.tabs.find((t) => t.tabId === state.activeTabId) ?? null;
      const selected = tab?.selectedNodeId ?? 'null';
      const pending = tab?.pendingNavTarget ?? 'null';
      return (
        <div>
          <span data-testid="selected-node-id">{selected}</span>
          <span data-testid="pending-nav-target">{pending}</span>
        </div>
      );
    }
    const user = userEvent.setup();
    render(
      <AppProvider>
        <DispatchAndRender action={openAction}>
          <NavProbe />
          <TreePanel />
        </DispatchAndRender>
      </AppProvider>,
    );
    act(() => { screen.getByTestId('dispatch').click(); });

    const rows = await screen.findAllByTestId('tree-node');
    // Locate the Pages indirect row. The [2 0 R] suffix identifies it
    // uniquely.
    const pagesRow = rows.find((el) => el.textContent?.includes('[2 0 R]'));
    expect(pagesRow).toBeDefined();
    await user.click(pagesRow!);

    // pendingNavTarget must stay null -- only the existing reference-click
    // path in DetailPanel/ContentStreamViewer (NOT tree row clicks) sets it.
    expect(screen.getByTestId('pending-nav-target').textContent).toBe('null');
  });
});
