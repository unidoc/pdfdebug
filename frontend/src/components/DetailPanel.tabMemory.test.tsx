/**
 * The detail tab (Object, XREF, Plain Text, ...) is remembered per document
 * tab: switching back restores it, a document seen for the first time opens on
 * Object, a restored Signatures tab shows Object until the signature list
 * resolves, and Diff is not restored because its comparison is not kept.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.tabMemory.test.tsx
 */
import { render, screen, act, waitFor, fireEvent } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import type { Dispatch } from 'react';
import { AppProvider, useAppDispatch, type AppAction } from '../hooks/useDocumentState';
import { resetDiskStatusStore } from '../lib/diskStatusStore';
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

type EventHandler = (event: { data: Record<string, unknown> }) => void;
const handlers = vi.hoisted(() => ({} as Record<string, ((e: { data: Record<string, unknown> }) => void)[]>));

vi.mock('@wailsio/runtime', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@wailsio/runtime')>()),
  Events: {
    On: (name: string, handler: EventHandler) => {
      (handlers[name] ??= []).push(handler);
      return () => {
        const i = handlers[name]?.indexOf(handler) ?? -1;
        if (i >= 0) handlers[name].splice(i, 1);
      };
    },
    Emit: vi.fn(),
  },
}));

const b = vi.hoisted(() => ({
  OpenFile: vi.fn(),
  GetTreeRoot: vi.fn(),
  GetChildren: vi.fn(),
  CloseDocument: vi.fn(),
  GetSignatures: vi.fn(),
  GetDiskStatus: vi.fn(),
  DiffDocuments: vi.fn(),
  OpenFileDialog: vi.fn(),
}));

vi.mock('../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  OpenFile: (...a: unknown[]) => b.OpenFile(...a),
  GetTreeRoot: (...a: unknown[]) => b.GetTreeRoot(...a),
  GetChildren: (...a: unknown[]) => b.GetChildren(...a),
  CloseDocument: (...a: unknown[]) => b.CloseDocument(...a),
  GetSignatures: (...a: unknown[]) => b.GetSignatures(...a),
  GetDiskStatus: (...a: unknown[]) => b.GetDiskStatus(...a),
  DiffDocuments: (...a: unknown[]) => b.DiffDocuments(...a),
  OpenFileDialog: (...a: unknown[]) => b.OpenFileDialog(...a),
  GetObjectDetail: vi.fn().mockResolvedValue(null),
  GetContentStream: vi.fn(),
  GetImageData: vi.fn(),
  DescribeImage: vi.fn().mockResolvedValue({ width: 0, height: 0, colorSpace: '', estimatedBytes: 0 }),
  SaveImageToFile: vi.fn().mockResolvedValue(''),
  GetReverseRefs: vi.fn().mockResolvedValue([]),
  GetFontView: vi.fn(),
  GetXRefTable: vi.fn().mockResolvedValue({ tabId: 'x', entries: [] }),
  GetPlainText: vi.fn().mockResolvedValue({ tabId: 'x', content: '', totalBytes: 0 }),
  GetPlainTextSize: vi.fn().mockResolvedValue(0),
  GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
  GetEmbeddedFileBytes: vi.fn(),
  GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '' }),
  SaveBytesToFile: vi.fn(),
  Validate: vi.fn().mockResolvedValue({ problems: [], rules: [], scope: '' }),
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
function Probe() {
  dispatch = useAppDispatch();
  return null;
}

function mount() {
  render(
    <AppProvider>
      <Probe />
      <DetailPanel />
    </AppProvider>,
  );
}

let n = 0;
function ids() {
  n += 1;
  return { a: `a${n}`, b: `b${n}`, disk: `disk${n}`, pathA: `/docs/${n}/a.pdf`, pathB: `/docs/${n}/b.pdf` };
}

function open(tabId: string, filePath: string) {
  act(() =>
    dispatch({
      type: 'OPEN_DOCUMENT',
      payload: { tabId, fileName: filePath.split('/').pop() ?? '', filePath, pageCount: 1, rootNode: catalogNode, rootChildren: [] },
    }),
  );
}


