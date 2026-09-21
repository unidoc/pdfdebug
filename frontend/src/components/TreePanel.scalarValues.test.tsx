/**
 * TreePanel scalar-value rendering: the row shows what a dictionary entry says,
 * not only what type it is.
 *
 * Source contract (backend):
 *   - TreeNode.value: the decoded, escaped display form for a dictionary-entry
 *     scalar leaf; "" (or absent) for dicts, arrays, streams, refs, error nodes
 *     and array-element scalars, whose value already lives in the label.
 *
 * Render contract:
 *   - Order, left to right: label, value, rawKey, [N G R], /T:TypeName.
 *   - The value is the only element that takes the remaining width and the only
 *     one allowed to ellipsize (min-w-0 + truncate); the three suffixes keep
 *     flex-shrink-0 and are never dropped.
 *   - The full, uncapped value lives in the value span's own title attribute.
 *     No rune count in the DOM.
 *
 * Run: cd frontend && npx vitest run \
 * src/components/TreePanel.scalarValues.test.tsx
 */
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  AppProvider,
  useAppDispatch,
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

// --- Fixtures. `value` is the added field; cast through unknown so the
// fixtures compile whichever way the TreeNode TS type is widened. ---

type AnyNode = Record<string, unknown>;

const DECODED_ALT = 'Rapport cell';
const LONG_VALUE = 'L'.repeat(500);

const catalogNode: AnyNode = {
  id: 'root', label: 'Catalog', rawKey: '', nodeType: 'dict', valueType: '',
  hasChildren: true, childCount: 4, iconHint: 'catalog', error: '',
  objectRef: '', typeName: '', value: '',
};

// Dictionary-entry scalar: the row that carries a value. rawKey is rendered
// because the row carries no objectRef.
const altScalar: AnyNode = {
  id: 'dict:root:Alt', label: 'Alt', rawKey: '/Alt', nodeType: 'scalar',
  valueType: 'string', hasChildren: false, childCount: 0, iconHint: 'default',
  error: '', objectRef: '', typeName: '', value: DECODED_ALT,
};

// A number, to prove the value is not string-only.
const rowSpanScalar: AnyNode = {
  id: 'dict:root:RowSpan', label: 'RowSpan', rawKey: '/RowSpan', nodeType: 'scalar',
  valueType: 'number', hasChildren: false, childCount: 0, iconHint: 'default',
  error: '', objectRef: '', typeName: '', value: '2',
};

// A row under width pressure, carrying every suffix the row can render.
const pressuredRow: AnyNode = {
  id: 'obj:0:9', label: 'Widget', rawKey: '/Widget', nodeType: 'scalar',
  valueType: 'string', hasChildren: false, childCount: 0, iconHint: 'default',
  error: '', objectRef: '9 0 R', typeName: 'Annot', value: LONG_VALUE,
};

// A container: no value, and the row must not gain an empty segment.
const containerNode: AnyNode = {
  id: 'dict:root:A', label: 'A', rawKey: '/A', nodeType: 'dict', valueType: '',
  hasChildren: true, childCount: 3, iconHint: 'default', error: '',
  objectRef: '', typeName: '', value: '',
};

