/**
 * XREF Table View Tests
 *
 * Covers:
 *   - table shape, columns, status pill text and the row count label;
 *   - in-use / in-objstm click navigation, free rows non-clickable;
 *   - semantic HTML, tabIndex, arrow-key row focus and Enter dispatch;
 *   - status pill text as a load-bearing signal;
 *   - the 200ms loading debounce;
 *   - in-objstm click navigating to the underlying object, NOT the host
 *     objstm;
 *   - error rendering with a mapped message.
 *
 * Run: cd frontend && npx vitest run src/components/XRefTableView.test.tsx
 */
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import { XRefTableView } from './XRefTableView';

// --- Mocks ---

const mockGetXRefTable = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    GetXRefTable: (...args: unknown[]) => mockGetXRefTable(...args),
  })
);

// --- Fixtures ---

type XRefEntryFixture = {
  objNum: number;
  gen: number;
  status: 'in-use' | 'free' | 'in-objstm';
  offset: number;
  hostObjStm: number;
  nodeID: string;
};

type XRefTableFixture = {
  tabId: string;
  entries: XRefEntryFixture[];
};

const xrefBasic: XRefTableFixture = {
  tabId: 'tab-1',
  entries: [
    { objNum: 1, gen: 0, status: 'in-use', offset: 15, hostObjStm: 0, nodeID: 'obj:0:1' },
    { objNum: 2, gen: 0, status: 'in-use', offset: 120, hostObjStm: 0, nodeID: 'obj:0:2' },
    { objNum: 3, gen: 0, status: 'free', offset: -1, hostObjStm: 0, nodeID: '' },
    { objNum: 4, gen: 0, status: 'in-objstm', offset: -1, hostObjStm: 9, nodeID: 'obj:0:4' },
    { objNum: 5, gen: 0, status: 'in-use', offset: 480, hostObjStm: 0, nodeID: 'obj:0:5' },
  ],
};

const xrefSingleInUse: XRefTableFixture = {
  tabId: 'tab-1',
  entries: [
    { objNum: 7, gen: 0, status: 'in-use', offset: 256, hostObjStm: 0, nodeID: 'obj:0:7' },
  ],
};

// ---------------------------------------------------------------------------
// Renders all five always-present columns in order.
// ---------------------------------------------------------------------------

describe('column headers and order', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefBasic);
  });

  test('renders Obj #, Gen, Offset, Status, Host ObjStm in order', async () => {
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByText('Obj #')).toBeInTheDocument();
    });
    const headers = screen.getAllByRole('columnheader').map((th) => th.textContent?.trim() ?? '');
    expect(headers).toEqual(['Obj #', 'Gen', 'Offset', 'Status', 'Host ObjStm']);
  });
});

// ---------------------------------------------------------------------------
// Rows sorted ascending by object number.
// ---------------------------------------------------------------------------

describe('rows sorted by object number', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefBasic);
  });

  test('row order matches payload order (backend already sorted)', async () => {
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByText('Obj #')).toBeInTheDocument();
    });
    // Render objNum column cells -- the first column body should display 1,2,3,4,5 in order.
    const objNumCells = screen.getAllByTestId(/^xref-row-objnum-/);
    const rendered = objNumCells.map((c) => c.textContent?.trim());
    expect(rendered).toEqual(['1', '2', '3', '4', '5']);
  });
});

// ---------------------------------------------------------------------------
// Status pill text is "in-use" / "free" / "in-objstm" -- load-bearing
// signal, NOT just color.
// ---------------------------------------------------------------------------

