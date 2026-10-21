/**
 * SignaturesView component tests.
 *
 * Component contract:
 *  - Document-level view fed by GetSignatures(tabId) -> signature list.
 *  - One key/value card per signature (data-testid="signature-card"): signer,
 *    issuer, validity window, algorithms, SubFilter, signing time, ByteRange
 *    coverage facts.
 *  - An explicit non-verdict note (data-testid="signature-trust-note") and NO
 *    trust-claim language anywhere (a hard requirement).
 *  - An expired/not-yet-valid cue (data-testid="signature-expiry-cue") that is
 *    about the cert DATE only.
 *  - An expandable certificate chain (data-testid="signature-cert-chain").
 *  - "Reveal in tree" reuses the NAVIGATE_TO_REF wiring (onNavigate prop):
 *    signatureNodeId primary, fieldNodeId fallback for a direct /V, omitted
 *    when neither resolves.
 *
 * Run: cd frontend && npx vitest run src/components/SignaturesView.test.tsx
 */
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import { SignaturesView, signaturesIssue } from './SignaturesView';

const mockGetSignatures = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    GetSignatures: (...a: unknown[]) => mockGetSignatures(...a),
  })
);

/** A decomposed, indirect-/V signature entry per the signatures JSON contract. */
const signedEntry = {
  fieldName: 'Sig1',
  signed: true,
  signatureRef: '5 0 R',
  signatureNodeId: 'obj:0:5',
  fieldNodeId: 'obj:0:4',
  subFilter: 'adbe.pkcs7.detached',
  type: 'signature',
  signingTimeRaw: "D:20260101120000+00'00'",
  signingTime: '2026-01-01T12:00:00Z',
  name: 'ATDD Signer',
  reason: 'Acceptance testing',
  location: 'Testville',
  contactInfo: 'atdd@example.com',
  digestAlgorithm: 'SHA-256',
  signatureAlgorithm: 'RSA',
  signer: {
    subject: 'CN=ATDD Test Signer,O=UniDoc ATDD',
    issuer: 'CN=ATDD Test Root CA,O=UniDoc ATDD',
    serial: '2026',
    notBefore: '2025-01-01T00:00:00Z',
    notAfter: '2027-01-01T00:00:00Z',
  },
  signerIdentified: true,
  certificates: [
    {
      subject: 'CN=ATDD Test Root CA,O=UniDoc ATDD',
      issuer: 'CN=ATDD Test Root CA,O=UniDoc ATDD',
      serial: '77001',
      notBefore: '2025-01-01T00:00:00Z',
      notAfter: '2027-01-01T00:00:00Z',
    },
    {
      subject: 'CN=ATDD Test Signer,O=UniDoc ATDD',
      issuer: 'CN=ATDD Test Root CA,O=UniDoc ATDD',
      serial: '2026',
      notBefore: '2025-01-01T00:00:00Z',
      notAfter: '2027-01-01T00:00:00Z',
    },
  ],
  notes: ['trust not verified - structural decomposition only'],
  decomposeError: '',
  byteRange: [0, 1234, 7382, 900],
  coversWholeFile: true,
  trailingGap: 0,
  holeMatchesContents: true,
  coverageError: '',
};

/** Same signature but with a cert validity window that ended in the past. */
const expiredEntry = {
  ...signedEntry,
  fieldName: 'SigExpired',
  signer: {
    ...signedEntry.signer,
    notBefore: '2022-01-01T00:00:00Z',
    notAfter: '2024-01-01T00:00:00Z',
  },
};

/** Direct-/V signature: no signatureNodeId, reveal falls back to the field. */
const directEntry = {
  ...signedEntry,
  fieldName: 'SigDirect',
  signatureRef: '',
  signatureNodeId: '',
  fieldNodeId: 'obj:0:4',
};

beforeEach(() => {
  // Freeze ONLY the clock (not setTimeout/setInterval) so the cert-expiry cue
  // is asserted against a fixed "now" and the 2027-01-01 "valid" fixtures do
  // not start expiring in the real future. toFake:['Date'] leaves real polling
  // timers intact, so RTL waitFor and promise flushing behave normally.
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(new Date('2026-01-01T00:00:00Z'));
  vi.clearAllMocks();
  mockGetSignatures.mockResolvedValue([signedEntry]);
});

