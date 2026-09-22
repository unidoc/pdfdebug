/**
 * Sample-interpretation fields reach the image panel -- DetailPanel wiring.
 *
 * The panel passes each image field by name, so a field the backend computes
 * and the panel does not forward is invisible to a presentational component
 * test. This pins the drill for the verdict and its evidence.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.imageSampleInterpretation.test.tsx
 */
import { render, screen } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import {
  AppProvider,
  useAppDispatch,
  type AppAction,
} from '../hooks/useDocumentState';
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
const mockGetImageData = vi.fn();
const mockDescribeImage = vi.fn();
const mockGetReverseRefs = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    OpenFile: vi.fn(),
    GetTreeRoot: vi.fn(),
    GetChildren: vi.fn(),
    CloseDocument: vi.fn(),
    OpenFileDialog: vi.fn(),
    GetObjectDetail: (...a: unknown[]) => mockGetObjectDetail(...a),
    GetContentStream: vi.fn(),
    GetImageData: (...a: unknown[]) => mockGetImageData(...a),
    DescribeImage: (...a: unknown[]) => mockDescribeImage(...a),
    SaveImageToFile: vi.fn(),
    GetReverseRefs: (...a: unknown[]) => mockGetReverseRefs(...a),
    GetFontView: vi.fn().mockResolvedValue({ kind: 'neither', detail: null, roster: null }),
    GetXRefTable: vi.fn().mockResolvedValue({ tabId: '', entries: [] }),
    GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
    GetSignatures: vi.fn().mockResolvedValue([]),
    GetEmbeddedFileBytes: vi.fn().mockResolvedValue(''),
    GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '', warning: '' }),
    SaveBytesToFile: vi.fn(),
  })
);

const imageTreeNode = {
  id: 'obj:0:7',
  label: 'Image',
  rawKey: '',
  nodeType: 'stream',
  valueType: '',
  hasChildren: false,
  childCount: 0,
  iconHint: 'image',
  error: '',
};

const imageStreamDetail = {
  nodeId: 'obj:0:7',
  objectRef: '7 0 R',
  type: 'stream',
  properties: [],
  elements: [],
  scalarValue: null,
  streamInfo: { filter: 'DCTDecode', length: 12345 },
};

function DispatchHelper({ action }: { action: AppAction }) {
  const dispatch = useAppDispatch();
  dispatch(action);
  return null;
}

const openAction: AppAction = {
  type: 'OPEN_DOCUMENT',
  payload: {
    tabId: 'tab-1',
    fileName: 'test.pdf',
    filePath: '/path/to/test.pdf',
    rootNode: imageTreeNode,
    rootChildren: [],
  },
};

function renderImageNode() {
  const selectAction: AppAction = {
    type: 'SELECT_NODE',
    payload: { nodeId: 'obj:0:7', iconHint: 'image' },
  };
  return render(
    <AppProvider>
      <DispatchHelper action={openAction} />
      <DispatchHelper action={selectAction} />
      <DetailPanel />
    </AppProvider>
  );
}

describe('the verdict and its evidence reach the panel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(imageStreamDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    // Small enough to load without a consent prompt.
    mockDescribeImage.mockResolvedValue({
      width: 320,
      height: 240,
      colorSpace: 'DeviceCMYK',
      estimatedBytes: 320 * 240 * 4,
    });
    mockGetImageData.mockResolvedValue({
      nodeId: 'obj:0:7',
      kind: 'ok',
      mimeType: 'image/png',
      base64: 'AAAA',
      width: 320,
      height: 240,
      thumbWidth: 320,
      thumbHeight: 240,
      colorSpace: 'DeviceCMYK',
      bitsPerComponent: 8,
      filter: 'DCTDecode',
      warning: '',
      error: '',
      decode: [1, 0, 1, 0, 1, 0, 1, 0],
      imageMask: false,
      smask: '12 0 R',
      adobeMarker: 'present',
      adobeTransform: 2,
      sampleInterpretation: 'Normal (net): /Decode inverts and Adobe APP14 inverts again',
    });
  });

  test('the verdict is rendered from the value the backend computed', async () => {
    renderImageNode();

    const verdict = await screen.findByTestId('image-preview-interpretation');
    expect(verdict).toHaveTextContent(
      'Normal (net): /Decode inverts and Adobe APP14 inverts again'
    );
  });

  test('the array, the transform and the soft mask reference are rendered beside it', async () => {
    renderImageNode();

    await screen.findByTestId('image-preview-interpretation');
    expect(screen.getByTestId('image-preview-decode').textContent).toMatch(
      /1\D+0\D+1\D+0\D+1\D+0\D+1\D+0/
    );
    expect(screen.getByTestId('image-preview-adobe-transform')).toHaveTextContent(
      'YCCK (transform 2)'
    );
    expect(screen.getByTestId('image-preview-smask')).toHaveTextContent('12 0 R');
  });
});
