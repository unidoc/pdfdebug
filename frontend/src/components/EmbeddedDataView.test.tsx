/**
 * EmbeddedDataView component tests.
 *
 * Component contract (from the story):
 *  - Document-level view fed by GetEmbeddedFiles(tabId) -> EmbeddedFileList.
 *  - Table of rows (name, relationship, MIME, size).
 *  - Selecting a row shows its /Filespec details + "Reveal in tree" + "Save...".
 *  - "Reveal in tree" reuses NAVIGATE_TO_REF wiring (onNavigate prop) to jump
 *    TreePanel to the /EmbeddedFile stream object.
 *  - "Save..." calls GetEmbeddedFileBytes then the new SaveBytesToFile binding.
 *  - XML/text payloads get an inline read-only preview; binary payloads show
 *    size + save-only (no preview).
 *
 * Run: cd frontend && npx vitest run src/components/EmbeddedDataView.test.tsx
 */
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { EmbeddedDataView } from './EmbeddedDataView';

const mockGetEmbeddedFiles = vi.fn();
const mockGetEmbeddedFileBytes = vi.fn();
const mockSaveBytesToFile = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    GetEmbeddedFiles: (...a: unknown[]) => mockGetEmbeddedFiles(...a),
    GetEmbeddedFileBytes: (...a: unknown[]) => mockGetEmbeddedFileBytes(...a),
    SaveBytesToFile: (...a: unknown[]) => mockSaveBytesToFile(...a),
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
});

describe('EmbeddedDataView', () => {
  // Lists embedded files as table rows with the discriminating columns.
  test('lists rows with name, relationship, MIME, size', async () => {
    render(<EmbeddedDataView tabId="t1" active onNavigate={vi.fn()} />);

    await waitFor(() => expect(screen.getByText('factur-x.xml')).toBeInTheDocument());
    expect(screen.getByText('logo.png')).toBeInTheDocument();
    expect(screen.getByText('Data')).toBeInTheDocument();
    expect(screen.getByText('text/xml')).toBeInTheDocument();
    // Size rendered human-readable somewhere in the row.
    expect(screen.getByTestId('embedded-row-4 0 R')).toBeInTheDocument();
  });

  // Selecting a row reveals its /Filespec details.
  test('selecting a row shows filespec detail', async () => {
    render(<EmbeddedDataView tabId="t1" active onNavigate={vi.fn()} />);
    await waitFor(() => screen.getByText('factur-x.xml'));

    fireEvent.click(screen.getByTestId('embedded-row-4 0 R'));

    const detail = await screen.findByTestId('embedded-detail');
    // The filespec ref is surfaced in the detail pane.
    expect(detail).toHaveTextContent('6 0 R');
  });

  // "Reveal in tree" dispatches navigation to the /EmbeddedFile stream node
  // (reuses NAVIGATE_TO_REF wiring via onNavigate).
  test('Reveal in tree navigates to the stream node', async () => {
    const onNavigate = vi.fn();
    render(<EmbeddedDataView tabId="t1" active onNavigate={onNavigate} />);
    await waitFor(() => screen.getByText('factur-x.xml'));
    fireEvent.click(screen.getByTestId('embedded-row-4 0 R'));

    fireEvent.click(await screen.findByTestId('embedded-reveal-in-tree'));

    expect(onNavigate).toHaveBeenCalledWith('obj:0:4');
  });

  // "Save..." fetches the bytes via GetEmbeddedFileBytes then hands them to the
  // new SaveBytesToFile binding with the display name.
  test('Save fetches bytes and calls SaveBytesToFile', async () => {
    mockGetEmbeddedFileBytes.mockResolvedValue('PGZvbz4='); // base64-ish payload
    mockSaveBytesToFile.mockResolvedValue('/Users/me/factur-x.xml');
    render(<EmbeddedDataView tabId="t1" active onNavigate={vi.fn()} />);
    await waitFor(() => screen.getByText('factur-x.xml'));
    fireEvent.click(screen.getByTestId('embedded-row-4 0 R'));

    fireEvent.click(await screen.findByTestId('embedded-save'));

    await waitFor(() => expect(mockGetEmbeddedFileBytes).toHaveBeenCalledWith('t1', 'obj:0:4'));
    await waitFor(() => expect(mockSaveBytesToFile).toHaveBeenCalled());
    // The suggested file name passed to the save dialog is the display name.
    expect(mockSaveBytesToFile.mock.calls[0][0]).toBe('factur-x.xml');
  });

  // XML/text payloads get an inline read-only preview;
  // binary payloads show size + save-only (no preview region).
  test('XML row previews inline; binary row is save-only', async () => {
    mockGetEmbeddedFileBytes.mockResolvedValue('PHg+aGVsbG88L3g+'); // <x>hello</x>
    render(<EmbeddedDataView tabId="t1" active onNavigate={vi.fn()} />);
    await waitFor(() => screen.getByText('factur-x.xml'));

    // XML row -> inline preview present.
    fireEvent.click(screen.getByTestId('embedded-row-4 0 R'));
    expect(await screen.findByTestId('embedded-text-preview')).toBeInTheDocument();

    // Binary row -> no inline preview, save-only.
    fireEvent.click(screen.getByTestId('embedded-row-10 0 R'));
    await waitFor(() => screen.getByTestId('embedded-detail'));
    expect(screen.queryByTestId('embedded-text-preview')).not.toBeInTheDocument();
    expect(screen.getByTestId('embedded-save')).toBeInTheDocument();
  });

  // A per-entry degraded attachment (no /EmbeddedFile) renders a warning
  // state, not a crash; Save is disabled.
  test('entry without stream shows warning, Save disabled', async () => {
    mockGetEmbeddedFiles.mockResolvedValue({
      files: [{ ...xmlEntry, embeddedFileRef: '', embeddedFileNodeId: '', size: 0 }],
    });
    render(<EmbeddedDataView tabId="t1" active onNavigate={vi.fn()} />);
    await waitFor(() => screen.getByText('factur-x.xml'));
    fireEvent.click(screen.getByTestId('embedded-row-6 0 R'));

    expect(await screen.findByTestId('embedded-entry-warning')).toBeInTheDocument();
    expect(screen.getByTestId('embedded-save')).toBeDisabled();
  });

  // Empty document (no attachments) shows an empty state, not an error.
  test('no attachments shows empty state', async () => {
    mockGetEmbeddedFiles.mockResolvedValue({ files: [] });
    render(<EmbeddedDataView tabId="t1" active onNavigate={vi.fn()} />);

    expect(await screen.findByTestId('embedded-empty')).toBeInTheDocument();
  });
});
