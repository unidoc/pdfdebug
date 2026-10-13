/**
 * Image thumbnail, consent-to-load and unconditional save -- DetailPanel
 * integration tests.
 *
 * These pin the wiring that is invisible to a presentational component test:
 * that an expensive image is described (geometry + estimated decoded bytes)
 * before it is decoded, that proceeding always issues the decode, that saving
 * routes to the backend-direct method rather than round-tripping bytes, and
 * that a ceiling refusal is branched on the outcome discriminator.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.imageConsent.test.tsx
 */
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
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
const mockGetContentStream = vi.fn();
const mockGetImageData = vi.fn();
const mockDescribeImage = vi.fn();
const mockSaveImageToFile = vi.fn();
const mockSaveBytesToFile = vi.fn();
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
    GetContentStream: (...a: unknown[]) => mockGetContentStream(...a),
    GetImageData: (...a: unknown[]) => mockGetImageData(...a),
    DescribeImage: (...a: unknown[]) => mockDescribeImage(...a),
    SaveImageToFile: (...a: unknown[]) => mockSaveImageToFile(...a),
    GetReverseRefs: (...a: unknown[]) => mockGetReverseRefs(...a),
    GetFontView: vi.fn().mockResolvedValue({ kind: 'neither', detail: null, roster: null }),
    GetXRefTable: vi.fn().mockResolvedValue({ tabId: '', entries: [] }),
    GetEmbeddedFiles: vi.fn().mockResolvedValue({ files: [] }),
    GetSignatures: vi.fn().mockResolvedValue([]),
    GetEmbeddedFileBytes: vi.fn().mockResolvedValue(''),
    GetDocumentMetadata: vi.fn().mockResolvedValue({ info: {}, xmp: '', warning: '' }),
    SaveBytesToFile: (...a: unknown[]) => mockSaveBytesToFile(...a),
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
  streamInfo: { filter: 'FlateDecode', length: 12345 },
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

// ---------------------------------------------------------------------------
// An estimate above the warning threshold is announced with its size and a
// proceed control INSTEAD of decoding, and proceeding then issues the decode.
// ---------------------------------------------------------------------------

describe('warn before an expensive decode, then load on proceed', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(imageStreamDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    // 8192x8192 RGB -> ~192 MB decoded, above the 64 MB warning threshold.
    mockDescribeImage.mockResolvedValue({
      width: 8192,
      height: 8192,
      colorSpace: 'DeviceRGB',
      estimatedBytes: 8192 * 8192 * 3,
    });
    mockGetImageData.mockResolvedValue({
      nodeId: 'obj:0:7',
      kind: 'ok',
      mimeType: 'image/png',
      base64: 'AAAA',
      width: 8192,
      height: 8192,
      thumbWidth: 2048,
      thumbHeight: 2048,
    });
  });

  test('shows the estimate and a proceed control, and defers the decode', async () => {
    renderImageNode();

    const proceed = await screen.findByTestId('image-preview-proceed');
    expect(proceed).toBeInTheDocument();

    const region = screen.getByTestId('image-preview-consent');
    expect(region.textContent).toMatch(/8192/);
    expect(region.textContent).toMatch(/about/i);
    expect(region.textContent).toMatch(/192\s*MB/i);

    // The decode has NOT run yet -- the estimate came from the decode-free path.
    expect(mockGetImageData).not.toHaveBeenCalled();

    fireEvent.click(proceed);

    await waitFor(() => {
      expect(mockGetImageData).toHaveBeenCalledWith('tab-1', 'obj:0:7');
    });
  });
});

// ---------------------------------------------------------------------------
// Every image offers "Save image...", and the save routes to the backend-direct
// method (tabID, nodeID) rather than round-tripping the full-resolution bytes
// through SaveBytesToFile.
// ---------------------------------------------------------------------------

