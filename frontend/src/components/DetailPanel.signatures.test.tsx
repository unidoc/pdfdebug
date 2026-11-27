/**
 * DetailPanel Signatures tab visibility tests.
 *
 * Contract:
 *  - The "Signatures" document-level tab is shown ONLY when the document has
 *    >= 1 signature field; hidden otherwise (a deliberate departure from the
 *    always-visible tabs).
 *  - Visibility is driven by ONE GetSignatures fetch per document tab, made on
 *    mount and cached in state -- no refetch on tab switches.
 *  - The tab label carries an issue marker: error tone for a decompose or
 *    coverage error, warning tone for trailing CMS bytes only, none when clean.
 *  - A failed fetch shows the tab with an error marker and the message.
 *
 * The new bound method (GetSignatures) is stubbed in the binding mock here so
 * DetailPanel's new tab does not widen the App.test.tsx vi.mock gap (mirrors
 * the DetailPanel.embeddedMetadata.test.tsx playbook).
 *
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
    DescribeImage: vi.fn().mockResolvedValue({ width: 0, height: 0, colorSpace: '', estimatedBytes: 0 }),
    SaveImageToFile: vi.fn().mockResolvedValue(''),
    GetReverseRefs: vi.fn().mockResolvedValue([]),
    GetFontView: vi.fn(),
    GetXRefTable: vi.fn().mockResolvedValue({ tabId: 'tab-1', entries: [] }),
    GetPlainText: vi.fn().mockResolvedValue({ tabId: 'tab-1', content: '', totalBytes: 0 }),
    GetPlainTextSize: vi.fn().mockResolvedValue(0),
    GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
    GetEmbeddedFileBytes: vi.fn(),
    GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '' }),
    SaveEmbeddedFileToFile: vi.fn(),
    // The signatures bound method.
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

describe('DetailPanel Signatures tab', () => {
  // The tab is HIDDEN when the document has no signature fields.
  test('tab hidden when no signatures', async () => {
    mockGetSignatures.mockResolvedValue([]);
    renderPanel([openAction]);

    await waitFor(() => expect(mockGetSignatures).toHaveBeenCalledWith('tab-1'));
    expect(screen.queryByRole('tab', { name: /signatures/i })).not.toBeInTheDocument();
  });

  // The tab is SHOWN when the document has at least one signature field
  // (signed or unsigned placeholder).
  test('tab shown when signatures present', async () => {
    mockGetSignatures.mockResolvedValue(oneSignature);
    renderPanel([openAction]);

    await waitFor(() =>
      expect(screen.getByRole('tab', { name: /signatures/i })).toBeInTheDocument()
    );
  });

  // ONE GetSignatures fetch per document tab -- tab switches re-display the
  // cached result, never refetch.
  test('single fetch per document tab across tab switches', async () => {
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

  // An expanded certificate chain stays expanded after switching to another
  // detail tab and back.
  test('certificate chain stays expanded across tab switches', async () => {
    const cert = {
      subject: 'CN=Signer',
      issuer: 'CN=Root',
      serial: '1',
      notBefore: '2025-01-01T00:00:00Z',
      notAfter: '2027-01-01T00:00:00Z',
      keyUsage: null,
      extKeyUsage: null,
      crlDistributionPoints: null,
      ocspServers: null,
      caIssuers: null,
    };
    mockGetSignatures.mockResolvedValue([{ ...oneSignature[0], signer: cert, certificates: [cert] }]);
    renderPanel([openAction]);

    fireEvent.click(await screen.findByRole('tab', { name: /signatures/i }));
    fireEvent.click(screen.getByTestId('signature-cert-chain'));
    expect(screen.getByTestId('signature-cert-chain-list')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('tab', { name: /object/i }));
    fireEvent.click(screen.getByRole('tab', { name: /signatures/i }));
    expect(screen.getByTestId('signature-cert-chain')).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByTestId('signature-cert-chain-list')).toBeInTheDocument();
  });

  // A clean signature list puts no marker on the tab.
  test('no issue marker when signatures are clean', async () => {
    mockGetSignatures.mockResolvedValue(oneSignature);
    renderPanel([openAction]);

    await waitFor(() => screen.getByRole('tab', { name: /signatures/i }));
    expect(screen.queryByTestId('tab-issue-signatures')).not.toBeInTheDocument();
  });

  // A decompose error marks the tab in the error tone with an accessible label.
  test('error marker for a decompose error', async () => {
    mockGetSignatures.mockResolvedValue([{ ...oneSignature[0], decomposeError: 'asn1: indefinite length' }]);
    renderPanel([openAction]);

    const marker = await screen.findByTestId('tab-issue-signatures');
    expect(marker.getAttribute('data-tone')).toBe('error');
    expect(marker.className).toContain('bg-error');
    expect(marker.getAttribute('title')).toBe('1 signature could not be decomposed');
    expect(screen.getByRole('img', { name: '1 signature could not be decomposed' })).toBe(marker);
    expect(screen.getByTestId('detail-tab-signatures').contains(marker)).toBe(true);
  });

  // Trailing CMS bytes alone mark the tab in the warning tone.
  test('warning marker for trailing CMS bytes only', async () => {
    mockGetSignatures.mockResolvedValue([{ ...oneSignature[0], cmsTrailingBytes: { offset: 100, count: 8 } }]);
    renderPanel([openAction]);

    const marker = await screen.findByTestId('tab-issue-signatures');
    expect(marker.getAttribute('data-tone')).toBe('warning');
    expect(marker.className).toContain('bg-warning');
    expect(marker.getAttribute('title')).toBe('1 signature has non-zero bytes after the CMS');
  });

  // A rejected fetch shows the tab with an error marker and the message in
  // the tab body.
  test('fetch failure shows the tab with marker and error text', async () => {
    mockGetSignatures.mockRejectedValue(new Error('signatures: parse failed'));
    renderPanel([openAction]);

    const tab = await screen.findByRole('tab', { name: /signatures/i });
    const marker = screen.getByTestId('tab-issue-signatures');
    expect(marker.getAttribute('data-tone')).toBe('error');
    expect(marker.getAttribute('title')).toBe('Could not load signatures: signatures: parse failed');
    fireEvent.click(tab);
    expect(screen.getByTestId('signatures-error').textContent).toBe('signatures: parse failed');
    expect(mockGetSignatures).toHaveBeenCalledTimes(1);
  });
});