describe('status pill text', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefBasic);
  });

  test('each row renders the literal status string', async () => {
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getAllByText('in-use').length).toBe(3);
    });
    expect(screen.getByText('free')).toBeInTheDocument();
    expect(screen.getByText('in-objstm')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Offset column shows decimal byte offset for in-use, "-" for free +
// in-objstm.
// ---------------------------------------------------------------------------

describe('Offset column sentinels', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefBasic);
  });

  test('in-use rows show the numeric offset; free + in-objstm rows show "-"', async () => {
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByText('15')).toBeInTheDocument();
    });
    expect(screen.getByText('120')).toBeInTheDocument();
    expect(screen.getByText('480')).toBeInTheDocument();
    // Free row (objNum 3) and in-objstm row (objNum 4) both render "-" in Offset.
    const offsetCells = screen.getAllByTestId(/^xref-row-offset-/);
    expect(offsetCells.find((c) => c.dataset.testid === 'xref-row-offset-3')?.textContent?.trim()).toBe('-');
    expect(offsetCells.find((c) => c.dataset.testid === 'xref-row-offset-4')?.textContent?.trim()).toBe('-');
  });
});

// ---------------------------------------------------------------------------
// Host ObjStm column shows the host number for in-objstm rows, "-" for free
// + in-use.
// ---------------------------------------------------------------------------

describe('Host ObjStm column sentinels', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefBasic);
  });

  test('in-objstm row shows host objstm number; other rows show "-"', async () => {
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByText('Host ObjStm')).toBeInTheDocument();
    });
    const hostCells = screen.getAllByTestId(/^xref-row-host-/);
    expect(hostCells.find((c) => c.dataset.testid === 'xref-row-host-4')?.textContent?.trim()).toBe('9');
    expect(hostCells.find((c) => c.dataset.testid === 'xref-row-host-1')?.textContent?.trim()).toBe('-');
    expect(hostCells.find((c) => c.dataset.testid === 'xref-row-host-3')?.textContent?.trim()).toBe('-');
  });
});

// ---------------------------------------------------------------------------
// Clicking an in-use row dispatches onNavigate with the nodeID
// (obj:<gen>:<num>).
// ---------------------------------------------------------------------------

describe('in-use row click dispatches onNavigate', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefSingleInUse);
  });

  test('click on row calls onNavigate("obj:0:7")', async () => {
    const onNavigate = vi.fn();
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={onNavigate} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByText('7')).toBeInTheDocument();
    });
    const row = screen.getByTestId('xref-row-7');
    fireEvent.click(row);
    expect(onNavigate).toHaveBeenCalledWith('obj:0:7');
  });
});

// ---------------------------------------------------------------------------
// Clicking an in-objstm row dispatches onNavigate with the UNDERLYING
// object's nodeID (NOT the host objstm). The risks list
// pins this distinction explicitly.
// ---------------------------------------------------------------------------

describe('in-objstm row click navigates to underlying object', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefBasic);
  });

  test('click on in-objstm row (objNum 4, host 9) navigates to obj:0:4 NOT obj:0:9', async () => {
    const onNavigate = vi.fn();
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={onNavigate} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByText('in-objstm')).toBeInTheDocument();
    });
    const row = screen.getByTestId('xref-row-4');
    fireEvent.click(row);
    expect(onNavigate).toHaveBeenCalledWith('obj:0:4');
    expect(onNavigate).not.toHaveBeenCalledWith('obj:0:9');
  });
});

// ---------------------------------------------------------------------------
// Clicking a free row is a no-op (no navigation target).
// ---------------------------------------------------------------------------

describe('free row click is no-op', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefBasic);
  });

  test('click on free row does NOT call onNavigate', async () => {
    const onNavigate = vi.fn();
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={onNavigate} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByText('free')).toBeInTheDocument();
    });
    const row = screen.getByTestId('xref-row-3');
    fireEvent.click(row);
    expect(onNavigate).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Enter on a focused in-use row dispatches the same navigation as a click.
// ---------------------------------------------------------------------------

describe('Enter on in-use row triggers onNavigate', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefSingleInUse);
  });

  test('Enter keypress on focused in-use row calls onNavigate', async () => {
    const onNavigate = vi.fn();
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={onNavigate} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByText('7')).toBeInTheDocument();
    });
    const row = screen.getByTestId('xref-row-7');
    row.focus();
    fireEvent.keyDown(row, { key: 'Enter' });
    expect(onNavigate).toHaveBeenCalledWith('obj:0:7');
  });
});