const openAction: AppAction = {
  type: 'OPEN_DOCUMENT',
  payload: {
    tabId: 'tab-1',
    fileName: 'test.pdf',
    filePath: '/test.pdf',
    rootNode: catalogNode as unknown as AppAction['payload']['rootNode'],
    rootChildren: [
      altScalar, rowSpanScalar, pressuredRow, containerNode,
    ] as unknown as AppAction['payload']['rootChildren'],
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

async function openTree() {
  const user = userEvent.setup();
  render(
    <AppProvider>
      <DispatchAndRender action={openAction}>
        <TreePanel />
      </DispatchAndRender>
    </AppProvider>,
  );
  await user.click(screen.getByTestId('dispatch'));
  return screen.findAllByTestId('tree-node');
}

/** Returns the row whose data-node-id matches, or fails the lookup. */
function rowById(rows: HTMLElement[], id: string): HTMLElement {
  const row = rows.find((r) => r.getAttribute('data-node-id')?.endsWith(id));
  if (!row) throw new Error(`no tree row for node id ${id}; got ${rows.map((r) => r.getAttribute('data-node-id')).join(', ')}`);
  return row;
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
// The value reaches the row
// ---------------------------------------------------------------------------

describe('scalar value on the tree row', () => {
  test('a dictionary-entry string scalar renders its decoded value', async () => {
    const rows = await openTree();
    expect(rowById(rows, 'dict:root:Alt').textContent).toContain(DECODED_ALT);
  });

  test('a dictionary-entry number scalar renders its value', async () => {
    const rows = await openTree();
    expect(rowById(rows, 'dict:root:RowSpan').textContent).toContain('2');
  });

  test('a container row renders no value segment', async () => {
    const rows = await openTree();
    const row = rowById(rows, 'dict:root:A');
    expect(row.textContent).toContain('A');
    // The value span is the only truncating one, so its absence is the check.
    expect(row.querySelectorAll('.truncate')).toHaveLength(0);
    expect(rowById(rows, 'dict:root:Alt').querySelectorAll('.truncate')).toHaveLength(1);
  });
});

// ---------------------------------------------------------------------------
// Order and drop priority
// ---------------------------------------------------------------------------

describe('row order and drop priority', () => {
  test('the value sits between the label and the rawKey', async () => {
    const rows = await openTree();
    const text = rowById(rows, 'dict:root:Alt').textContent ?? '';

    const label = text.indexOf('Alt');
    const value = text.indexOf(DECODED_ALT);
    const rawKey = text.indexOf('/Alt');

    expect(label).toBeGreaterThanOrEqual(0);
    expect(value).toBeGreaterThan(label);
    expect(rawKey).toBeGreaterThan(value);
  });

  test('the value precedes the [N G R] and /T: suffixes', async () => {
    const rows = await openTree();
    const text = rowById(rows, 'obj:0:9').textContent ?? '';

    const value = text.indexOf(LONG_VALUE.slice(0, 20));
    const ref = text.indexOf('[9 0 R]');
    const type = text.indexOf('/T:Annot');

    expect(value).toBeGreaterThanOrEqual(0);
    expect(ref).toBeGreaterThan(value);
    expect(type).toBeGreaterThan(ref);
  });

  test('a 500-character value never displaces the [N G R] or /T: suffix', async () => {
    const rows = await openTree();
    const text = rowById(rows, 'obj:0:9').textContent ?? '';

    expect(text).toContain('[9 0 R]');
    expect(text).toContain('/T:Annot');
  });
});

// ---------------------------------------------------------------------------
// Clamp and title
// ---------------------------------------------------------------------------

describe('single-line clamp', () => {
  test('the value span is the only one allowed to ellipsize', async () => {
    await openTree();
    const valueSpan = screen.getByTitle(LONG_VALUE);

    expect(valueSpan.className).toContain('truncate');
    expect(valueSpan.className).toContain('min-w-0');
  });

  test('the suffixes keep flex-shrink-0 so they are never clipped', async () => {
    const rows = await openTree();
    const row = rowById(rows, 'obj:0:9');

    const refSpan = Array.from(row.querySelectorAll('span'))
      .find((s) => s.textContent === '[9 0 R]');
    const typeSpan = Array.from(row.querySelectorAll('span'))
      .find((s) => s.textContent === '/T:Annot');

    expect(refSpan?.className).toContain('flex-shrink-0');
    expect(typeSpan?.className).toContain('flex-shrink-0');
  });

  test('the full uncapped value lives in the title attribute, with no rune count in the DOM', async () => {
    const rows = await openTree();
    const row = rowById(rows, 'obj:0:9');

    expect(screen.getByTitle(LONG_VALUE)).toBeTruthy();
    expect(row.textContent).not.toContain('[truncated');
  });
});
