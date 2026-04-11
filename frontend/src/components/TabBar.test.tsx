/**
 * Story 4.1: Tab Bar Component for Multi-Document Management
 *
 * TDD RED PHASE: Tests MUST fail until TabBar.tsx is implemented.
 *
 * Component-level acceptance tests for TabBar:
 *   4.1-UNIT-004 [P0]: TabBar renders one tab per open document with correct file names
 *   4.1-UNIT-005 [P1]: TabBar highlights the active tab (aria-selected)
 *   4.1-UNIT-006 [P1]: TabBar tab has close button visible on hover
 *   4.1-UNIT-007 [P1]: Cmd/Ctrl+Tab cycles to next tab; Cmd/Ctrl+Shift+Tab cycles to previous
 *   4.1-UNIT-008 [P1]: TabBar uses Radix UI Tabs with proper ARIA roles (tablist, tab, aria-selected)
 *   4.1-UNIT-009 [P1]: Tab switch is instant -- no loading state, no backend calls
 *   4.1-UNIT-011 [P2]: Tab label shows truncated file name with tooltip showing full path
 *   4.1-UNIT-012 [P2]: Tab bar scrolls horizontally when too many tabs to fit
 *
 * Run: cd frontend && npx vitest run src/components/TabBar.test.tsx
 */
import { render, screen, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import {
  AppProvider,
  useAppState,
  useAppDispatch,
  type AppAction,
} from '../hooks/useDocumentState';
// RED PHASE: This import will fail until TabBar.tsx is created.
import { TabBar } from './TabBar';

// Mock Wails bindings -- TabBar calls CloseDocument on tab close
const mockCloseDocument = vi.fn();
vi.mock(
  '../../bindings/unipdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: vi.fn(),
    CloseDocument: (...args: unknown[]) => mockCloseDocument(...args),
    OpenFileDialog: vi.fn(),
    GetObjectDetail: vi.fn(),
    GetContentStream: vi.fn(),
  })
);

beforeEach(() => {
  vi.clearAllMocks();
});

// --- Test fixtures ---

const catalogNode = {
  id: 'root',
  label: 'Catalog',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 2,
  iconHint: 'catalog',
  error: '',
};

const childNodes = [
  {
    id: 'dict:root:Type',
    label: 'Type',
    rawKey: '/Type',
    nodeType: 'scalar',
    valueType: 'name',
    hasChildren: false,
    childCount: 0,
    iconHint: 'default',
    error: '',
  },
];

/**
 * Helper: opens N documents then renders TabBar.
 * Each document gets tabId "tab-{i+1}", fileName "doc-{i+1}.pdf",
 * filePath "/path/to/doc-{i+1}.pdf".
 */
function SetupAndRenderTabBar({ tabCount }: { tabCount: number }) {
  const dispatch = useAppDispatch();
  const state = useAppState();
  return (
    <div>
      <span data-testid="active-tab-id">{state.activeTabId ?? 'null'}</span>
      <span data-testid="tab-count">{state.tabs.length}</span>
      {Array.from({ length: tabCount }, (_, i) => (
        <button
          key={i}
          data-testid={`open-doc-${i + 1}`}
          onClick={() =>
            dispatch({
              type: 'OPEN_DOCUMENT',
              payload: {
                tabId: `tab-${i + 1}`,
                fileName: `doc-${i + 1}.pdf`,
                filePath: `/path/to/doc-${i + 1}.pdf`,
                rootNode: catalogNode,
                rootChildren: childNodes,
              },
            } as AppAction)
          }
        />
      ))}
      <TabBar />
    </div>
  );
}

/** Open N tabs by clicking each open button. */
function openTabs(count: number) {
  for (let i = 1; i <= count; i++) {
    act(() => screen.getByTestId(`open-doc-${i}`).click());
  }
}

