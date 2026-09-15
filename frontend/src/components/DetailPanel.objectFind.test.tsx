/**
 * Object-tab find.
 *
 * Cmd+F find extends from Plain Text to the Object tab, scoped to the active
 * tab and searching the rendered text of the parsed object (property keys +
 * values across Dict/Array/Scalar) via the `object-find-*` testids.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.objectFind.test.tsx
 */
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import { AppProvider, useAppDispatch, type AppAction } from '../hooks/useDocumentState';
import { DetailPanel } from './DetailPanel';

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

const mockGetObjectDetail = vi.fn();
const mockGetXRefTable = vi.fn();
const mockGetPlainText = vi.fn();
const mockGetPlainTextSize = vi.fn();
const mockGetReverseRefs = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: vi.fn(),
    CloseDocument: vi.fn(),
    OpenFileDialog: vi.fn(),
    GetObjectDetail: (...args: unknown[]) => mockGetObjectDetail(...args),
    GetContentStream: vi.fn(),
    GetImageData: vi.fn(),
    DescribeImage: vi.fn().mockResolvedValue({ width: 0, height: 0, colorSpace: '', estimatedBytes: 0 }),
    SaveImageToFile: vi.fn().mockResolvedValue(''),
    GetReverseRefs: (...args: unknown[]) => mockGetReverseRefs(...args),
    GetFontView: vi.fn(),
    GetXRefTable: (...args: unknown[]) => mockGetXRefTable(...args),
    GetPlainText: (...args: unknown[]) => mockGetPlainText(...args),
    GetPlainTextSize: (...args: unknown[]) => mockGetPlainTextSize(...args),
    GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
    GetSignatures: vi.fn().mockResolvedValue([]),
    GetEmbeddedFileBytes: vi.fn().mockResolvedValue(''),
    GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '', warning: '' }),
    SaveBytesToFile: vi.fn().mockResolvedValue(''),
    DiffDocuments: vi.fn().mockResolvedValue({ root: null, summary: {} }),
  }),
);

const catalogNode = {
  id: 'root', label: 'Catalog', rawKey: '', nodeType: 'dict', valueType: '',
  hasChildren: true, childCount: 1, iconHint: 'catalog', error: '',
};

// A dict whose rendered text carries "objectonly-needle" twice (once in a
// value, once inside a longer value) so navigation across two matches works.
const dictDetail = {
  nodeId: 'obj:0:3',
  objectRef: '3 0 R',
  type: 'dict',
  properties: [
    { key: '/Type', value: { type: 'name', display: '/Page', raw: '/Page', refTarget: '' } },
    { key: '/Marker', value: { type: 'string', display: 'objectonly-needle', raw: 'objectonly-needle', refTarget: '' } },
    { key: '/Note', value: { type: 'string', display: 'second objectonly-needle tail', raw: '', refTarget: '' } },
  ],
  elements: [],
  scalarValue: null,
  streamInfo: null,
};

// A dict whose two values differ only in case, so a case-insensitive query
// matches both while a case-sensitive query matches only the exact-case one.
const caseDictDetail = {
  nodeId: 'obj:0:3',
  objectRef: '3 0 R',
  type: 'dict',
  properties: [
    { key: '/Upper', value: { type: 'string', display: 'CaseNeedle', raw: '', refTarget: '' } },
    { key: '/Lower', value: { type: 'string', display: 'caseneedle', raw: '', refTarget: '' } },
  ],
  elements: [],
  scalarValue: null,
  streamInfo: null,
};

// An array whose elements carry the token twice so array-element matching and
// navigation are exercised.
const arrayDetail = {
  nodeId: 'obj:0:4',
  objectRef: '4 0 R',
  type: 'array',
  properties: null,
  elements: [
    { type: 'name', display: '/ArrayNeedle', raw: '', refTarget: '' },
    { type: 'string', display: 'trailing ArrayNeedle here', raw: '', refTarget: '' },
  ],
  scalarValue: null,
  streamInfo: null,
};

