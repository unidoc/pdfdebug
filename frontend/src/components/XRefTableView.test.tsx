/**
 * Story 9.11: XREF Table View Tests
 *
 * TDD RED PHASE: Tests MUST fail until XRefTableView.tsx is created and
 * GetXRefTable is wired through the Wails bindings.
 *
 * Covers AC#2 (table shape + columns + status pill text + row count label),
 * AC#3 (in-use / in-objstm click navigation, free rows non-clickable),
 * AC#4 (semantic HTML + tabIndex + arrow-key row focus + Enter dispatch),
 * AC#5 (status pill text is load-bearing signal),
 * AC#10 (200ms loading debounce),
 * AC#12 (in-objstm click navigates to underlying object, NOT host objstm),
 * AC#13 (error rendering with mapped message).
 *
 * Run: cd frontend && npx vitest run src/components/XRefTableView.test.tsx
 */
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
// RED PHASE: this import fails until XRefTableView.tsx exists.
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
// 9.11-UNIT-001 [P0] AC#2: renders all five always-present columns in order.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-001: column headers and order', () => {
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
// 9.11-UNIT-002 [P0] AC#2: rows sorted ascending by object number.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-002: rows sorted by object number', () => {
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
// 9.11-UNIT-003 [P0] AC#2 + AC#5: status pill text is "in-use" / "free" /
// "in-objstm" -- load-bearing signal, NOT just color.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-003: status pill text', () => {
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
// 9.11-UNIT-004 [P0] AC#2: Offset column shows decimal byte offset for in-use,
// "-" for free + in-objstm.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-004: Offset column sentinels', () => {
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
// 9.11-UNIT-005 [P0] AC#2 + AC#12: Host ObjStm column shows the host number
// for in-objstm rows, "-" for free + in-use.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-005: Host ObjStm column sentinels', () => {
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
// 9.11-UNIT-006 [P0] AC#3: clicking an in-use row dispatches onNavigate with
// the nodeID (obj:<gen>:<num>).
// ---------------------------------------------------------------------------

describe('9.11-UNIT-006: in-use row click dispatches onNavigate', () => {
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
// 9.11-UNIT-007 [P0] AC#3 + AC#12: clicking an in-objstm row dispatches
// onNavigate with the UNDERLYING object's nodeID (NOT the host objstm).
// R4 of Story 9-11 risks list pins this distinction explicitly.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-007: in-objstm row click navigates to underlying object', () => {
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
// 9.11-UNIT-008 [P0] AC#3 + AC#4: clicking a free row is a no-op (no
// navigation target).
// ---------------------------------------------------------------------------

describe('9.11-UNIT-008: free row click is no-op', () => {
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
// 9.11-UNIT-009 [P0] AC#4: Enter on a focused in-use row dispatches the same
// navigation as a click.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-009: Enter on in-use row triggers onNavigate', () => {
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
// 9.11-UNIT-010 [P0] AC#4: Enter on a focused FREE row is a no-op.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-010: Enter on free row is no-op', () => {
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
// 9.11-UNIT-011 [P0] AC#4: ArrowDown moves focus from row N to row N+1.
// ArrowUp moves focus to row N-1. Wrap-around NOT required.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-011: arrow keys move row focus', () => {
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
// 9.11-UNIT-012 [P1] AC#4: free rows are focusable (tabIndex=0) AND carry
// aria-disabled="true" so screen readers announce the disabled state.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-012: free row a11y attributes', () => {
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
// 9.11-UNIT-013 [P0] AC#10: 200ms loading debounce. Under 200ms -> no
// indicator. Over 200ms -> indicator visible.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-013: 200ms loading debounce', () => {
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
// 9.11-UNIT-014 [P0] AC#13: error rendering. A rejected fetch surfaces the
// mapped error message in data-testid="xref-error".
// ---------------------------------------------------------------------------

describe('9.11-UNIT-014: error rendering', () => {
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
// 9.11-UNIT-015 [P1]: the fetch is DEFERRED until the XREF tab is first
// activated. The payload can be very large (a 129k-entry PDF serializes ~12 MB);
// because the pane is force-mounted, an unconditional fetch would JSON.parse
// ~12 MB and render all rows on the main thread on EVERY document open, freezing
// the UI while the user is still on the Object tree. active=false must NOT fetch;
// activation triggers a single fetch. (Supersedes the pre-perf "eager fetch on
// mount regardless of active" behavior.)
// ---------------------------------------------------------------------------

describe('9.11-UNIT-015: fetch deferred until activation', () => {
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

  test('switching documents while the tab stays active re-fetches for the new document', async () => {
    // Distinct data per document so the assertion proves the new document's rows
    // actually RENDER -- not merely that GetXRefTable was called (a call that is
    // started then cancelled would still register, so it would not guard the fix).
    mockGetXRefTable.mockImplementation((id: string) =>
      Promise.resolve(id === 'tab-2' ? xrefSingleInUse : xrefBasic),
    );
    const { rerender } = render(
      <XRefTableView tabId="tab-1" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />,
    );
    await screen.findByTestId('xref-row-objnum-1'); // tab-1 rendered (objNum 1)
    // New document, XREF tab still active (active stays true across the switch).
    rerender(<XRefTableView tabId="tab-2" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    // tab-2's distinct row (objNum 7) must appear: proves the new document fetched
    // AND rendered. Without tabId in the latch deps, everActive stays false after
    // the reset and this never resolves.
    expect(await screen.findByTestId('xref-row-objnum-7')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// 9.11-UNIT-017 [P1] Perf: the row list is viewport-virtualized -- a large xref
// (a 750-page PDF can carry ~129k entries) must render only a bounded window of
// rows, not one <tr> per entry, or the main thread freezes on render.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-017: row list is virtualized', () => {
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
// 9.11-UNIT-016 [P1] AC#2 + Task 6.10: onLoaded fires with the entry count
// after a successful fetch.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-016: onLoaded callback', () => {
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
// 9.11-UNIT-017 [P0] Task 6.8: empty state when no tabId / no document open.
// ---------------------------------------------------------------------------

describe('9.11-UNIT-017: empty state when no document', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('xref-empty visible when tabId is empty', () => {
    render(<XRefTableView tabId="" active={true} onNavigate={vi.fn()} onLoaded={vi.fn()} />);
    expect(screen.getByTestId('xref-empty')).toBeInTheDocument();
    expect(mockGetXRefTable).not.toHaveBeenCalled();
  });
});
