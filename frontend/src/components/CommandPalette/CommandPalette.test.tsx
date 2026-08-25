/**
 * Command Palette (Cmd+K) -- inline labels + jump-to-object.
 *
 * Covers palette-side wiring.
 *
 * Approach: mock the Wails binding so GetObjectIndex returns a deterministic
 * fixture. Render <App-shell-equivalent> with AppProvider and assert against
 * the rendered palette overlay and the reducer's pendingNavTarget state.
 *
 * Run: cd frontend && npx vitest run \
 * src/components/CommandPalette/CommandPalette.test.tsx
 */
import { render, screen, act, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import {
  AppProvider,
  useAppDispatch,
  useAppState,
  type AppAction,
} from '../../hooks/useDocumentState';

import { CommandPalette } from './CommandPalette';
import { useCommandPalette } from '../../hooks/useCommandPalette';

// --- Wails binding mock. GetObjectIndex is the new IPC. Stub the whole
// binding module so this test never touches Go. ---
const mockGetObjectIndex = vi.hoisted(() => vi.fn());
const mockGetAncestorPath = vi.hoisted(() => vi.fn());

// The palette open shortcut is now platform-aware (Cmd on macOS,
// Ctrl elsewhere). Default the mock to 'Cmd' so the Meta+K cases below open
// the palette; the dedicated Ctrl+K test overrides it to 'Ctrl'.
const mockGetPlatformModifier = vi.hoisted(() => vi.fn(() => 'Cmd'));
vi.mock('../../lib/platform', () => ({
  getPlatformModifier: () => mockGetPlatformModifier(),
  getShortcutHint: (key: string) => `${mockGetPlatformModifier()}+${key}`,
}));

vi.mock(
  '../../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: vi.fn(),
    CloseDocument: vi.fn(),
    OpenFileDialog: vi.fn(),
    GetObjectDetail: vi.fn(),
    GetAncestorPath: (...args: unknown[]) => mockGetAncestorPath(...args),
    GetObjectIndex: (...args: unknown[]) => mockGetObjectIndex(...args),
    GetReverseRefs: vi.fn(),
    GoToPage: vi.fn(),
    GetContentStream: vi.fn(),
    GetImageData: vi.fn(),
    GetObjectSource: vi.fn(),
  }),
);

// --- Test fixture: a tiny index. ObjNums are deliberately spread so
// "no object N" assertions can pin the "highest object" message. ---
const indexFixture = [
  { objNum: 1, gen: 0, typeName: 'Catalog', free: false, reachable: true, nodeId: 'obj:0:1' },
  { objNum: 2, gen: 0, typeName: 'Pages', free: false, reachable: true, nodeId: 'obj:0:2' },
  { objNum: 3, gen: 0, typeName: 'Page', free: false, reachable: true, nodeId: 'obj:0:3' },
  { objNum: 5, gen: 0, typeName: 'Font', free: false, reachable: true, nodeId: 'obj:0:5' },
  { objNum: 6, gen: 0, typeName: 'FontDescriptor', free: false, reachable: true, nodeId: 'obj:0:6' },
  { objNum: 9, gen: 0, typeName: '', free: true, reachable: false, nodeId: '' },
];

const catalog = {
  id: 'root', label: 'Catalog', rawKey: '', nodeType: 'dict', valueType: '',
  hasChildren: true, childCount: 0, iconHint: 'catalog', error: '',
};

const openTab = (tabId: string): AppAction => ({
  type: 'OPEN_DOCUMENT',
  payload: {
    tabId,
    fileName: `${tabId}.pdf`,
    filePath: `/${tabId}.pdf`,
    pageCount: 10,
    rootNode: catalog,
    rootChildren: [],
  },
});

// Probe component to surface reducer state into the DOM for assertions.
function StateProbe() {
  const state = useAppState();
  const tab = state.tabs.find((t) => t.tabId === state.activeTabId) ?? null;
  return (
    <div>
      <span data-testid="pending-nav-target">{tab?.pendingNavTarget ?? 'null'}</span>
      <span data-testid="active-tab-id">{state.activeTabId ?? 'null'}</span>
    </div>
  );
}

// Test harness: mount the palette + hook + a button that opens a tab.
function Harness({ initialTabs = 1 }: { initialTabs?: number }) {
  const dispatch = useAppDispatch();
  // useCommandPalette wires the global Cmd+K listener. Tests trigger it
  // via userEvent.keyboard so we exercise the real key dispatch path.
  useCommandPalette();
  return (
    <div>
      <button data-testid="bootstrap-open" onClick={() => {
        for (let i = 1; i <= initialTabs; i++) {
          dispatch(openTab(`tab-${i}`));
        }
      }}>open</button>
      <button data-testid="switch-tab-2" onClick={() => {
        dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: 'tab-2' } });
      }}>switch</button>
      <StateProbe />
      <CommandPalette />
    </div>
  );
}