// A scalar object; a scalar holds exactly one value, so at most one match.
const scalarDetail = {
  nodeId: 'obj:0:5',
  objectRef: '5 0 R',
  type: 'scalar',
  properties: null,
  elements: null,
  scalarValue: { type: 'string', display: 'scalarNeedle value', raw: '', refTarget: '' },
  streamInfo: null,
};

const openAction: AppAction = {
  type: 'OPEN_DOCUMENT',
  payload: { tabId: 'tab-1', fileName: 'test.pdf', filePath: '/test.pdf', rootNode: catalogNode, rootChildren: [] },
};
const selectAction: AppAction = { type: 'SELECT_NODE', payload: { nodeId: 'obj:0:3' } };

function Dispatch({ action }: { action: AppAction }) {
  useAppDispatch()(action);
  return null;
}

// Selects a different tree node on click, so a test can drive an in-tab
// navigation to another object after opening find.
function NodeSelector() {
  const dispatch = useAppDispatch();
  return (
    <button
      data-testid="select-other-node"
      onClick={() => dispatch({ type: 'SELECT_NODE', payload: { nodeId: 'obj:0:4' } })}
    >
      select other
    </button>
  );
}

function forceMacPlatform() {
  const original = Object.getOwnPropertyDescriptor(window.navigator, 'platform');
  Object.defineProperty(window.navigator, 'platform', { configurable: true, get: () => 'MacIntel' });
  return () => { if (original) Object.defineProperty(window.navigator, 'platform', original); };
}

function cmdF() {
  act(() => {
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', metaKey: true, bubbles: true, cancelable: true }));
  });
}

function pressF3(shift = false) {
  act(() => {
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'F3', shiftKey: shift, bubbles: true, cancelable: true }));
  });
}

function renderPanel() {
  return render(
    <AppProvider>
      <Dispatch action={openAction} />
      <Dispatch action={selectAction} />
      <NodeSelector />
      <DetailPanel />
    </AppProvider>,
  );
}

function objectMarkCount() {
  return (
    screen.queryAllByTestId('object-find-match').length +
    screen.queryAllByTestId('object-find-active-match').length
  );
}

let restore: () => void;
beforeEach(() => {
  vi.clearAllMocks();
  mockGetObjectDetail.mockResolvedValue(dictDetail);
  mockGetReverseRefs.mockResolvedValue([]);
  mockGetXRefTable.mockResolvedValue({ tabId: 'tab-1', entries: [] });
  mockGetPlainText.mockResolvedValue({ tabId: 'tab-1', content: 'x\n', totalBytes: 2 });
  mockGetPlainTextSize.mockResolvedValue(2);
  restore = forceMacPlatform();
});
afterEach(() => restore());

describe('Cmd+F opens find in the Object tab', () => {
  test('with the Object tab active and detail loaded, Cmd+F mounts object-find-bar', async () => {
    renderPanel();
    await waitFor(() => {
      expect(screen.getByTestId('detail-pane-object').getAttribute('data-state')).toBe('active');
      expect(screen.getByText('/Marker')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('object-find-bar')).toBeNull();
    cmdF();
    expect(screen.getByTestId('object-find-bar')).toBeInTheDocument();
  });
});

describe('Object find matches keys and values', () => {
  test('typing a token that appears in two property values yields two marks, one active', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'objectonly-needle' } });
    await waitFor(() => {
      const active = screen.queryAllByTestId('object-find-active-match');
      const inactive = screen.queryAllByTestId('object-find-match');
      expect(active.length + inactive.length).toBeGreaterThanOrEqual(2);
      expect(active.length).toBe(1);
    });
    expect(screen.getByTestId('object-find-count').textContent).toBe('1 of 2');
  });

  test('a property key is matchable', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: '/Marker' } });
    await waitFor(() => {
      const total =
        screen.queryAllByTestId('object-find-match').length +
        screen.queryAllByTestId('object-find-active-match').length;
      expect(total).toBeGreaterThanOrEqual(1);
    });
  });
});

