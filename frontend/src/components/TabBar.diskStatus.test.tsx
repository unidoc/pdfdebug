/**
 * Tab dot for a file changed on disk: an indicator only. It sits in a fixed
 * slot between the file name and the close button that is reserved whether or
 * not the dot shows, is hidden from assistive tech (the tab's accessible name
 * and tooltip carry the text), and a click on it activates the tab like a
 * click anywhere else on the tab.
 *
 * Test ids: tab-disk-slot-<tabId> (always rendered), tab-disk-dot-<tabId>
 * (only while the tab is changed or missing).
 *
 * Run: cd frontend && npx vitest run src/components/TabBar.diskStatus.test.tsx
 */
import { render, screen, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import type { Dispatch } from 'react';
import { AppProvider, useAppDispatch, type AppAction } from '../hooks/useDocumentState';
import { applyDiskStatusEvent, resetDiskStatusStore } from '../lib/diskStatusStore';
import { TabBar } from './TabBar';

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn(() => vi.fn()),
    Emit: vi.fn(),
  },
}));

vi.mock('../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  OpenFile: vi.fn(),
  GetTreeRoot: vi.fn(),
  GetChildren: vi.fn(),
  CloseDocument: vi.fn().mockResolvedValue(undefined),
  OpenFileDialog: vi.fn(),
  GetDiskStatus: vi.fn(),
}));

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

let dispatch: Dispatch<AppAction>;
function Grab() {
  dispatch = useAppDispatch();
  return null;
}

function mount() {
  render(
    <AppProvider>
      <Grab />
      <TabBar />
    </AppProvider>,
  );
}

function open(tabId: string, filePath: string) {
  act(() =>
    dispatch({
      type: 'OPEN_DOCUMENT',
      payload: { tabId, fileName: filePath.split('/').pop() ?? '', filePath, pageCount: 1, rootNode: catalogNode, rootChildren: [] },
    }),
  );
}

// 2026-01-12, local time.
const today = (h: number, m: number) => new Date(2026, 0, 12, h, m).getTime();

function mark(tabId: string, seq: number, state: 'changed' | 'missing' | 'current') {
  act(() =>
    applyDiskStatusEvent({
      tabId,
      seq,
      state,
      loadedModTime: today(14, 0),
      diskModTime: state === 'missing' ? 0 : today(14, 5),
      diskSize: state === 'missing' ? 0 : 2048,
    }),
  );
}

const originalPlatform = Object.getOwnPropertyDescriptor(window.navigator, 'platform');

beforeEach(() => {
  resetDiskStatusStore();
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(new Date(2026, 0, 12, 18, 0));
});

afterEach(() => {
  vi.useRealTimers();
  if (originalPlatform) Object.defineProperty(window.navigator, 'platform', originalPlatform);
});