const signature = {
  fieldName: 'Sig1',
  signed: true,
  signatureRef: '5 0 R',
  signatureNodeId: 'obj:0:5',
  fieldNodeId: 'obj:0:4',
  subFilter: 'adbe.pkcs7.detached',
  type: 'signature',
  signingTimeRaw: '',
  signingTime: '',
  name: '',
  reason: '',
  location: '',
  contactInfo: '',
  digestAlgorithm: 'SHA-256',
  signatureAlgorithm: 'RSA',
  signer: null,
  signerIdentified: false,
  certificates: [],
  notes: [],
  decomposeError: '',
  byteRange: [0, 10, 20, 30],
  coversWholeFile: true,
  trailingGap: 0,
  holeMatchesContents: true,
  coverageError: '',
};

function activate(tabId: string) {
  act(() => dispatch({ type: 'ACTIVATE_TAB', payload: { tabId } }));
}

function expectActive(view: string) {
  expect(screen.getByTestId(`detail-tab-${view}`)).toHaveAttribute('data-state', 'active');
}

beforeEach(() => {
  vi.clearAllMocks();
  resetDiskStatusStore();
  b.GetChildren.mockResolvedValue([]);
  b.GetSignatures.mockResolvedValue([]);
  b.GetDiskStatus.mockResolvedValue({ state: 'unchanged' });
  b.CloseDocument.mockResolvedValue(undefined);
});

describe('DetailPanel keeps the active tab per document', () => {
  test('switching documents restores each document\'s last tab', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    fireEvent.click(screen.getByTestId('detail-tab-xref'));
    expectActive('xref');

    open(t.b, t.pathB);
    expectActive('object');
    fireEvent.click(screen.getByTestId('detail-tab-metadata'));

    activate(t.a);
    expectActive('xref');
    activate(t.b);
    expectActive('metadata');
  });

  test('a document seen for the first time opens on Object', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    fireEvent.click(screen.getByTestId('detail-tab-validate'));
    open(t.b, t.pathB);
    expectActive('object');
  });

  test('a restored Signatures tab comes back once the list loads', async () => {
    const t = ids();
    b.GetSignatures.mockImplementation((tabId: string) =>
      Promise.resolve(tabId === t.a ? [signature] : []),
    );
    mount();
    open(t.a, t.pathA);
    fireEvent.click(await screen.findByTestId('detail-tab-signatures'));
    open(t.b, t.pathB);
    expectActive('object');

    activate(t.a);
    await waitFor(() => expectActive('signatures'));
  });

  test('while the signature list loads after a switch back, Object is shown', async () => {
    const t = ids();
    let pending: ((v: unknown) => void) | null = null;
    let callsA = 0;
    b.GetSignatures.mockImplementation((tabId: string) => {
      if (tabId !== t.a) return Promise.resolve([]);
      callsA += 1;
      if (callsA === 1) return Promise.resolve([signature]);
      return new Promise((resolve) => {
        pending = resolve;
      });
    });
    mount();
    open(t.a, t.pathA);
    fireEvent.click(await screen.findByTestId('detail-tab-signatures'));
    open(t.b, t.pathB);

    activate(t.a);
    await waitFor(() => expect(pending).not.toBeNull());
    expectActive('object');

    await act(async () => {
      pending?.([signature]);
    });
    await waitFor(() => expectActive('signatures'));
  });

  test('a document left on Diff comes back on Object', async () => {
    const t = ids();
    mount();
    open(t.a, t.pathA);
    fireEvent.click(screen.getByTestId('detail-tab-diff'));
    expectActive('diff');
    open(t.b, t.pathB);

    activate(t.a);
    expectActive('object');
  });

  test('a restored Signatures tab falls back to Object when the list is empty', async () => {
    const t = ids();
    let calls = 0;
    b.GetSignatures.mockImplementation((tabId: string) => {
      if (tabId !== t.a) return Promise.resolve([]);
      calls += 1;
      return Promise.resolve(calls === 1 ? [signature] : []);
    });
    mount();
    open(t.a, t.pathA);
    fireEvent.click(await screen.findByTestId('detail-tab-signatures'));
    open(t.b, t.pathB);

    activate(t.a);
    await waitFor(() => expect(calls).toBe(2));
    await waitFor(() => expectActive('object'));
    expect(screen.queryByTestId('detail-tab-signatures')).toBeNull();
  });
});