describe('unconditional backend-direct save', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(imageStreamDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    // Small image: below the warning threshold, so it loads without a prompt.
    mockDescribeImage.mockResolvedValue({
      width: 320,
      height: 240,
      colorSpace: 'DeviceRGB',
      estimatedBytes: 320 * 240 * 3,
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
    });
    mockSaveImageToFile.mockResolvedValue('/tmp/out.png');
  });

  test('Save invokes SaveImageToFile and never SaveBytesToFile', async () => {
    renderImageNode();

    const save = await screen.findByTestId('image-preview-save');
    fireEvent.click(save);

    await waitFor(() => {
      expect(mockSaveImageToFile).toHaveBeenCalled();
    });
    expect(mockSaveImageToFile.mock.calls[0][0]).toBe('tab-1');
    expect(mockSaveImageToFile.mock.calls[0][1]).toBe('obj:0:7');
    expect(mockSaveBytesToFile).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// A stream refused at the geometry ceiling is rendered as a finding, branched
// on the outcome discriminator, with no "load anyway" control.
// ---------------------------------------------------------------------------

describe('lying-stream refusal is a finding, not a consent prompt', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(imageStreamDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    // Declared small, so the pre-decode estimate is under the threshold and the
    // decode is attempted; it then inflates past the ceiling.
    mockDescribeImage.mockResolvedValue({
      width: 8,
      height: 8,
      colorSpace: 'DeviceGray',
      estimatedBytes: 64,
    });
    // The refusal happens below the dictionary read, so the backend carries the
    // full metadata set on this path.
    mockGetImageData.mockResolvedValue({
      nodeId: 'obj:0:7',
      kind: 'ceiling-refusal',
      base64: '',
      width: 8,
      height: 8,
      colorSpace: 'DeviceGray',
      bitsPerComponent: 8,
      filter: 'FlateDecode',
      decode: [1, 0],
      imageMask: false,
      smask: '5 0 R',
      adobeMarker: 'not-applicable',
      adobeTransform: null,
      sampleInterpretation: 'Inverted by /Decode',
      error: 'image data too large (the stream inflates past what it declares)',
    });
  });

  test('shows the finding and offers no proceed / load-anyway control', async () => {
    renderImageNode();

    expect(await screen.findByTestId('image-preview-finding')).toBeInTheDocument();
    expect(screen.queryByTestId('image-preview-proceed')).not.toBeInTheDocument();
  });

  // The dictionary was read, so the rows it produced are shown beside the
  // refusal, the same rows the CLI prints for the same object.
  test('still shows the metadata rows beside the refusal', async () => {
    renderImageNode();

    expect(await screen.findByTestId('image-preview-finding')).toBeInTheDocument();
    expect(screen.getByTestId('image-preview-interpretation')).toHaveTextContent(
      'Inverted by /Decode'
    );
    expect(screen.getByTestId('image-preview-decode')).toHaveTextContent('[1 0]');
    expect(screen.getByTestId('image-preview-smask')).toHaveTextContent('5 0 R');
    expect(screen.getByTestId('image-preview-metadata')).toHaveTextContent('DeviceGray');
    // The refusal message is the finding above, not a second error row.
    expect(screen.queryByTestId('image-preview-error')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// While a decode is in flight the indicator escalates past the threshold to a
// "taking longer than expected" message.
// ---------------------------------------------------------------------------

describe('loading indicator escalates', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    mockGetObjectDetail.mockResolvedValue(imageStreamDetail);
    mockGetReverseRefs.mockResolvedValue([]);
    mockDescribeImage.mockResolvedValue({
      width: 320,
      height: 240,
      colorSpace: 'DeviceRGB',
      estimatedBytes: 320 * 240 * 3,
    });
    // Never resolves: the decode stays in flight for the whole test. The bound
    // call is a Wails cancellable promise.
    mockGetImageData.mockImplementation(() => Object.assign(new Promise(() => {}), { cancel: vi.fn() }));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  test('shows "taking longer than expected" once the threshold passes', async () => {
    renderImageNode();
    await act(async () => {
      vi.advanceTimersByTime(6000);
    });
    expect(screen.getByText(/taking longer than expected/i)).toBeInTheDocument();
  });
});