afterEach(() => {
  vi.useRealTimers();
});

describe('SignaturesView', () => {
  // Renders a key/value card per signature with the decomposed facts.
  test('renders signature card with decomposed facts', async () => {
    render(<SignaturesView tabId="tab-1" active onNavigate={vi.fn()} />);

    await waitFor(() => expect(screen.getByTestId('signature-card')).toBeInTheDocument());
    const card = screen.getByTestId('signature-card');
    for (const fact of [
      'Sig1',
      'ATDD Test Signer',
      'ATDD Test Root CA',
      'adbe.pkcs7.detached',
      'SHA-256',
      'RSA',
      '2025-01-01',
      '2027-01-01',
    ]) {
      expect(card.textContent).toContain(fact);
    }
  });

  // A CN holding an escaped comma (RFC 4514 "\,") shows whole and unescaped,
  // and a "CN=" inside another escaped value is not taken as the CN.
  test('signer and issuer CN keep escaped commas', async () => {
    mockGetSignatures.mockResolvedValue([
      {
        ...signedEntry,
        signer: {
          ...signedEntry.signer,
          subject: 'CN=DocuSign\\, Inc.,OU=Technical Operations,O=DocuSign\\, Inc.',
          issuer: 'O=Fake\\,CN=Spoof,CN=Entrust Class 3 Client CA - SHA256',
        },
      },
    ]);
    render(<SignaturesView tabId="tab-1" active onNavigate={vi.fn()} />);

    await waitFor(() => expect(screen.getByTestId('signature-card')).toBeInTheDocument());
    const text = screen.getByTestId('signature-card').textContent ?? '';
    expect(text).toContain('DocuSign, Inc.');
    expect(text).not.toContain('DocuSign\\');
    expect(text).toContain('Entrust Class 3 Client CA - SHA256');
    expect(text).not.toContain('Spoof');
  });

  // Expanded chain DNs drop RFC 4514 escapes; a value holding a comma or a
  // plus is quoted, and multi-valued RDN parts join with " + ".
  test('chain shows readable DNs with quoted comma values', async () => {
    const subject = 'CN=DocuSign\\, Inc.,OU=Technical Operations,C=US';
    const issuer =
      'CN=Entrust Class 3 Client CA - SHA256,OU=See www.entrust.net/legal-terms+OU=(c) 2015 Entrust\\, Inc. - for authorized use only,O=Entrust\\, Inc.,C=US';
    mockGetSignatures.mockResolvedValue([
      {
        ...signedEntry,
        certificates: [{ ...signedEntry.certificates[0], subject, issuer }],
      },
    ]);
    render(<SignaturesView tabId="tab-1" active onNavigate={vi.fn()} />);

    await waitFor(() => screen.getByTestId('signature-cert-chain'));
    fireEvent.click(screen.getByTestId('signature-cert-chain'));
    const list = await screen.findByTestId('signature-cert-chain-list');
    expect(list.textContent).toContain('CN="DocuSign, Inc.", OU=Technical Operations, C=US');
    expect(list.textContent).toContain(
      'issued by CN=Entrust Class 3 Client CA - SHA256, OU=See www.entrust.net/legal-terms + OU="(c) 2015 Entrust, Inc. - for authorized use only", O="Entrust, Inc.", C=US'
    );
    expect(list.textContent).not.toContain('\\');
  });

  // The explicit non-verdict note renders, and no trust-claim language
  // appears (valid/trusted/verified allowed only in negated/factual forms).
  test('shows trust note and never claims validity', async () => {
    render(<SignaturesView tabId="tab-1" active onNavigate={vi.fn()} />);

    await waitFor(() => expect(screen.getByTestId('signature-trust-note')).toBeInTheDocument());
    expect(screen.getByTestId('signature-trust-note').textContent?.toLowerCase()).toContain(
      'trust not verified'
    );
    const text = (screen.getByTestId('signature-card').textContent ?? '').toLowerCase();
    for (const m of text.matchAll(/valid|trusted|verified/g)) {
      const pos = m.index ?? 0;
      const before = text.slice(Math.max(0, pos - 4), pos);
      const allowed =
        before.endsWith('not ') ||
        before.endsWith('un') ||
        before.endsWith('in') ||
        text.slice(pos).startsWith('validity');
      expect(allowed, `trust-claim language at "${text.slice(pos - 10, pos + 15)}"`).toBe(true);
    }
  });

  // An expired signer cert shows a DATE-only visual cue -- about the cert
  // date, never a trust verdict.
  test('expired cert shows date-only cue', async () => {
    mockGetSignatures.mockResolvedValue([expiredEntry]);
    render(<SignaturesView tabId="tab-1" active onNavigate={vi.fn()} />);

    await waitFor(() => expect(screen.getByTestId('signature-expiry-cue')).toBeInTheDocument());
    const cue = screen.getByTestId('signature-expiry-cue').textContent?.toLowerCase() ?? '';
    expect(cue).toContain('expired');
    expect(cue).toContain('2024-01-01');
    // Date fact only: the cue must not escalate into a validity verdict.
    expect(cue).not.toContain('invalid');
    expect(cue).not.toContain('not valid');
  });

  // The certificate chain is expandable -- collapsed by default, expanding
  // reveals every embedded cert.
  test('certificate chain expands on demand', async () => {
    render(<SignaturesView tabId="tab-1" active onNavigate={vi.fn()} />);
    await waitFor(() => screen.getByTestId('signature-card'));

    expect(screen.queryByText('CN=ATDD Test Root CA,O=UniDoc ATDD')).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId('signature-cert-chain'));
    await waitFor(() =>
      expect(screen.getByText('CN=ATDD Test Root CA, O=UniDoc ATDD')).toBeInTheDocument()
    );
  });

  // "Reveal in tree" navigates to the /V dict node for an indirect ref and
  // falls back to the field node for a direct /V.
  test('reveal-in-tree targets /V node with field fallback', async () => {
    const onNavigate = vi.fn();
    const { unmount } = render(<SignaturesView tabId="tab-1" active onNavigate={onNavigate} />);
    await waitFor(() => screen.getByTestId('signature-card'));

    fireEvent.click(screen.getByTestId('signature-reveal-in-tree'));
    expect(onNavigate).toHaveBeenCalledWith('obj:0:5');
    unmount();

    mockGetSignatures.mockResolvedValue([directEntry]);
    const onNavigate2 = vi.fn();
    render(<SignaturesView tabId="tab-1" active onNavigate={onNavigate2} />);
    await waitFor(() => screen.getByTestId('signature-card'));

    fireEvent.click(screen.getByTestId('signature-reveal-in-tree'));
    expect(onNavigate2).toHaveBeenCalledWith('obj:0:4');
  });

  // An unsigned placeholder field renders as a card marked unsigned, with
  // no signer facts and no error.
  test('unsigned placeholder renders without decomposition', async () => {
    mockGetSignatures.mockResolvedValue([
      {
        fieldName: 'EmptySig',
        signed: false,
        signatureRef: '',
        signatureNodeId: '',
        fieldNodeId: 'obj:0:4',
        subFilter: '',
        notes: [],
        certificates: [],
        decomposeError: '',
      },
    ]);
    render(<SignaturesView tabId="tab-1" active onNavigate={vi.fn()} />);

    await waitFor(() => expect(screen.getByTestId('signature-card')).toBeInTheDocument());
    const card = screen.getByTestId('signature-card');
    expect(card.textContent).toContain('EmptySig');
    expect(card.textContent?.toLowerCase()).toContain('unsigned');
  });

  // A signature whose CMS could not be decomposed shows a red callout at the
  // top of the card and no separate gray error row.
  test('decompose error shows the problem callout above the facts', async () => {
    mockGetSignatures.mockResolvedValue([
      { ...signedEntry, signer: null, certificates: [], decomposeError: 'asn1: syntax error: indefinite length found' },
    ]);
    render(<SignaturesView tabId="tab-1" active onNavigate={vi.fn()} />);

    const callout = await screen.findByTestId('signature-problem');
    expect(callout.textContent).toBe(
      'Could not decompose this signature: asn1: syntax error: indefinite length found'
    );
    expect(callout.className).toContain('text-error');
    expect(callout.className).toContain('border-error');
    const card = screen.getByTestId('signature-card');
    const dl = card.querySelector('dl');
    expect(dl).not.toBeNull();
    expect(callout.compareDocumentPosition(dl!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(card.textContent).not.toContain('Decompose error');
    expect(screen.queryByTestId('signature-decompose-error')).not.toBeInTheDocument();
    expect(card.textContent?.split('indefinite length found').length).toBe(2);
  });

  // A clean signature has no problem callout.
  test('clean signature has no problem callout', async () => {
    render(<SignaturesView tabId="tab-1" active onNavigate={vi.fn()} />);
    await waitFor(() => screen.getByTestId('signature-card'));
    expect(screen.queryByTestId('signature-problem')).not.toBeInTheDocument();
  });

  // A ByteRange coverage error shows in the callout and the coverage rows
  // are not rendered.
  test('coverage error shows the problem callout', async () => {
    mockGetSignatures.mockResolvedValue([
      { ...signedEntry, byteRange: undefined, coverageError: 'ByteRange has 3 elements, want 4' },
    ]);
    render(<SignaturesView tabId="tab-1" active onNavigate={vi.fn()} />);

    const callout = await screen.findByTestId('signature-problem');
    expect(callout.textContent).toBe('Could not check ByteRange coverage: ByteRange has 3 elements, want 4');
    expect(screen.queryByTestId('signature-coverage-error')).not.toBeInTheDocument();
    expect(screen.queryByTestId('signature-coverage')).not.toBeInTheDocument();
    expect(screen.getByTestId('signature-card').textContent).not.toContain('Coverage error');
  });

  // Both errors on one entry show as two lines in one callout.
  test('decompose and coverage errors share one callout', async () => {
    mockGetSignatures.mockResolvedValue([
      { ...signedEntry, decomposeError: 'bad CMS', coverageError: 'bad ByteRange' },
    ]);
    render(<SignaturesView tabId="tab-1" active onNavigate={vi.fn()} />);

    const callouts = await screen.findAllByTestId('signature-problem');
    expect(callouts).toHaveLength(1);
    expect(callouts[0].textContent).toContain('Could not decompose this signature: bad CMS');
    expect(callouts[0].textContent).toContain('Could not check ByteRange coverage: bad ByteRange');
  });

  // A parent-supplied fetch error renders the error state instead of cards.
  test('fetchError prop renders the error state', () => {
    render(
      <SignaturesView tabId="tab-1" active onNavigate={vi.fn()} data={null} fetchError="parse failed: bad xref" />
    );
    expect(screen.getByTestId('signatures-error').textContent).toBe('parse failed: bad xref');
    expect(mockGetSignatures).not.toHaveBeenCalled();
  });
});

describe('signaturesIssue', () => {
  test('error tone for a decompose error', () => {
    expect(signaturesIssue([{ ...signedEntry, decomposeError: 'bad CMS' }])).toEqual({
      tone: 'error',
      title: '1 signature could not be decomposed',
    });
  });

  test('error tone for a coverage error', () => {
    expect(signaturesIssue([{ ...signedEntry, coverageError: 'bad' }, { ...signedEntry, coverageError: 'bad' }])).toEqual({
      tone: 'error',
      title: '2 signatures could not be checked for ByteRange coverage',
    });
  });

  test('error outranks trailing bytes', () => {
    const issue = signaturesIssue([
      { ...signedEntry, cmsTrailingBytes: { offset: 10, count: 4 } },
      { ...signedEntry, decomposeError: 'bad CMS' },
    ]);
    expect(issue?.tone).toBe('error');
  });

  test('warning tone for trailing bytes only', () => {
    expect(signaturesIssue([{ ...signedEntry, cmsTrailingBytes: { offset: 10, count: 4 } }])).toEqual({
      tone: 'warning',
      title: '1 signature has non-zero bytes after the CMS',
    });
  });

  test('null for a clean list', () => {
    expect(signaturesIssue([signedEntry, { ...signedEntry, cmsTrailingBytes: null }])).toBeNull();
    expect(signaturesIssue([])).toBeNull();
  });
});
