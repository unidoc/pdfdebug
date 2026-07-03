/**
 * Story 13.4: DetailPanel Signatures tab visibility tests (AC 6, 8).
 *
 * Authored red-phase (`describe.skip`); unskipped in the Story 13.4 green
 * phase once the `DetailView` union gained 'signatures' and the CONDITIONAL
 * Tabs.Trigger/Tabs.Content pair landed.
 *
 * AC6 contract:
 *  - The "Signatures" document-level tab is shown ONLY when the document has
 *    >= 1 signature field; hidden otherwise (a deliberate departure from the
 *    always-visible tabs).
 *  - Visibility is driven by ONE GetSignatures fetch per document tab, made on
 *    mount and cached in state -- no refetch on tab switches.
 *
 * The new bound method (GetSignatures) is stubbed in the binding mock here so
 * DetailPanel's new tab does not widen the App.test.tsx vi.mock gap (mirrors
 * the 13-2 DetailPanel.embeddedMetadata.test.tsx playbook).
 *
 * Naming: 13.4-UNIT-11N [Px].
 * Run: cd frontend && npx vitest run src/components/DetailPanel.signatures.test.tsx
 */
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
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

const mockGetSignatures = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: vi.fn(),
    CloseDocument: vi.fn(),
    OpenFileDialog: vi.fn(),
    GetObjectDetail: vi.fn().mockResolvedValue(null),
    GetContentStream: vi.fn(),
    GetImageData: vi.fn(),
    GetReverseRefs: vi.fn().mockResolvedValue([]),
    GetFontView: vi.fn(),
    GetXRefTable: vi.fn().mockResolvedValue({ tabId: 'tab-1', entries: [] }),
    GetPlainText: vi.fn().mockResolvedValue({ tabId: 'tab-1', content: '', totalBytes: 0 }),
    GetPlainTextSize: vi.fn().mockResolvedValue(0),
    CancelPlainText: vi.fn(),
    GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
    GetEmbeddedFileBytes: vi.fn(),
    GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '' }),
    SaveBytesToFile: vi.fn(),
    // The NEW Story 13.4 bound method.
    GetSignatures: (...a: unknown[]) => mockGetSignatures(...a),
  })
);

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

const openAction: AppAction = {
  type: 'OPEN_DOCUMENT',
  payload: {
    tabId: 'tab-1',
    fileName: 'signed.pdf',
    filePath: '/path/to/signed.pdf',
    rootNode: catalogNode,
    rootChildren: [],
  },
};

function DispatchHelper({ action }: { action: AppAction }) {
  const dispatch = useAppDispatch();
  dispatch(action);
  return null;
}

function renderPanel(actions: AppAction[]) {
  return render(
    <AppProvider>
      {actions.map((a, i) => (
        <DispatchHelper key={i} action={a} />
      ))}
      <DetailPanel />
    </AppProvider>
  );
}

const oneSignature = [
  {
    fieldName: 'Sig1',
    signed: true,
    signatureRef: '5 0 R',
    signatureNodeId: 'obj:0:5',
    fieldNodeId: 'obj:0:4',
    subFilter: 'adbe.pkcs7.detached',
    notes: ['trust not verified - structural decomposition only'],
    certificates: [],
    decomposeError: '',
    coversWholeFile: true,
    trailingGap: 0,
    holeMatchesContents: true,
    coverageError: '',
  },
];

beforeEach(() => {
  vi.clearAllMocks();
});

describe('DetailPanel Signatures tab (Story 13.4)', () => {
  // 13.4-UNIT-110 [P0] AC6: the tab is HIDDEN when the document has no
  // signature fields.
  test('13.4-UNIT-110 tab hidden when no signatures', async () => {
    mockGetSignatures.mockResolvedValue([]);
    renderPanel([openAction]);

    await waitFor(() => expect(mockGetSignatures).toHaveBeenCalledWith('tab-1'));
    expect(screen.queryByRole('tab', { name: /signatures/i })).not.toBeInTheDocument();
  });

  // 13.4-UNIT-111 [P0] AC6: the tab is SHOWN when the document has at least
  // one signature field (signed or unsigned placeholder).
  test('13.4-UNIT-111 tab shown when signatures present', async () => {
    mockGetSignatures.mockResolvedValue(oneSignature);
    renderPanel([openAction]);

    await waitFor(() =>
      expect(screen.getByRole('tab', { name: /signatures/i })).toBeInTheDocument()
    );
  });

  // 13.4-UNIT-112 [P1] AC6: ONE GetSignatures fetch per document tab -- tab
  // switches re-display the cached result, never refetch.
  test('13.4-UNIT-112 single fetch per document tab across tab switches', async () => {
    mockGetSignatures.mockResolvedValue(oneSignature);
    renderPanel([openAction]);

    await waitFor(() =>
      expect(screen.getByRole('tab', { name: /signatures/i })).toBeInTheDocument()
    );
    fireEvent.click(screen.getByRole('tab', { name: /signatures/i }));
    fireEvent.click(screen.getByRole('tab', { name: /object/i }));
    fireEvent.click(screen.getByRole('tab', { name: /signatures/i }));
    expect(mockGetSignatures).toHaveBeenCalledTimes(1);
  });
});