describe('tab dot', () => {
  test('a stale active tab and a stale background tab both show the dot', () => {
    mount();
    open('a1', '/docs/a.pdf');
    open('b1', '/docs/b.pdf');
    mark('a1', 1, 'changed');
    mark('b1', 1, 'missing');
    expect(screen.getByTestId('tab-disk-dot-a1')).toBeInTheDocument();
    expect(screen.getByTestId('tab-disk-dot-b1')).toBeInTheDocument();
  });

  test('a tab that is current shows no dot but keeps its slot', () => {
    mount();
    open('a1', '/docs/a.pdf');
    expect(screen.queryByTestId('tab-disk-dot-a1')).not.toBeInTheDocument();
    expect(screen.getByTestId('tab-disk-slot-a1')).toBeInTheDocument();
  });

  test('the dot is aria-hidden, not focusable and not a control', () => {
    mount();
    open('a1', '/docs/a.pdf');
    mark('a1', 1, 'changed');
    const dot = screen.getByTestId('tab-disk-dot-a1');
    expect(dot).toHaveAttribute('aria-hidden', 'true');
    expect(dot.tagName).toBe('SPAN');
    expect(dot).not.toHaveAttribute('tabindex');
    expect(dot).not.toHaveAttribute('role');
    expect(dot.querySelector('button, a, [tabindex]')).toBeNull();
  });

  test('clicking the dot activates its tab like clicking the label', async () => {
    const user = userEvent.setup();
    mount();
    open('a1', '/docs/a.pdf');
    open('b1', '/docs/b.pdf');
    mark('a1', 1, 'changed');
    expect(screen.getByTestId('tab-a1')).toHaveAttribute('aria-selected', 'false');

    await user.click(screen.getByTestId('tab-disk-dot-a1'));

    expect(screen.getByTestId('tab-a1')).toHaveAttribute('aria-selected', 'true');
  });

  test('the slot sits between the file name and the close button with the same classes stale or not', () => {
    mount();
    open('a1', '/docs/a.pdf');
    const slotBefore = screen.getByTestId('tab-disk-slot-a1').className;
    const closeBefore = screen.getByTestId('tab-close-a1').className;

    mark('a1', 1, 'changed');

    const slot = screen.getByTestId('tab-disk-slot-a1');
    const close = screen.getByTestId('tab-close-a1');
    const name = screen.getByText('a.pdf', { selector: 'span' });
    expect(slot.className).toBe(slotBefore);
    expect(close.className).toBe(closeBefore);
    expect(name.compareDocumentPosition(slot) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(slot.compareDocumentPosition(close) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(slot.contains(close)).toBe(false);
    expect(slot.contains(screen.getByTestId('tab-disk-dot-a1'))).toBe(true);
  });

  test('the dot goes away when the tab returns to current', () => {
    mount();
    open('a1', '/docs/a.pdf');
    mark('a1', 1, 'changed');
    mark('a1', 2, 'current');
    expect(screen.queryByTestId('tab-disk-dot-a1')).not.toBeInTheDocument();
  });
});

describe('tab tooltip and accessible name', () => {
  test('changed: tooltip adds the disk time and the Ctrl+R hint off macOS', () => {
    Object.defineProperty(window.navigator, 'platform', { value: 'Win32', configurable: true });
    mount();
    open('a1', '/docs/a.pdf');
    mark('a1', 1, 'changed');
    expect(screen.getByTestId('tab-a1')).toHaveAttribute(
      'title',
      '/docs/a.pdf\nDocument updated on disk 14:05 - press Ctrl+R to reload',
    );
  });

  test('changed: tooltip says Cmd+R on macOS', () => {
    Object.defineProperty(window.navigator, 'platform', { value: 'MacIntel', configurable: true });
    mount();
    open('a1', '/docs/a.pdf');
    mark('a1', 1, 'changed');
    expect(screen.getByTestId('tab-a1').getAttribute('title')).toContain('press Cmd+R to reload');
  });

  test('missing: tooltip says the file is no longer found', () => {
    mount();
    open('a1', '/docs/a.pdf');
    mark('a1', 1, 'missing');
    expect(screen.getByTestId('tab-a1')).toHaveAttribute('title', '/docs/a.pdf\nFile no longer found at this path');
  });

  test('current: tooltip is the file path alone', () => {
    mount();
    open('a1', '/docs/a.pdf');
    expect(screen.getByTestId('tab-a1')).toHaveAttribute('title', '/docs/a.pdf');
  });

  test('accessible name carries the change through visually hidden text', () => {
    mount();
    open('a1', '/docs/a.pdf');
    open('b1', '/docs/b.pdf');
    mark('a1', 1, 'changed');
    mark('b1', 1, 'missing');
    expect(screen.getByRole('tab', { name: /a\.pdf, changed on disk/ })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: /b\.pdf, file not found/ })).toBeInTheDocument();
  });

  test('accessible name has no change text for a current tab', () => {
    mount();
    open('a1', '/docs/a.pdf');
    expect(screen.getByRole('tab', { name: /a\.pdf/ }).textContent).not.toMatch(/changed on disk|file not found/);
  });
});
