/**
 * Story 13.2: DocumentMetadataView component tests (AC 7).
 *
 * TDD RED PHASE: `describe.skip` keeps CI green until Task 5 lands
 * `DocumentMetadataView.tsx`. The component is lazy-imported inside beforeAll so
 * a missing module never breaks test collection. Unskip + convert to a
 * top-level import for the green phase.
 *
 * Component contract (from the story):
 *  - Fed by GetDocumentMetadata(tabId) -> { info: {...}, xmp: "...", warning }.
 *  - /Info fields rendered as a key/value block.
 *  - XMP packet rendered in a read-only, scrollable, bounded region.
 *  - XMP is rendered as PLAIN TEXT only -- NEVER injected as HTML.
 *
 * Run: cd frontend && npx vitest run src/components/DocumentMetadataView.test.tsx
 */
import { render, screen, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { DocumentMetadataView } from './DocumentMetadataView';

const mockGetDocumentMetadata = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    GetDocumentMetadata: (...a: unknown[]) => mockGetDocumentMetadata(...a),
  })
);

beforeEach(() => {
  vi.clearAllMocks();
});

describe('DocumentMetadataView', () => {
  // 13.2-UNIT-111 [P0] AC7: /Info fields render as a key/value block.
  test('renders Info key/value block', async () => {
    mockGetDocumentMetadata.mockResolvedValue({
      info: { Title: 'Invoice 2024-001', Author: 'ACME GmbH' },
      xmp: '<x:xmpmeta>ok</x:xmpmeta>',
      warning: '',
    });
    render(<DocumentMetadataView tabId="t1" active />);

    await waitFor(() => expect(screen.getByText('Invoice 2024-001')).toBeInTheDocument());
    expect(screen.getByText('ACME GmbH')).toBeInTheDocument();
    // The keys are surfaced too.
    expect(screen.getByText(/Title/)).toBeInTheDocument();
  });

  // 13.2-UNIT-112 [P0] AC7: the XMP packet renders in a read-only scrollable
  // region AS PLAIN TEXT -- the markup is shown as text, never parsed into DOM.
  test('XMP rendered as plain text, never HTML-injected', async () => {
    const xmp = '<x:xmpmeta><script>window.__pwned=1</script>marker</x:xmpmeta>';
    mockGetDocumentMetadata.mockResolvedValue({ info: {}, xmp, warning: '' });
    render(<DocumentMetadataView tabId="t1" active />);

    const region = await screen.findByTestId('metadata-xmp');
    // The packet text is present verbatim as text content...
    expect(region).toHaveTextContent('marker');
    expect(region).toHaveTextContent('<script>');
    // ...and the script tag was NOT parsed into a real element.
    expect(region.querySelector('script')).toBeNull();
    // No side effect from any injected script.
    expect((window as unknown as { __pwned?: number }).__pwned).toBeUndefined();
  });

  // 13.2-UNIT-113 [P1] AC3/AC7: missing metadata renders an empty state, not an
  // error.
  test('empty metadata shows empty state', async () => {
    mockGetDocumentMetadata.mockResolvedValue({ info: {}, xmp: '', warning: '' });
    render(<DocumentMetadataView tabId="t1" active />);

    expect(await screen.findByTestId('metadata-empty')).toBeInTheDocument();
  });

  // 13.2-UNIT-114 [P1] AC8: an undecodable-/Metadata warning is surfaced.
  test('surfaces the XMP decode warning', async () => {
    mockGetDocumentMetadata.mockResolvedValue({
      info: { Title: 'X' },
      xmp: '',
      warning: 'metadata stream could not be decoded',
    });
    render(<DocumentMetadataView tabId="t1" active />);

    expect(await screen.findByTestId('metadata-warning')).toBeInTheDocument();
  });
});