function renderHarness(opts?: { initialTabs?: number }) {
  return render(
    <AppProvider>
      <Harness initialTabs={opts?.initialTabs ?? 1} />
    </AppProvider>,
  );
}

beforeEach(() => {
  mockGetObjectIndex.mockReset();
  mockGetAncestorPath.mockReset();
  mockGetObjectIndex.mockResolvedValue(indexFixture);
  mockGetAncestorPath.mockResolvedValue(['root', 'obj:0:2', 'obj:0:3']);
  mockGetPlatformModifier.mockReturnValue('Cmd');
});

// ---------------------------------------------------------------------------
// Cmd+K opens, Esc closes, click-outside closes, focus trap/restore
// ---------------------------------------------------------------------------

describe('open/close lifecycle', () => {
  test('Cmd+K opens the palette overlay', async () => {
    const user = userEvent.setup();
    renderHarness();
    act(() => screen.getByTestId('bootstrap-open').click());

    await user.keyboard('{Meta>}k{/Meta}');

    expect(await screen.findByTestId('command-palette')).toBeInTheDocument();
  });

  test('Ctrl+K opens the palette overlay (Windows/Linux)', async () => {
    mockGetPlatformModifier.mockReturnValue('Ctrl');
    const user = userEvent.setup();
    renderHarness();
    act(() => screen.getByTestId('bootstrap-open').click());

    await user.keyboard('{Control>}k{/Control}');

    expect(await screen.findByTestId('command-palette')).toBeInTheDocument();
  });

  test('Esc closes the palette', async () => {
    const user = userEvent.setup();
    renderHarness();
    act(() => screen.getByTestId('bootstrap-open').click());
    await user.keyboard('{Meta>}k{/Meta}');
    expect(screen.getByTestId('command-palette')).toBeInTheDocument();

    await user.keyboard('{Escape}');

    await waitFor(() => expect(screen.queryByTestId('command-palette')).not.toBeInTheDocument());
  });
});

// ---------------------------------------------------------------------------
// Numeric query, single-match Enter, navigation dispatch
// ---------------------------------------------------------------------------

describe('numeric query Enter -> NAVIGATE_TO_REF', () => {
  test('single-match Enter dispatches NAVIGATE_TO_REF and closes palette', async () => {
    const user = userEvent.setup();
    renderHarness();
    act(() => screen.getByTestId('bootstrap-open').click());

    await user.keyboard('{Meta>}k{/Meta}');
    const input = await screen.findByTestId('command-palette-input');
    await user.type(input, '3');

    // 50ms idle gate. Pause longer than that before Enter.
    await new Promise((r) => setTimeout(r, 80));
    await user.keyboard('{Enter}');

    await waitFor(() => {
      expect(screen.getByTestId('pending-nav-target').textContent).toBe('obj:0:3');
    });
    await waitFor(() => expect(screen.queryByTestId('command-palette')).not.toBeInTheDocument());
  });

  test('Enter pressed during typing (< 50ms idle) is ignored', async () => {
    // We cannot easily synchronize keystrokes to be strictly faster than
    // 50ms with userEvent, so this test fires Enter with NO intermediate
    // idle pause -- userEvent.type queues keystrokes back-to-back, so the
    // last keystroke and the Enter are dispatched within the same
    // microtask. Implementations that respect the gate will not commit.
    const user = userEvent.setup();
    renderHarness();
    act(() => screen.getByTestId('bootstrap-open').click());

    await user.keyboard('{Meta>}k{/Meta}');
    const input = await screen.findByTestId('command-palette-input');
    // Last keystroke + immediate Enter, no settle window.
    await user.type(input, '3{Enter}');

    // 50ms gate must drop the Enter. Palette stays open, no nav fired.
    expect(screen.getByTestId('command-palette')).toBeInTheDocument();
    expect(screen.getByTestId('pending-nav-target').textContent).toBe('null');
  });

  test('no-match numeric input shows "No object N -- highest is M" empty state', async () => {
    const user = userEvent.setup();
    renderHarness();
    act(() => screen.getByTestId('bootstrap-open').click());

    await user.keyboard('{Meta>}k{/Meta}');
    const input = await screen.findByTestId('command-palette-input');
    await user.type(input, '99999');

    // Highest objNum in indexFixture is 9.
    const empty = await screen.findByTestId('command-palette-empty');
    expect(empty.textContent).toMatch(/no object 99999/i);
    expect(empty.textContent).toMatch(/highest object.*9/i);
  });
});

