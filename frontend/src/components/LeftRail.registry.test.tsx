/**
 * The rail renders from its destination registry. With a three-entry registry
 * the rail shows three items, and every positional behaviour - order, roving
 * focus bounds, Cmd/Ctrl+digit, the panel switch and the persisted view id -
 * follows the array without any other edit.
 *
 * Run: cd frontend && npx vitest run src/components/LeftRail.registry.test.tsx
 */
import { render, screen, fireEvent, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import type { ReactNode } from 'react';
import { AppProvider } from '../hooks/useDocumentState';
import { MainLayout } from './MainLayout';

const RAIL_KEY = 'unidoc-pdf-debugger:left-rail';

vi.mock('./leftRailDestinations', async () => {
  const { ListTree, Files, Image } = await import('lucide-react');
  const panel = (id: string) => {
    function Panel() {
      return <div data-testid={`stub-panel-${id}`}>{id}</div>;
    }
    return Panel;
  };
  return {
    LEFT_RAIL_DESTINATIONS: [
      { id: 'structure', label: 'Structure', icon: ListTree, panel: panel('structure') },
      { id: 'pages', label: 'Pages', icon: Files, panel: panel('pages') },
      { id: 'images', label: 'Images', icon: Image, panel: panel('images') },
    ],
  };
});

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

vi.mock('../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  OpenFile: vi.fn(),
  GetTreeRoot: vi.fn(),
  GetChildren: vi.fn().mockResolvedValue([]),
  CloseDocument: vi.fn(),
  OpenFileDialog: vi.fn(),
  GetObjectDetail: vi.fn().mockResolvedValue(null),
  GetObjectSource: vi.fn().mockResolvedValue(''),
  GetReverseRefs: vi.fn().mockResolvedValue([]),
  GetAncestorPath: vi.fn().mockResolvedValue([]),
  GetXRefTable: vi.fn().mockResolvedValue({ tabId: '', entries: [] }),
  GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
  GetSignatures: vi.fn().mockResolvedValue([]),
  GetEmbeddedFileBytes: vi.fn().mockResolvedValue(''),
  GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '', warning: '' }),
  SaveBytesToFile: vi.fn().mockResolvedValue(''),
  DiffDocuments: vi.fn().mockResolvedValue({ root: null, summary: {} }),
  GetPageIndex: vi.fn().mockResolvedValue([]),
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

function rail() {
  const lists = screen.getAllByRole('tablist').filter((l) => l.getAttribute('aria-orientation') === 'vertical');
  expect(lists).toHaveLength(1);
  return lists[0];
}

function tab(name: string) {
  return within(rail()).getByRole('tab', { name });
}

function panelFor(tabEl: HTMLElement) {
  const panel = document.getElementById(tabEl.getAttribute('aria-controls') ?? '');
  expect(panel).not.toBeNull();
  return panel!;
}

function renderLayout() {
  return render(
    <AppProvider>
      <MainLayout />
    </AppProvider>,
  );
}

beforeEach(() => {
  (globalThis as Record<string, unknown>).ResizeObserver = MockResizeObserver;
  window.localStorage.removeItem(RAIL_KEY);
});

afterEach(() => {
  delete (globalThis as Record<string, unknown>).ResizeObserver;
});

describe('rail driven by a three-entry registry', () => {
  test('renders one labelled item per entry, in registry order', () => {
    renderLayout();
    const items = within(rail()).getAllByRole('tab');
    expect(items.map((t) => t.getAttribute('aria-label'))).toEqual(['Structure', 'Pages', 'Images']);
    expect(within(items[2]).getByText('Images')).toBeVisible();
  });

  test('renders one tabpanel per entry, each holding that entry\'s panel', () => {
    renderLayout();
    for (const [name, id] of [['Structure', 'structure'], ['Pages', 'pages'], ['Images', 'images']]) {
      const panel = panelFor(tab(name));
      expect(panel).toHaveAttribute('role', 'tabpanel');
      expect(panel).toHaveAttribute('aria-labelledby', tab(name).id);
      expect(within(panel).getByTestId(`stub-panel-${id}`)).toBeInTheDocument();
    }
  });

  test('Cmd/Ctrl+3 selects the third entry and shows its panel', () => {
    renderLayout();
    fireEvent.keyDown(window, { key: '3', metaKey: true, ctrlKey: true });
    expect(tab('Images')).toHaveAttribute('aria-selected', 'true');
    expect(tab('Images')).toHaveFocus();
    expect(panelFor(tab('Images'))).not.toHaveClass('invisible');
    expect(panelFor(tab('Structure'))).toHaveClass('invisible');
  });

  test('a digit past the third entry does nothing', () => {
    renderLayout();
    fireEvent.keyDown(window, { key: '4', metaKey: true, ctrlKey: true });
    expect(tab('Structure')).toHaveAttribute('aria-selected', 'true');
  });

  test('End reaches the third entry and Down wraps from it to the first', async () => {
    const user = userEvent.setup();
    renderLayout();
    tab('Structure').focus();
    await user.keyboard('{End}');
    expect(tab('Images')).toHaveFocus();
    expect(tab('Images')).toHaveAttribute('aria-selected', 'true');
    await user.keyboard('{ArrowDown}');
    expect(tab('Structure')).toHaveFocus();
    await user.keyboard('{ArrowUp}');
    expect(tab('Images')).toHaveFocus();
  });

  test('clicking the active third entry collapses, like any other entry', async () => {
    const user = userEvent.setup();
    renderLayout();
    await user.click(tab('Images'));
    expect(tab('Images')).toHaveAttribute('aria-expanded', 'true');
    await user.click(tab('Images'));
    expect(tab('Images')).toHaveAttribute('aria-expanded', 'false');
  });

  test('a persisted third view id is restored', () => {
    window.localStorage.setItem(RAIL_KEY, JSON.stringify({ view: 'images', collapsed: false }));
    renderLayout();
    expect(tab('Images')).toHaveAttribute('aria-selected', 'true');
  });
});