describe('Object find navigation', () => {
  test('Next advances the active match and updates the count', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'objectonly-needle' } });
    await waitFor(() => expect(screen.getByTestId('object-find-count').textContent).toBe('1 of 2'));
    fireEvent.click(screen.getByTestId('object-find-next'));
    expect(screen.getByTestId('object-find-count').textContent).toBe('2 of 2');
  });

  test('Next scrolls the active match into view', async () => {
    const scrollSpy = vi.fn();
    const original = Element.prototype.scrollIntoView;
    Element.prototype.scrollIntoView = scrollSpy;
    try {
      renderPanel();
      await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
      cmdF();
      fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'objectonly-needle' } });
      await waitFor(() => expect(screen.getByTestId('object-find-count').textContent).toBe('1 of 2'));
      fireEvent.click(screen.getByTestId('object-find-next'));
      expect(scrollSpy).toHaveBeenCalled();
    } finally {
      Element.prototype.scrollIntoView = original;
    }
  });
});

describe('Object find no-match state', () => {
  test('a query absent from the object shows "0 of 0" and renders no marks', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'zzz-not-here' } });
    expect(screen.getByTestId('object-find-count').textContent).toBe('0 of 0');
    expect(screen.queryAllByTestId('object-find-match')).toHaveLength(0);
    expect(screen.queryAllByTestId('object-find-active-match')).toHaveLength(0);
  });
});

describe('Object find case sensitivity toggle', () => {
  test('toggling Aa drops a case-only duplicate match', async () => {
    mockGetObjectDetail.mockResolvedValue(caseDictDetail);
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Upper')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'caseneedle' } });
    await waitFor(() => expect(screen.getByTestId('object-find-count').textContent).toBe('1 of 2'));

    fireEvent.click(screen.getByTestId('object-find-case-toggle'));
    await waitFor(() => expect(screen.getByTestId('object-find-count').textContent).toBe('1 of 1'));
    expect(screen.getByTestId('object-find-case-toggle').getAttribute('aria-pressed')).toBe('true');
  });
});

describe('Object find previous navigation', () => {
  test('Prev wraps backward from the first match to the last', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'objectonly-needle' } });
    await waitFor(() => expect(screen.getByTestId('object-find-count').textContent).toBe('1 of 2'));
    fireEvent.click(screen.getByTestId('object-find-prev'));
    expect(screen.getByTestId('object-find-count').textContent).toBe('2 of 2');
  });

  test('Shift+F3 navigates to the previous match', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'objectonly-needle' } });
    await waitFor(() => expect(screen.getByTestId('object-find-count').textContent).toBe('1 of 2'));
    pressF3(true);
    expect(screen.getByTestId('object-find-count').textContent).toBe('2 of 2');
  });
});

describe('Object find matches array elements', () => {
  test('a token in two array elements yields two marks, one active', async () => {
    mockGetObjectDetail.mockResolvedValue(arrayDetail);
    renderPanel();
    await waitFor(() => expect(screen.getByText('/ArrayNeedle')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'ArrayNeedle' } });
    await waitFor(() => expect(screen.getByTestId('object-find-count').textContent).toBe('1 of 2'));
    expect(screen.queryAllByTestId('object-find-active-match')).toHaveLength(1);
    expect(
      screen.queryAllByTestId('object-find-match').length +
        screen.queryAllByTestId('object-find-active-match').length,
    ).toBe(2);
  });
});

describe('Object find matches a scalar value', () => {
  test('a token in the scalar value yields a single match', async () => {
    mockGetObjectDetail.mockResolvedValue(scalarDetail);
    renderPanel();
    await waitFor(() => expect(screen.getByText('scalarNeedle value')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'scalarNeedle' } });
    await waitFor(() => expect(screen.getByTestId('object-find-count').textContent).toBe('1 of 1'));
    expect(screen.queryAllByTestId('object-find-active-match')).toHaveLength(1);
  });
});