// ---------------------------------------------------------------------------
// Enter on a focused FREE row is a no-op.
// ---------------------------------------------------------------------------

describe('Enter on free row is no-op', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefBasic);
  });

  test('Enter keypress on focused free row does NOT call onNavigate', async () => {
    const onNavigate = vi.fn();
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={onNavigate} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByText('free')).toBeInTheDocument();
    });
    const row = screen.getByTestId('xref-row-3');
    row.focus();
    fireEvent.keyDown(row, { key: 'Enter' });
    expect(onNavigate).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// ArrowDown moves focus from row N to row N+1. ArrowUp moves focus to
// row N-1. Wrap-around NOT required.
// ---------------------------------------------------------------------------

describe('arrow keys move row focus', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefBasic);
  });

  test('ArrowDown moves focus to next row', async () => {
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByTestId('xref-row-1')).toBeInTheDocument();
    });
    const firstRow = screen.getByTestId('xref-row-1');
    firstRow.focus();
    expect(document.activeElement).toBe(firstRow);
    fireEvent.keyDown(firstRow, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(screen.getByTestId('xref-row-2'));
  });

  test('ArrowUp moves focus to previous row', async () => {
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByTestId('xref-row-2')).toBeInTheDocument();
    });
    const secondRow = screen.getByTestId('xref-row-2');
    secondRow.focus();
    fireEvent.keyDown(secondRow, { key: 'ArrowUp' });
    expect(document.activeElement).toBe(screen.getByTestId('xref-row-1'));
  });
});

// ---------------------------------------------------------------------------
// Free rows are focusable (tabIndex=0) AND carry aria-disabled="true" so
// screen readers announce the disabled state.
// ---------------------------------------------------------------------------

describe('free row a11y attributes', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefBasic);
  });

  test('free row has tabIndex=0 and aria-disabled="true"', async () => {
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByText('free')).toBeInTheDocument();
    });
    const freeRow = screen.getByTestId('xref-row-3');
    expect(freeRow.getAttribute('tabindex')).toBe('0');
    expect(freeRow.getAttribute('aria-disabled')).toBe('true');
  });
});

// ---------------------------------------------------------------------------
// 200ms loading debounce. Under 200ms -> no indicator. Over 200ms ->
// indicator visible.
// ---------------------------------------------------------------------------

describe('200ms loading debounce', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  test('loading indicator does NOT show before 200ms elapses', async () => {
    let resolveFn: ((val: XRefTableFixture) => void) | null = null;
    mockGetXRefTable.mockReturnValueOnce(
      new Promise<XRefTableFixture>((resolve) => {
        resolveFn = resolve;
      })
    );
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    // Advance just under the debounce window.
    act(() => {
      vi.advanceTimersByTime(150);
    });
    expect(screen.queryByTestId('xref-loading')).not.toBeInTheDocument();
    // Cleanup: resolve so cleanup effect can clear.
    act(() => {
      resolveFn!(xrefBasic);
    });
  });

  test('loading indicator appears after 200ms elapses while in flight', async () => {
    let resolveFn: ((val: XRefTableFixture) => void) | null = null;
    mockGetXRefTable.mockReturnValueOnce(
      new Promise<XRefTableFixture>((resolve) => {
        resolveFn = resolve;
      })
    );
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    act(() => {
      vi.advanceTimersByTime(250);
    });
    expect(screen.getByTestId('xref-loading')).toBeInTheDocument();
    // Cleanup.
    act(() => {
      resolveFn!(xrefBasic);
    });
  });
});

// ---------------------------------------------------------------------------
// Error rendering. A rejected fetch surfaces the mapped error message in
// data-testid="xref-error".
// ---------------------------------------------------------------------------

describe('error rendering', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('rejected fetch renders xref-error with mapped message', async () => {
    mockGetXRefTable.mockRejectedValueOnce(new Error('xref build panicked'));
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByTestId('xref-error')).toBeInTheDocument();
    });
    expect(screen.getByTestId('xref-error').textContent).toContain('xref build panicked');
  });
});