describe('4.1 TabBar Component Tests', () => {
  /**
   * 4.1-UNIT-004 [P0]: TabBar renders one tab per open document with correct file names.
   *
   * RED PHASE: TabBar.tsx does not exist yet.
   */
  test('4.1-UNIT-004 [P0]: renders one tab per open document with correct file names', () => {
    render(
      <AppProvider>
        <SetupAndRenderTabBar tabCount={3} />
      </AppProvider>
    );

    openTabs(3);

    // TabBar should render a tab-bar container
    const tabBar = screen.getByTestId('tab-bar');
    expect(tabBar).toBeInTheDocument();

    // Should have a tab list
    const tabList = screen.getByTestId('tab-list');
    expect(tabList).toBeInTheDocument();

    // Should render 3 tabs with correct labels
    expect(screen.getByTestId('tab-tab-1')).toBeInTheDocument();
    expect(screen.getByTestId('tab-tab-2')).toBeInTheDocument();
    expect(screen.getByTestId('tab-tab-3')).toBeInTheDocument();

    expect(screen.getByTestId('tab-tab-1')).toHaveTextContent('doc-1.pdf');
    expect(screen.getByTestId('tab-tab-2')).toHaveTextContent('doc-2.pdf');
    expect(screen.getByTestId('tab-tab-3')).toHaveTextContent('doc-3.pdf');
  });

  /**
   * 4.1-UNIT-005 [P1]: TabBar highlights the active tab with visual distinction
   * (aria-selected).
   *
   * RED PHASE: TabBar.tsx does not exist yet.
   */
  test('4.1-UNIT-005 [P1]: highlights the active tab with aria-selected', () => {
    render(
      <AppProvider>
        <SetupAndRenderTabBar tabCount={2} />
      </AppProvider>
    );

    openTabs(2);

    // After opening 2 tabs, tab-2 should be active (last opened)
    const tab1 = screen.getByTestId('tab-tab-1');
    const tab2 = screen.getByTestId('tab-tab-2');

    expect(tab2).toHaveAttribute('aria-selected', 'true');
    expect(tab1).toHaveAttribute('aria-selected', 'false');
  });

  /**
   * 4.1-UNIT-006 [P1]: TabBar tab has close button visible on hover.
   *
   * RED PHASE: TabBar.tsx does not exist yet.
   * Note: Hover visibility is CSS-only (opacity-0 -> opacity-100 on group-hover).
   * We verify the button exists in the DOM with correct test ID and aria-label.
   */
  test('4.1-UNIT-006 [P1]: tab has close button with correct aria-label', () => {
    render(
      <AppProvider>
        <SetupAndRenderTabBar tabCount={2} />
      </AppProvider>
    );

    openTabs(2);

    // Each tab should have a close button
    const closeBtn1 = screen.getByTestId('tab-close-tab-1');
    const closeBtn2 = screen.getByTestId('tab-close-tab-2');

    expect(closeBtn1).toBeInTheDocument();
    expect(closeBtn2).toBeInTheDocument();

    // Close buttons should have accessible labels
    expect(closeBtn1).toHaveAttribute('aria-label', 'Close doc-1.pdf');
    expect(closeBtn2).toHaveAttribute('aria-label', 'Close doc-2.pdf');
  });

  /**
   * 4.1-UNIT-007 [P1]: Cmd/Ctrl+Tab cycles to next tab;
   * Cmd/Ctrl+Shift+Tab cycles to previous.
   *
   * RED PHASE: TabBar.tsx does not exist yet. Keyboard handler not registered.
   */
  test('4.1-UNIT-007 [P1]: Ctrl+Tab cycles to next tab, Ctrl+Shift+Tab cycles to previous', () => {
    render(
      <AppProvider>
        <SetupAndRenderTabBar tabCount={3} />
      </AppProvider>
    );

    openTabs(3);

    // Tab 3 is active (last opened)
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-3');

    // Ctrl+Tab -> next tab (wraps to tab-1)
    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', {
          key: 'Tab',
          ctrlKey: true,
          bubbles: true,
        })
      );
    });
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');

    // Ctrl+Tab -> tab-2
    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', {
          key: 'Tab',
          ctrlKey: true,
          bubbles: true,
        })
      );
    });
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2');

    // Ctrl+Shift+Tab -> back to tab-1
    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', {
          key: 'Tab',
          ctrlKey: true,
          shiftKey: true,
          bubbles: true,
        })
      );
    });
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');
  });

  /**
   * 4.1-UNIT-008 [P1]: TabBar uses Radix UI Tabs with proper ARIA roles
   * (tablist, tab, aria-selected).
   *
   * Note: Per story AC #4, do NOT assert role="tabpanel" -- we don't use
   * Tabs.Content. The test design doc lists "tabpanel" for this test ID
   * but that is a spec error for our architecture.
   *
   * RED PHASE: TabBar.tsx does not exist yet.
   */
  test('4.1-UNIT-008 [P1]: uses Radix UI Tabs with proper ARIA roles', () => {
    render(
      <AppProvider>
        <SetupAndRenderTabBar tabCount={2} />
      </AppProvider>
    );

    openTabs(2);

    // Should have a tablist role
    const tablist = screen.getByRole('tablist');
    expect(tablist).toBeInTheDocument();

    // Each tab should have role="tab"
    const tabs = screen.getAllByRole('tab');
    expect(tabs).toHaveLength(2);

    // Active tab has aria-selected="true"
    const activeTab = tabs.find(
      (t) => t.getAttribute('aria-selected') === 'true'
    );
    expect(activeTab).toBeDefined();
    expect(activeTab).toHaveTextContent('doc-2.pdf');

    // Inactive tab has aria-selected="false"
    const inactiveTab = tabs.find(
      (t) => t.getAttribute('aria-selected') === 'false'
    );
    expect(inactiveTab).toBeDefined();
    expect(inactiveTab).toHaveTextContent('doc-1.pdf');

    // Explicitly: no tabpanel role should exist (we don't use Tabs.Content)
    expect(screen.queryByRole('tabpanel')).not.toBeInTheDocument();
  });

  /**
   * 4.1-UNIT-009 [P1]: Tab switch is instant -- no loading state shown,
   * no backend calls.
   *
   * RED PHASE: TabBar.tsx does not exist yet.
   */
  test('4.1-UNIT-009 [P1]: tab switch triggers no backend calls and shows no loading state', async () => {
    const user = userEvent.setup();
    render(
      <AppProvider>
        <SetupAndRenderTabBar tabCount={2} />
      </AppProvider>
    );

    openTabs(2);

    // Clear mock call counts from opening docs
    mockCloseDocument.mockClear();

    // Click tab-1 to switch (userEvent fires mousedown which Radix handles)
    await user.click(screen.getByTestId('tab-tab-1'));

    // Active tab should switch
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');

    // No backend calls should have been made
    expect(mockCloseDocument).not.toHaveBeenCalled();

    // No loading indicator should be present
    expect(screen.queryByText(/loading/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/decoding/i)).not.toBeInTheDocument();
  });

  /**
   * 4.1-UNIT-011 [P2]: Tab label shows truncated file name with tooltip
   * showing full path.
   *
   * RED PHASE: TabBar.tsx does not exist yet.
   */
  test('4.1-UNIT-011 [P2]: tab trigger has title attribute with full file path', () => {
    render(
      <AppProvider>
        <SetupAndRenderTabBar tabCount={1} />
      </AppProvider>
    );

    openTabs(1);

    const tab = screen.getByTestId('tab-tab-1');
    expect(tab).toHaveAttribute('title', '/path/to/doc-1.pdf');
  });

  /**
   * 4.1-UNIT-012 [P2]: Tab bar scrolls horizontally when too many tabs to fit.
   *
   * RED PHASE: TabBar.tsx does not exist yet.
   * Note: This is a CSS behavior test -- we verify the overflow class is present
   * on the tab list element.
   */
  test('4.1-UNIT-012 [P2]: tab list has horizontal scroll overflow class', () => {
    render(
      <AppProvider>
        <SetupAndRenderTabBar tabCount={10} />
      </AppProvider>
    );

    openTabs(10);

    const tabList = screen.getByTestId('tab-list');
    expect(tabList.className).toMatch(/overflow-x-auto/);
  });

  /**
   * 4.1-UNIT-006 supplemental: clicking close button dispatches CLOSE_DOCUMENT
   * and calls backend CloseDocument.
   * // Also covers 4.3-UNIT-004
   *
   * RED PHASE: TabBar.tsx does not exist yet.
   */
  test('4.1-UNIT-006 supplemental: close button dispatches CLOSE_DOCUMENT and calls backend', () => {
    render(
      <AppProvider>
        <SetupAndRenderTabBar tabCount={2} />
      </AppProvider>
    );

    openTabs(2);
    mockCloseDocument.mockClear();

    // Close tab-1 (not the active tab)
    act(() => {
      screen.getByTestId('tab-close-tab-1').click();
    });

    // Tab should be removed
    expect(screen.getByTestId('tab-count').textContent).toBe('1');
    expect(screen.queryByTestId('tab-tab-1')).not.toBeInTheDocument();
    expect(screen.getByTestId('tab-tab-2')).toBeInTheDocument();

    // Active tab must remain unchanged (tab-2 was active, still is)
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2');

    // Backend CloseDocument should have been called with the tab ID
    expect(mockCloseDocument).toHaveBeenCalledWith('tab-1');
  });

  /**
   * 4.1-UNIT-007 supplemental: Ctrl+W closes the active tab.
   * // Also covers 4.3-UNIT-005
   *
   * RED PHASE: TabBar.tsx does not exist yet. Keyboard handler not registered.
   */
  test('4.1-UNIT-007 supplemental: Ctrl+W closes the active tab', () => {
    render(
      <AppProvider>
        <SetupAndRenderTabBar tabCount={2} />
      </AppProvider>
    );

    openTabs(2);
    mockCloseDocument.mockClear();

    // Tab-2 is active. Ctrl+W should close it.
    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', {
          key: 'w',
          ctrlKey: true,
          bubbles: true,
        })
      );
    });

    expect(screen.getByTestId('tab-count').textContent).toBe('1');
    expect(mockCloseDocument).toHaveBeenCalledWith('tab-2');
    // Tab-1 should now be active (fallback)
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');
  });

  /**
   * 4.3-UNIT-006 [P1]: No confirmation dialog on tab close (read-only app,
   * nothing to lose).
   * AC#3: No confirmation dialog is shown when closing a tab.
   *
   * Given a tab is open,
   * When the close button is clicked,
   * Then the tab is removed immediately with no dialog rendered.
   */
  test('4.3-UNIT-006 [P1]: no confirmation dialog on tab close', () => {
    render(
      <AppProvider>
        <SetupAndRenderTabBar tabCount={1} />
      </AppProvider>
    );

    openTabs(1);
    expect(screen.getByTestId('tab-tab-1')).toBeInTheDocument();

    // Click close button
    act(() => {
      screen.getByTestId('tab-close-tab-1').click();
    });

    // Tab should be removed immediately
    expect(screen.getByTestId('tab-count').textContent).toBe('0');
    expect(screen.queryByTestId('tab-tab-1')).not.toBeInTheDocument();

    // No confirmation dialog should exist in the DOM
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  });

  /**
   * 4.1-UNIT-009 supplemental: clicking a tab triggers ACTIVATE_TAB, not
   * OPEN_DOCUMENT or any data-fetching action.
   *
   * RED PHASE: TabBar.tsx does not exist yet.
   */
  test('4.1-UNIT-009 supplemental: clicking tab switches active tab immediately', async () => {
    const user = userEvent.setup();
    render(
      <AppProvider>
        <SetupAndRenderTabBar tabCount={3} />
      </AppProvider>
    );

    openTabs(3);
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-3');

    // Click tab-1 (userEvent fires mousedown which Radix handles)
    await user.click(screen.getByTestId('tab-tab-1'));
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-1');

    // Click tab-2
    await user.click(screen.getByTestId('tab-tab-2'));
    expect(screen.getByTestId('active-tab-id').textContent).toBe('tab-2');

    // All 3 tabs still exist (no tabs removed)
    expect(screen.getByTestId('tab-count').textContent).toBe('3');
  });
});
