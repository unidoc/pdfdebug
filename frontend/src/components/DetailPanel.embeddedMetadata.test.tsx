/**
 * Story 13.2: DetailPanel Embedded + Metadata tab integration tests (AC 6, 7).
 *
 * TDD RED PHASE: `describe.skip` keeps CI green until Task 5 extends the
 * `DetailView` union with 'embedded' | 'metadata', adds the reset-on-activeTabId
 * effect entries, and renders the two new Tabs.Trigger/Tabs.Content pairs
 * (taking the tab bar from 3 to 5 tabs). Unskip during the green phase.
 *
 * AC6: an "Embedded" document-level tab beside Object/XREF/Plain Text, with an
 *      optional "(N)" count mirroring the XREF tab.
 * AC7: a dedicated "Metadata" tab beside Embedded (NOT crammed into Embedded).
 *
 * The four new bound methods (GetEmbeddedFiles, GetEmbeddedFileBytes,
 * GetDocumentMetadata, SaveBytesToFile) are added to the binding mock here per
 * AC9 so DetailPanel's new tabs do not widen the App.test.tsx vi.mock gap.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.embeddedMetadata.test.tsx
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

const mockGetEmbeddedFiles = vi.fn();
const mockGetEmbeddedFileBytes = vi.fn();
const mockGetDocumentMetadata = vi.fn();
const mockSaveBytesToFile = vi.fn();
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
    // The four NEW Story 13.2 bound methods (AC9).
    GetEmbeddedFiles: (...a: unknown[]) => mockGetEmbeddedFiles(...a),
    GetSignatures: vi.fn().mockResolvedValue([]),
    GetEmbeddedFileBytes: (...a: unknown[]) => mockGetEmbeddedFileBytes(...a),
    GetDocumentMetadata: (...a: unknown[]) => mockGetDocumentMetadata(...a),
    SaveBytesToFile: (...a: unknown[]) => mockSaveBytesToFile(...a),
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
    fileName: 'invoice.pdf',
    filePath: '/path/to/invoice.pdf',
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

beforeEach(() => {
  vi.clearAllMocks();
  mockGetEmbeddedFiles.mockResolvedValue({
    files: [
      {
        name: 'factur-x.xml',
        filespecRef: '6 0 R',
        embeddedFileRef: '4 0 R',
        embeddedFileNodeId: 'obj:0:4',
        afRelationship: 'Data',
        subtype: 'text/xml',
        size: 42,
      },
    ],
  });
  mockGetDocumentMetadata.mockResolvedValue({
    info: { Title: 'Invoice 2024-001' },
    xmp: '<x:xmpmeta>marker</x:xmpmeta>',
    warning: '',
  });
});

describe('DetailPanel Embedded + Metadata tabs (Story 13.2)', () => {
  // 13.2-UNIT-100 [P0] AC6: the Embedded tab trigger is present in the tab bar.
  test('13.2-UNIT-100 Embedded tab trigger present', async () => {
    renderPanel([openAction]);
    expect(await screen.findByTestId('detail-tab-embedded')).toBeInTheDocument();
  });

  // 13.2-UNIT-110 [P0] AC7: the Metadata tab trigger is present beside Embedded.
  // Base tab bar is now 6 triggers after Story 13.5 added the Validate tab:
  // object/xref/plaintext/embedded/metadata/validate.
  test('13.2-UNIT-110 Metadata tab trigger present (6 tabs total)', async () => {
    renderPanel([openAction]);
    await screen.findByTestId('detail-tab-embedded');
    expect(screen.getByTestId('detail-tab-metadata')).toBeInTheDocument();

    const list = screen.getByTestId('detail-tab-list');
    expect(list.querySelectorAll('[role="tab"]').length).toBe(6);
  });

  // 13.2-UNIT-115 [P0] AC6: activating Embedded renders the embedded view fed by
  // GetEmbeddedFiles.
  test('13.2-UNIT-115 activating Embedded renders the list', async () => {
    renderPanel([openAction]);
    fireEvent.click(await screen.findByTestId('detail-tab-embedded'));

    await waitFor(() => expect(mockGetEmbeddedFiles).toHaveBeenCalledWith('tab-1'));
    expect(await screen.findByText('factur-x.xml')).toBeInTheDocument();
  });

  // 13.2-UNIT-116 [P0] AC7: activating Metadata renders the metadata view fed by
  // GetDocumentMetadata.
  test('13.2-UNIT-116 activating Metadata renders Info + XMP', async () => {
    renderPanel([openAction]);
    fireEvent.click(await screen.findByTestId('detail-tab-metadata'));

    await waitFor(() => expect(mockGetDocumentMetadata).toHaveBeenCalledWith('tab-1'));
    expect(await screen.findByText('Invoice 2024-001')).toBeInTheDocument();
  });

  // 13.2-UNIT-117 [P1] AC6: the Embedded tab label shows the optional "(N)"
  // count mirroring the XREF tab.
  test('13.2-UNIT-117 Embedded tab shows the (N) count', async () => {
    renderPanel([openAction]);
    const tab = await screen.findByTestId('detail-tab-embedded');
    await waitFor(() => expect(tab).toHaveTextContent('(1)'));
  });
});
