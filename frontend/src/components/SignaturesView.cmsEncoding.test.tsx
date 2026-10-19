/**
 * SignaturesView CMS encoding row.
 *
 * A signed entry with `cmsEncoding` "BER" or "DER" shows a "CMS encoding" row
 * (data-testid="signature-cms-encoding") right after "Signature algorithm";
 * an empty or missing value shows no row.
 *
 * Run: cd frontend && npx vitest run src/components/SignaturesView.cmsEncoding.test.tsx
 */
import { render, screen, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { SignaturesView } from './SignaturesView';

const mockGetSignatures = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    GetSignatures: (...a: unknown[]) => mockGetSignatures(...a),
  })
);

const signer = {
  subject: 'CN=Fixture Signer,O=UniDoc Fixtures',
  issuer: 'CN=Fixture Root CA,O=UniDoc Fixtures',
  serial: '2026',
  notBefore: '2025-01-01T00:00:00Z',
  notAfter: '2099-01-01T00:00:00Z',
};

/** A decomposed signature entry without a cmsEncoding key. */
const baseEntry = {
  fieldName: 'Sig1',
  signed: true,
  signatureRef: '5 0 R',
  signatureNodeId: 'obj:0:5',
  fieldNodeId: 'obj:0:4',
  subFilter: 'adbe.pkcs7.detached',
  type: 'signature',
  signingTimeRaw: "D:20260101120000+00'00'",
  signingTime: '2026-01-01T12:00:00Z',
  digestAlgorithm: 'SHA-256',
  signatureAlgorithm: 'RSA',
  signer,
  signerIdentified: true,
  certificates: [signer],
  notes: ['trust not verified - structural decomposition only'],
  decomposeError: '',
  byteRange: [0, 501, 6647, 287],
  coversWholeFile: true,
  trailingGap: 0,
  holeMatchesContents: true,
  coverageError: '',
  holeCheckError: '',
};

async function renderWith(entry: Record<string, unknown>) {
  mockGetSignatures.mockResolvedValue([entry]);
  render(<SignaturesView tabId="tab-1" active onNavigate={vi.fn()} />);
  await waitFor(() => expect(screen.getByTestId('signature-card')).toBeInTheDocument());
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('SignaturesView CMS encoding row', () => {
  test('BER payload shows the normalized-for-parsing text', async () => {
    await renderWith({ ...baseEntry, cmsEncoding: 'BER' });
    const row = screen.getByTestId('signature-cms-encoding');
    expect(row.textContent).toBe('BER (normalized to DER for parsing)');
    expect(row.previousElementSibling?.textContent).toBe('CMS encoding');
  });

  test('DER payload shows DER', async () => {
    await renderWith({ ...baseEntry, cmsEncoding: 'DER' });
    const row = screen.getByTestId('signature-cms-encoding');
    expect(row.textContent).toBe('DER');
    expect(row.previousElementSibling?.textContent).toBe('CMS encoding');
  });

  test('empty cmsEncoding shows no row', async () => {
    await renderWith({
      ...baseEntry,
      cmsEncoding: '',
      decomposeError: 'CMS parse failed: malformed end-of-contents at offset 4838',
      digestAlgorithm: '',
      signatureAlgorithm: '',
      signer: null,
      signerIdentified: false,
      certificates: [],
    });
    expect(screen.queryByTestId('signature-cms-encoding')).not.toBeInTheDocument();
    expect(screen.getByTestId('signature-card').textContent).not.toContain('CMS encoding');
  });

  test('missing cmsEncoding shows no row', async () => {
    await renderWith(baseEntry);
    expect(screen.queryByTestId('signature-cms-encoding')).not.toBeInTheDocument();
    expect(screen.getByTestId('signature-card').textContent).not.toContain('CMS encoding');
  });

  test('row sits right after the signature algorithm row', async () => {
    await renderWith({ ...baseEntry, cmsEncoding: 'BER' });
    const label = screen.getByTestId('signature-cms-encoding').previousElementSibling;
    const before = label?.previousElementSibling;
    const beforeLabel = before?.previousElementSibling;
    expect(before?.textContent).toBe('RSA');
    expect(beforeLabel?.textContent).toBe('Signature algorithm');
  });
});
