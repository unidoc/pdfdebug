/**
 * Object Source View + Reverse References
 *
 * Covers the frontend failure-mode banner for the "Referenced by" section.
 * Keyboard parity for the row click target is also asserted here, mirroring
 * DetailShared's Enter/Space contract.
 *
 * Behavioral focus: the section MUST render three distinct empty states based
 * on props (index-unavailable / catalog / orphan) and MUST default-expand or
 * default-collapse based on entries.length <= 5. Toggle resets on remount
 * because DetailPanel passes key={selectedNodeId}.
 *
 * Run: cd frontend && npx vitest run src/components/ReverseRefsSection.test.tsx
 */
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import {
  AppProvider,
  useAppDispatch,
  useAppState,
  type AppAction,
} from '../hooks/useDocumentState';
import { ReverseRefsSection } from './ReverseRefsSection';

// Mock allotment so jsdom (no layout API) does not blow up.
vi.mock('allotment', () => {
  function Pane({ children }: { children: React.ReactNode }) {
    return <div>{children}</div>;
  }
  function Allotment({ children }: { children: React.ReactNode }) {
    return <div>{children}</div>;
  }
  Allotment.Pane = Pane;
  return { Allotment };
});
vi.mock('allotment/dist/style.css', () => ({}));

// --- Fixtures ---

/** Five entries -- default expanded. */
const fiveEntries = [
  { parentNodeId: 'obj:0:10', parentRef: '10 0 R', parentType: 'Pages', path: '/Kids[0]', parentPath: '/Pages' },
  { parentNodeId: 'obj:0:11', parentRef: '11 0 R', parentType: 'Pages', path: '/Kids[1]', parentPath: '/Pages' },
  { parentNodeId: 'obj:0:12', parentRef: '12 0 R', parentType: 'Pages', path: '/Kids[2]', parentPath: '/Pages' },
  { parentNodeId: 'obj:0:13', parentRef: '13 0 R', parentType: 'Pages', path: '/Kids[3]', parentPath: '/Pages' },
  { parentNodeId: 'obj:0:14', parentRef: '14 0 R', parentType: 'Pages', path: '/Kids[4]', parentPath: '/Pages' },
];

/** Six entries -- default collapsed. */
const sixEntries = [
  ...fiveEntries,
  { parentNodeId: 'obj:0:15', parentRef: '15 0 R', parentType: 'Pages', path: '/Kids[5]', parentPath: '/Pages' },
];

/** Entry with no /Type key -- ParentType nil; the column must be omitted. */
const entryNoType = [
  { parentNodeId: 'obj:0:20', parentRef: '20 0 R', parentType: null, path: '/Resources /Font /F1', parentPath: '/Pages /Kids[0]' },
];

// --- Helpers ---

function DispatchHelper({ action }: { action: AppAction }) {
  const dispatch = useAppDispatch();
  dispatch(action);
  return null;
}

function StateReader() {
  const state = useAppState();
  const activeTab = state.tabs.find((t) => t.tabId === state.activeTabId);
  return (
    <span data-testid="pending-nav-target">
      {String(activeTab?.pendingNavTarget ?? '')}
    </span>
  );
}

const openAction: AppAction = {
  type: 'OPEN_DOCUMENT',
  payload: {
    tabId: 'tab-1',
    fileName: 'test.pdf',
    filePath: '/path/to/test.pdf',
    rootNode: {
      id: 'root',
      label: 'Catalog',
      rawKey: '',
      nodeType: 'dict',
      valueType: '',
      hasChildren: true,
      childCount: 3,
      iconHint: 'catalog',
      error: '',
    },
    rootChildren: [],
  },
};

interface RenderOpts {
  entries?: Array<{ parentNodeId: string; parentRef: string; parentType: string | null; path: string; parentPath: string }>;
  selectedIconHint?: string | null;
  indexUnavailable?: boolean;
}

function renderSection(opts: RenderOpts = {}) {
  const { entries = [], selectedIconHint = null, indexUnavailable = false } = opts;
  return render(
    <AppProvider>
      <DispatchHelper action={openAction} />
      <ReverseRefsSection
        entries={entries}
        selectedIconHint={selectedIconHint}
        indexUnavailable={indexUnavailable}
      />
      <StateReader />
    </AppProvider>
  );
}

// ---------------------------------------------------------------------------
// default-expanded when entries.length <= 5
// ---------------------------------------------------------------------------

