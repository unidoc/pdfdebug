/**
 * 2.4-UNIT-003 [P3]: File name displayed / tree content after open.
 *
 * Tests that MainLayout renders the root Catalog node and its children
 * when document state is populated.
 */
import { render, screen, act } from '@testing-library/react';
import { describe, test, expect, vi } from 'vitest';
import {
  AppProvider,
  useAppDispatch,
  type AppAction,
} from '../hooks/useDocumentState';
import { MainLayout } from './MainLayout';

// Mock allotment -- it requires browser layout APIs not available in jsdom
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

const catalogNode = {
  id: 'root',
  label: 'Catalog',
  rawKey: '',
  nodeType: 'dict',
  valueType: '',
  hasChildren: true,
  childCount: 3,
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
  {
    id: 'obj:0:2',
    label: 'Pages',
    rawKey: '/Pages',
    nodeType: 'dict',
    valueType: 'reference',
    hasChildren: true,
    childCount: 2,
    iconHint: 'page',
    error: '',
  },
];

function DispatchThenLayout({ action }: { action: AppAction }) {
  const dispatch = useAppDispatch();
  return (
    <div>
      <button data-testid="dispatch" onClick={() => dispatch(action)} />
      <MainLayout />
    </div>
  );
}

describe('2.4-UNIT-003: MainLayout tree content', () => {
  test('shows placeholder when no document is open', () => {
    render(
      <AppProvider>
        <MainLayout />
      </AppProvider>
    );
    expect(screen.getByText('Tree Panel')).toBeInTheDocument();
  });

  test('shows Catalog root and children after OPEN_DOCUMENT', () => {
    const action: AppAction = {
      type: 'OPEN_DOCUMENT',
      payload: {
        tabId: 'tab-1',
        fileName: 'test.pdf',
        rootNode: catalogNode,
        rootChildren: childNodes,
      },
    };

    render(
      <AppProvider>
        <DispatchThenLayout action={action} />
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    expect(screen.getByText('Catalog')).toBeInTheDocument();
    expect(screen.getByText('Type')).toBeInTheDocument();
    expect(screen.getByText('Pages')).toBeInTheDocument();
  });

  test('shows expand indicator for nodes with children', () => {
    const action: AppAction = {
      type: 'OPEN_DOCUMENT',
      payload: {
        tabId: 'tab-1',
        fileName: 'test.pdf',
        rootNode: catalogNode,
        rootChildren: childNodes,
      },
    };

    render(
      <AppProvider>
        <DispatchThenLayout action={action} />
      </AppProvider>
    );

    act(() => screen.getByTestId('dispatch').click());

    // Catalog (hasChildren) and Pages (hasChildren) should show "v" indicator
    const indicators = screen.getAllByText('v');
    expect(indicators.length).toBeGreaterThanOrEqual(2);
  });
});
