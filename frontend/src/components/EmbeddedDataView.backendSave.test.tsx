/**
 * EmbeddedDataView save path: "Save..." hands the tab id, node id and display
 * name to the backend, which reads the bytes, shows the dialog and writes the
 * file. The bytes never come to the frontend for a save; the text preview
 * still fetches them for display.
 *
 * Run: cd frontend && npx vitest run src/components/EmbeddedDataView.backendSave.test.tsx
 */
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { EmbeddedDataView } from './EmbeddedDataView';

const mockGetEmbeddedFiles = vi.fn();
const mockGetEmbeddedFileBytes = vi.fn();
const mockSaveEmbeddedFileToFile = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    GetEmbeddedFiles: (...a: unknown[]) => mockGetEmbeddedFiles(...a),
    GetEmbeddedFileBytes: (...a: unknown[]) => mockGetEmbeddedFileBytes(...a),
    SaveEmbeddedFileToFile: (...a: unknown[]) => mockSaveEmbeddedFileToFile(...a),
  })
);

const xmlEntry = {
  name: 'factur-x.xml',
  filespecRef: '6 0 R',
  embeddedFileRef: '4 0 R',
  embeddedFileNodeId: 'obj:0:4',
  afRelationship: 'Data',
  subtype: 'text/xml',
  size: 42,
};
const binEntry = {
  name: 'logo.png',
  filespecRef: '12 0 R',
  embeddedFileRef: '10 0 R',
  embeddedFileNodeId: 'obj:0:10',
  afRelationship: 'Supplement',
  subtype: 'image/png',
  size: 2048,
};

beforeEach(() => {
  vi.clearAllMocks();
  mockGetEmbeddedFiles.mockResolvedValue({ files: [xmlEntry, binEntry] });
  mockGetEmbeddedFileBytes.mockResolvedValue('PHg+aGVsbG88L3g+');
});

async function selectRow(ref: string) {
  render(<EmbeddedDataView tabId="t1" active onNavigate={vi.fn()} />);
  await waitFor(() => screen.getByText('factur-x.xml'));
  fireEvent.click(screen.getByTestId(`embedded-row-${ref}`));
}

describe('EmbeddedDataView backend save', () => {
  test('Save passes tab id, node id and display name to SaveEmbeddedFileToFile', async () => {
    mockSaveEmbeddedFileToFile.mockResolvedValue('/Users/me/logo.png');
    await selectRow('10 0 R');

    fireEvent.click(await screen.findByTestId('embedded-save'));

    await waitFor(() => expect(mockSaveEmbeddedFileToFile).toHaveBeenCalledTimes(1));
    expect(mockSaveEmbeddedFileToFile).toHaveBeenCalledWith('t1', 'obj:0:10', 'logo.png');
  });

  test('saving a binary row never fetches the bytes in the frontend', async () => {
    mockSaveEmbeddedFileToFile.mockResolvedValue('/Users/me/logo.png');
    await selectRow('10 0 R');

    fireEvent.click(await screen.findByTestId('embedded-save'));

    await waitFor(() => expect(mockSaveEmbeddedFileToFile).toHaveBeenCalled());
    expect(mockGetEmbeddedFileBytes).not.toHaveBeenCalled();
  });

  test('saving a text row adds no byte fetch beyond the preview', async () => {
    mockSaveEmbeddedFileToFile.mockResolvedValue('/Users/me/factur-x.xml');
    await selectRow('4 0 R');
    await screen.findByTestId('embedded-text-preview');
    const previewFetches = mockGetEmbeddedFileBytes.mock.calls.length;

    fireEvent.click(screen.getByTestId('embedded-save'));

    await waitFor(() => expect(mockSaveEmbeddedFileToFile).toHaveBeenCalledWith('t1', 'obj:0:4', 'factur-x.xml'));
    expect(mockGetEmbeddedFileBytes).toHaveBeenCalledTimes(previewFetches);
  });

  test('a backend error, such as the extraction ceiling, lands in the save error slot', async () => {
    mockSaveEmbeddedFileToFile.mockRejectedValue(new Error('unsupported PDF version or feature: embedded file exceeds the 50 MB limit'));
    await selectRow('10 0 R');

    fireEvent.click(await screen.findByTestId('embedded-save'));

    const slot = await screen.findByTestId('embedded-save-error');
    expect(slot).toHaveTextContent('exceeds the 50 MB limit');
    expect(mockSaveEmbeddedFileToFile).toHaveBeenCalledWith('t1', 'obj:0:10', 'logo.png');
  });

  test('cancelling the dialog is a silent no-op', async () => {
    mockSaveEmbeddedFileToFile.mockResolvedValue('');
    await selectRow('10 0 R');

    fireEvent.click(await screen.findByTestId('embedded-save'));

    await waitFor(() => expect(mockSaveEmbeddedFileToFile).toHaveBeenCalled());
    expect(screen.queryByTestId('embedded-save-error')).not.toBeInTheDocument();
  });
});