describe('default expansion by entry count', () => {
  test('default expanded for 5 entries -- all rows visible', () => {
    renderSection({ entries: fiveEntries });
    expect(screen.getByText('10 0 R')).toBeInTheDocument();
    expect(screen.getByText('14 0 R')).toBeInTheDocument();
    // Count is shown in header
    expect(screen.getByText(/Referenced by \(5\)/)).toBeInTheDocument();
  });

  test('default collapsed for 6 entries -- rows hidden, count in header', () => {
    renderSection({ entries: sixEntries });
    expect(screen.getByText(/Referenced by \(6\)/)).toBeInTheDocument();
    expect(screen.queryByText('10 0 R')).not.toBeInTheDocument();
    expect(screen.queryByText('15 0 R')).not.toBeInTheDocument();
  });

  test('toggle expands a collapsed section', async () => {
    const user = userEvent.setup();
    renderSection({ entries: sixEntries });
    const toggle = screen.getByRole('button', { name: /Referenced by/i });
    await user.click(toggle);
    expect(screen.getByText('10 0 R')).toBeInTheDocument();
    expect(screen.getByText('15 0 R')).toBeInTheDocument();
  });

  test('toggle collapses an expanded section', async () => {
    const user = userEvent.setup();
    renderSection({ entries: fiveEntries });
    const toggle = screen.getByRole('button', { name: /Referenced by/i });
    await user.click(toggle);
    expect(screen.queryByText('10 0 R')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Row content (ParentRef, Path, ParentType)
// ---------------------------------------------------------------------------

describe('row content', () => {
  test('row shows parent ref, global path, and parent type', () => {
    renderSection({ entries: fiveEntries });
    // First row: ParentRef + global path (parentPath joined with within-parent path)
    expect(screen.getByText('10 0 R')).toBeInTheDocument();
    expect(screen.getByText('/Pages /Kids[0]')).toBeInTheDocument();
    // Type is rendered (we expect at least one /Pages occurrence; five rows share it)
    expect(screen.getAllByText('Pages').length).toBeGreaterThan(0);
  });

  test('row omits ParentType column when parentType is null', () => {
    renderSection({ entries: entryNoType });
    expect(screen.getByText('20 0 R')).toBeInTheDocument();
    // Global path = parentPath + within-parent path
    expect(screen.getByText('/Pages /Kids[0] /Resources /Font /F1')).toBeInTheDocument();
    // The string "null" or the JS null literal must NOT leak into the DOM
    expect(screen.queryByText('null')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Clicking a row dispatches NAVIGATE_TO_REF with the parent's
// indirect-object node ID. Keyboard parity required.
// ---------------------------------------------------------------------------

describe('row click dispatches NAVIGATE_TO_REF', () => {
  test('mouse click dispatches NAVIGATE_TO_REF with parentNodeId', async () => {
    const user = userEvent.setup();
    renderSection({ entries: fiveEntries });
    const row = screen.getByText('10 0 R').closest('[role="button"]');
    expect(row).not.toBeNull();
    await user.click(row!);
    expect(screen.getByTestId('pending-nav-target').textContent).toBe('obj:0:10');
  });

  test('Enter key dispatches NAVIGATE_TO_REF', async () => {
    const user = userEvent.setup();
    renderSection({ entries: fiveEntries });
    const row = screen.getByText('11 0 R').closest('[role="button"]') as HTMLElement;
    row.focus();
    await user.keyboard('{Enter}');
    expect(screen.getByTestId('pending-nav-target').textContent).toBe('obj:0:11');
  });

  test('Space key dispatches NAVIGATE_TO_REF', async () => {
    const user = userEvent.setup();
    renderSection({ entries: fiveEntries });
    const row = screen.getByText('12 0 R').closest('[role="button"]') as HTMLElement;
    row.focus();
    await user.keyboard(' ');
    expect(screen.getByTestId('pending-nav-target').textContent).toBe('obj:0:12');
  });

  test('row has role="button" and tabIndex=0 for keyboard a11y', () => {
    renderSection({ entries: fiveEntries });
    const row = screen.getByText('10 0 R').closest('[role="button"]');
    expect(row).not.toBeNull();
    expect(row).toHaveAttribute('tabindex', '0');
  });
});

// ---------------------------------------------------------------------------
// Catalog empty state -- "Document root..."
// ---------------------------------------------------------------------------

describe('catalog empty state', () => {
  test('catalog iconHint with empty entries renders "Document root..."', () => {
    renderSection({ entries: [], selectedIconHint: 'catalog' });
    expect(
      screen.getByText('Document root (no incoming references).')
    ).toBeInTheDocument();
  });

  test('catalog empty state does NOT show the orphan copy', () => {
    renderSection({ entries: [], selectedIconHint: 'catalog' });
    expect(
      screen.queryByText(/possible orphan/i)
    ).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Orphan empty state -- "No incoming dict-graph..."
// ---------------------------------------------------------------------------

describe('orphan empty state', () => {
  test('non-catalog iconHint with empty entries renders orphan copy', () => {
    renderSection({ entries: [], selectedIconHint: 'page' });
    expect(
      screen.getByText('No incoming dict-graph references (possible orphan).')
    ).toBeInTheDocument();
  });

  test('orphan empty-state copy retains the "dict-graph" qualifier (load-bearing)', () => {
    renderSection({ entries: [], selectedIconHint: 'page' });
    // The qualifier disclaims content-stream-only referents (R7 in story Risks).
    // If a future change drops it, this test fails immediately.
    expect(screen.getByText(/dict-graph/)).toBeInTheDocument();
  });

  test('orphan empty state does NOT show the catalog copy', () => {
    renderSection({ entries: [], selectedIconHint: 'page' });
    expect(
      screen.queryByText(/Document root/)
    ).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// index-unavailable banner has priority over the other empty states.
// ---------------------------------------------------------------------------

describe('index-unavailable banner', () => {
  test('indexUnavailable=true renders the unavailable banner', () => {
    renderSection({ entries: [], selectedIconHint: 'page', indexUnavailable: true });
    expect(
      screen.getByText('Reverse-ref index unavailable for this document.')
    ).toBeInTheDocument();
  });

  test('indexUnavailable wins over the orphan empty state', () => {
    renderSection({ entries: [], selectedIconHint: 'page', indexUnavailable: true });
    expect(screen.queryByText(/possible orphan/i)).not.toBeInTheDocument();
  });

  test('indexUnavailable wins over the catalog empty state', () => {
    renderSection({ entries: [], selectedIconHint: 'catalog', indexUnavailable: true });
    expect(screen.queryByText(/Document root/)).not.toBeInTheDocument();
  });

  test('indexUnavailable banner is shown even if entries are somehow provided', () => {
    // Defensive: backend should not return entries alongside the sentinel,
    // but the empty-state priority order puts unavailable first regardless.
    renderSection({ entries: fiveEntries, selectedIconHint: 'page', indexUnavailable: true });
    expect(
      screen.getByText('Reverse-ref index unavailable for this document.')
    ).toBeInTheDocument();
    // Rows must not render when the banner is up
    expect(screen.queryByText('10 0 R')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Count appears in header when there are entries
// ---------------------------------------------------------------------------

describe('section header count', () => {
  test('header shows entry count when entries are present', () => {
    renderSection({ entries: fiveEntries });
    expect(screen.getByText(/Referenced by \(5\)/)).toBeInTheDocument();
  });

  test('header omits the count parenthesis for empty entries (no count shown)', () => {
    renderSection({ entries: [], selectedIconHint: 'page' });
    // Either there's no header at all (acceptable for the empty-list UI)
    // or the header omits a number-in-parens. We assert NOTHING matches "(0)".
    expect(screen.queryByText(/Referenced by \(0\)/)).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Reset rule: remount on selection change resets the toggle to the default
// for the new entry count.
// ---------------------------------------------------------------------------

describe('remount-on-key resets toggle', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('changing the key prop forces a fresh default state', async () => {
    const user = userEvent.setup();

    // Render a 6-entry section keyed by 'A' -- default collapsed.
    const { rerender } = render(
      <AppProvider>
        <DispatchHelper action={openAction} />
        <ReverseRefsSection
          key="A"
          entries={sixEntries}
          selectedIconHint="page"
          indexUnavailable={false}
        />
      </AppProvider>
    );
    expect(screen.queryByText('10 0 R')).not.toBeInTheDocument();

    // User toggles open
    await user.click(screen.getByRole('button', { name: /Referenced by/i }));
    expect(screen.getByText('10 0 R')).toBeInTheDocument();

    // Now switch the key to 'B' with a NEW 6-entry list -- default must be
    // back to collapsed, regardless of the prior toggle.
    rerender(
      <AppProvider>
        <DispatchHelper action={openAction} />
        <ReverseRefsSection
          key="B"
          entries={sixEntries}
          selectedIconHint="page"
          indexUnavailable={false}
        />
      </AppProvider>
    );
    expect(screen.queryByText('10 0 R')).not.toBeInTheDocument();
  });
});