// ---------------------------------------------------------------------------
// The fetch is DEFERRED until the XREF tab is first
// activated. The payload can be very large (a 129k-entry PDF serializes ~12 MB);
// because the pane is force-mounted, an unconditional fetch would JSON.parse
// ~12 MB and render all rows on the main thread on EVERY document open, freezing
// the UI while the user is still on the Object tree. active=false must NOT fetch;
// activation triggers a single fetch. (Supersedes the pre-perf "eager fetch on
// mount regardless of active" behavior.)
// ---------------------------------------------------------------------------

describe('fetch deferred until activation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefBasic);
  });

  test('active=false does not fetch', () => {
    render(<XRefTableView tabId="tab-1" active={false} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    // render() flushes mount effects under act; with active=false the fetch
    // effect exits synchronously, so no fetch is scheduled -- assert immediately.
    expect(mockGetXRefTable).not.toHaveBeenCalled();
  });

  test('activation after an inactive mount triggers a single fetch', async () => {
    const { rerender } = render(
      <XRefTableView tabId="tab-1" active={false} onNavigate={vi.fn()} onLoaded={vi.fn()} />,
    );
    expect(mockGetXRefTable).not.toHaveBeenCalled();
    rerender(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(mockGetXRefTable).toHaveBeenCalledWith('tab-1');
    });
    expect(mockGetXRefTable).toHaveBeenCalledTimes(1);
  });

  test('active=true triggers a single fetch', async () => {
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    await waitFor(() => {
      expect(mockGetXRefTable).toHaveBeenCalledWith('tab-1');
    });
    expect(mockGetXRefTable).toHaveBeenCalledTimes(1);
  });

  test('switching documents while XREF is inactive does NOT eagerly fetch; re-activating fetches the new doc', async () => {
    // Distinct data per document so re-activation can be asserted by rendered rows.
    mockGetXRefTable.mockImplementation((id: string) =>
      Promise.resolve(id === 'tab-2' ? xrefSingleInUse : xrefBasic),
    );
    const { rerender } = render(
      <XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />,
    );
    await screen.findByTestId('xref-row-objnum-1'); // tab-1 fetched + rendered
    expect(mockGetXRefTable).toHaveBeenCalledTimes(1);

    // Switch documents with the XREF tab INACTIVE (parent lands on Object). A
    // stale activation must NOT eagerly fetch the new, unopened document.
    rerender(<XRefTableView tabId="tab-2" active={false} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    await new Promise((r) => setTimeout(r, 20));
    expect(mockGetXRefTable).not.toHaveBeenCalledWith('tab-2');
    expect(mockGetXRefTable).toHaveBeenCalledTimes(1);

    // Re-activating XREF on the new document fetches and renders it.
    rerender(<XRefTableView tabId="tab-2" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    expect(await screen.findByTestId('xref-row-objnum-7')).toBeInTheDocument();
    expect(mockGetXRefTable).toHaveBeenCalledWith('tab-2');
  });

  test('returning to a previously-opened document on the Object tab does NOT eagerly re-fetch', async () => {
    mockGetXRefTable.mockResolvedValue(xrefBasic);
    // Doc A: open XREF (latches A).
    const { rerender } = render(
      <XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />,
    );
    await screen.findByTestId('xref-row-objnum-1');
    expect(mockGetXRefTable).toHaveBeenCalledTimes(1);
    // Switch to doc B on the Object tab (inactive).
    rerender(<XRefTableView tabId="tab-2" active={false} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    // Switch BACK to doc A, still on the Object tab (inactive). The latch must
    // have been cleared on the first switch, so returning to A must NOT re-fetch.
    rerender(<XRefTableView tabId="tab-1" active={false} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    await new Promise((r) => setTimeout(r, 20));
    expect(mockGetXRefTable).toHaveBeenCalledTimes(1); // still just the original doc-A fetch
  });
});

// ---------------------------------------------------------------------------
// Perf: the row list is viewport-virtualized -- a large xref (a 750-page PDF
// can carry ~129k entries) must render only a bounded window of rows, not one
// <tr> per entry, or the main thread freezes on render.
// ---------------------------------------------------------------------------

describe('row list is virtualized', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('a large table renders only a bounded window of rows', async () => {
    const big: XRefTableFixture = {
      tabId: 'tab-1',
      entries: Array.from({ length: 3000 }, (_, i) => ({
        objNum: i + 1,
        gen: 0,
        status: 'in-use' as const,
        offset: (i + 1) * 16,
        hostObjStm: 0,
        nodeID: `obj:0:${i + 1}`,
      })),
    };
    mockGetXRefTable.mockResolvedValue(big);
    const { container } = render(
      <XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />,
    );
    await waitFor(() => {
      expect(container.querySelectorAll('[data-testid^="xref-row-objnum-"]').length).toBeGreaterThan(0);
    });
    // Only a small window commits to the DOM (jsdom viewport fallback ~= 36
    // rows), never all 3000. A spacer <tr> reserves the off-window scroll height.
    const rendered = container.querySelectorAll('[data-testid^="xref-row-objnum-"]').length;
    expect(rendered).toBeGreaterThan(0);
    expect(rendered).toBeLessThan(200);
    expect(container.querySelectorAll('tr[aria-hidden="true"]').length).toBeGreaterThan(0);
  });
});

// ---------------------------------------------------------------------------
// Virtualization: ArrowDown past the rendered window scrolls the next row
// into view and focuses it. DOM-sibling focus cannot work here because
// off-window rows are unmounted; the handler walks the index.
// ---------------------------------------------------------------------------

describe('keyboard nav crosses the virtualization window', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    const big: XRefTableFixture = {
      tabId: 'tab-1',
      entries: Array.from({ length: 3000 }, (_, i) => ({
        objNum: i + 1,
        gen: 0,
        status: 'in-use' as const,
        offset: (i + 1) * 16,
        hostObjStm: 0,
        nodeID: `obj:0:${i + 1}`,
      })),
    };
    mockGetXRefTable.mockResolvedValue(big);
  });

  test('ArrowDown from the last in-window row focuses the next (initially unrendered) row', async () => {
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    // Row 36 is at the edge of the initial ~36-row window; row 37 is not rendered.
    const row36 = await screen.findByTestId('xref-row-36');
    expect(screen.queryByTestId('xref-row-37')).toBeNull();
    row36.focus();
    fireEvent.keyDown(row36, { key: 'ArrowDown' });
    // Row 37 must scroll in AND receive focus.
    const row37 = await screen.findByTestId('xref-row-37');
    await waitFor(() => expect(document.activeElement).toBe(row37));
  });

  test('ArrowUp at the top row does not wrap', async () => {
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    const row1 = await screen.findByTestId('xref-row-1');
    row1.focus();
    fireEvent.keyDown(row1, { key: 'ArrowUp' });
    // No wrap: focus stays on row 1.
    expect(document.activeElement).toBe(row1);
  });
});

// ---------------------------------------------------------------------------
// onLoaded fires with the entry count after a successful fetch.
// ---------------------------------------------------------------------------

describe('onLoaded callback', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetXRefTable.mockResolvedValue(xrefBasic);
  });

  test('onLoaded receives the entry count', async () => {
    const onLoaded = vi.fn();
    render(<XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={onLoaded} />);
    await waitFor(() => {
      expect(onLoaded).toHaveBeenCalledWith(5);
    });
  });
});

// ---------------------------------------------------------------------------
// Empty state when no tabId / no document open.
// ---------------------------------------------------------------------------

describe('empty state when no document', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('xref-empty visible when tabId is empty', () => {
    render(<XRefTableView tabId="" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    expect(screen.getByTestId('xref-empty')).toBeInTheDocument();
    expect(mockGetXRefTable).not.toHaveBeenCalled();
  });
});