describe('Object find issues no backend call', () => {
  test('opening, typing, navigating and toggling case make no pdfservice call', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    // Discard the load-phase calls; find must add none.
    mockGetObjectDetail.mockClear();
    mockGetXRefTable.mockClear();
    mockGetPlainText.mockClear();
    mockGetPlainTextSize.mockClear();
    mockGetReverseRefs.mockClear();

    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'objectonly-needle' } });
    await waitFor(() => expect(screen.getByTestId('object-find-count').textContent).toBe('1 of 2'));
    fireEvent.click(screen.getByTestId('object-find-next'));
    fireEvent.click(screen.getByTestId('object-find-prev'));
    fireEvent.click(screen.getByTestId('object-find-case-toggle'));

    expect(mockGetObjectDetail).not.toHaveBeenCalled();
    expect(mockGetXRefTable).not.toHaveBeenCalled();
    expect(mockGetPlainText).not.toHaveBeenCalled();
    expect(mockGetPlainTextSize).not.toHaveBeenCalled();
    expect(mockGetReverseRefs).not.toHaveBeenCalled();
  });
});

describe('closing find clears the search', () => {
  test('closing the bar removes the highlights and the bar', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'objectonly-needle' } });
    await waitFor(() => expect(objectMarkCount()).toBe(2));
    fireEvent.click(screen.getByTestId('object-find-close'));
    await waitFor(() => {
      expect(screen.queryByTestId('object-find-bar')).toBeNull();
      expect(objectMarkCount()).toBe(0);
    });
  });

  test('closing the bar restores focus to the object content', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'objectonly-needle' } });
    await waitFor(() => expect(objectMarkCount()).toBe(2));
    fireEvent.click(screen.getByTestId('object-find-close'));
    await waitFor(() => expect(screen.queryByTestId('object-find-bar')).toBeNull());
    expect(document.activeElement).toBe(screen.getByTestId('detail-panel-content'));
  });

  test('closing after navigating past the first match does not scroll the viewport', async () => {
    const scrollSpy = vi.fn();
    const original = Element.prototype.scrollIntoView;
    Element.prototype.scrollIntoView = scrollSpy;
    try {
      renderPanel();
      await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
      cmdF();
      fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'objectonly-needle' } });
      await waitFor(() => expect(screen.getByTestId('object-find-count').textContent).toBe('1 of 2'));
      fireEvent.click(screen.getByTestId('object-find-next'));
      await waitFor(() => expect(screen.getByTestId('object-find-count').textContent).toBe('2 of 2'));
      // Closing must not move the active match back to #0 under the still-live
      // (deferred) match list, which would scroll the pane for one frame.
      scrollSpy.mockClear();
      fireEvent.click(screen.getByTestId('object-find-close'));
      await waitFor(() => expect(screen.queryByTestId('object-find-bar')).toBeNull());
      expect(scrollSpy).not.toHaveBeenCalled();
    } finally {
      Element.prototype.scrollIntoView = original;
    }
  });
});

describe('selecting a different object starts find fresh', () => {
  test('navigating to another object clears the previous search and closes the bar', async () => {
    renderPanel();
    await waitFor(() => expect(screen.getByText('/Marker')).toBeInTheDocument());
    cmdF();
    fireEvent.change(screen.getByTestId('object-find-input'), { target: { value: 'objectonly-needle' } });
    await waitFor(() => expect(objectMarkCount()).toBe(2));
    fireEvent.click(screen.getByTestId('select-other-node'));
    await waitFor(() => {
      expect(screen.queryByTestId('object-find-bar')).toBeNull();
      expect(objectMarkCount()).toBe(0);
    });
  });
});