// ---------------------------------------------------------------------------
// multi-match: arrow keys + Enter
// ---------------------------------------------------------------------------

describe('multi-match arrow navigation', () => {
  test('prefix "Font" yields multi-match list; ArrowDown + Enter commits second row', async () => {
    const user = userEvent.setup();
    renderHarness();
    act(() => screen.getByTestId('bootstrap-open').click());

    await user.keyboard('{Meta>}k{/Meta}');
    const input = await screen.findByTestId('command-palette-input');
    await user.type(input, 'Font');

    const rows = await screen.findAllByTestId('command-palette-row');
    expect(rows.length).toBeGreaterThanOrEqual(2);

    await user.keyboard('{ArrowDown}');
    await new Promise((r) => setTimeout(r, 80));
    await user.keyboard('{Enter}');

    // Second row by ranker order: FontDescriptor (obj 6) follows Font (obj 5).
    await waitFor(() => {
      expect(screen.getByTestId('pending-nav-target').textContent).toBe('obj:0:6');
    });
  });
});

// ---------------------------------------------------------------------------
// Empty input shows per-tab recents (max 5)
// ---------------------------------------------------------------------------

describe('recent jumps', () => {
  test('first-time empty input shows grammar hint and no Recent header', async () => {
    const user = userEvent.setup();
    renderHarness();
    act(() => screen.getByTestId('bootstrap-open').click());

    await user.keyboard('{Meta>}k{/Meta}');

    expect(await screen.findByTestId('command-palette-grammar-hint')).toBeInTheDocument();
    expect(screen.queryByTestId('command-palette-recent-header')).not.toBeInTheDocument();
  });

  test('after a successful jump, empty input shows the Recent header above the entries', async () => {
    const user = userEvent.setup();
    renderHarness();
    act(() => screen.getByTestId('bootstrap-open').click());

    // First jump: object 5.
    await user.keyboard('{Meta>}k{/Meta}');
    await user.type(await screen.findByTestId('command-palette-input'), '5');
    await new Promise((r) => setTimeout(r, 80));
    await user.keyboard('{Enter}');
    await waitFor(() => expect(screen.queryByTestId('command-palette')).not.toBeInTheDocument());

    // Re-open. Empty input should show the recent and the "Recent" header.
    await user.keyboard('{Meta>}k{/Meta}');
    const recents = await screen.findAllByTestId('command-palette-recent-row');
    expect(recents.length).toBeGreaterThanOrEqual(1);
    expect(recents[0].textContent).toMatch(/5 0 R/);
    const header = await screen.findByTestId('command-palette-recent-header');
    expect(header).toBeInTheDocument();
    expect(header.textContent).toBe('Recent');
  });
});

// ---------------------------------------------------------------------------
// free/orphan rows render but are non-navigable
// ---------------------------------------------------------------------------

describe('free/orphan rows', () => {
  test('typing a free object number shows the row tagged (free) and Enter is a no-op', async () => {
    const user = userEvent.setup();
    renderHarness();
    act(() => screen.getByTestId('bootstrap-open').click());

    await user.keyboard('{Meta>}k{/Meta}');
    await user.type(await screen.findByTestId('command-palette-input'), '9');

    const row = await screen.findByTestId('command-palette-row');
    expect(row.textContent).toMatch(/\(free\)/i);

    await new Promise((r) => setTimeout(r, 80));
    await user.keyboard('{Enter}');

    // Palette stays open and pendingNavTarget remains null.
    expect(screen.getByTestId('command-palette')).toBeInTheDocument();
    expect(screen.getByTestId('pending-nav-target').textContent).toBe('null');

    // Inline non-navigable notice surfaces.
    expect(await screen.findByTestId('command-palette-unreachable-notice')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Tab switch closes the palette; recents are per-tab
// ---------------------------------------------------------------------------

describe('tab switching', () => {
  test('switching tabs while palette is open closes it', async () => {
    const user = userEvent.setup();
    renderHarness({ initialTabs: 2 });
    act(() => screen.getByTestId('bootstrap-open').click());

    await user.keyboard('{Meta>}k{/Meta}');
    expect(await screen.findByTestId('command-palette')).toBeInTheDocument();

    act(() => screen.getByTestId('switch-tab-2').click());

    await waitFor(() => expect(screen.queryByTestId('command-palette')).not.toBeInTheDocument());
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2');
  });
});
