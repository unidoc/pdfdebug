/**
 * SignaturesView certificate extension rows.
 *
 * With the certificate chain expanded, each chain certificate shows five rows
 * under its subject, issuer and serial lines: Key usage, Ext key usage,
 * CRL DP, OCSP and CA Issuers (data-testids signature-cert-key-usage,
 * signature-cert-ext-key-usage, signature-cert-crl-dp, signature-cert-ocsp,
 * signature-cert-ca-issuers). A null or missing value reads "absent"; an
 * empty URL list reads "present, no URI"; an empty key usage list reads
 * "present, no bits set". URLs are plain text, not links.
 *
 * Run: cd frontend && npx vitest run src/components/SignaturesView.certExtensions.test.tsx
 */
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { SignaturesView } from './SignaturesView';

const mockGetSignatures = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    GetSignatures: (...a: unknown[]) => mockGetSignatures(...a),
  })
);

const leaf = {
  subject: 'CN=Fixture Signer,O=UniDoc Fixtures',
  issuer: 'CN=Fixture Root CA,O=UniDoc Fixtures',
  serial: '2026',
  notBefore: '2025-01-01T00:00:00Z',
  notAfter: '2099-01-01T00:00:00Z',
  keyUsage: ['digitalSignature', 'nonRepudiation'],
  extKeyUsage: [
    { oid: '1.3.6.1.5.5.7.3.4', name: 'emailProtection' },
    { oid: '1.2.840.113583.1.1.5', name: 'adobeAuthenticDocumentsTrust' },
    { oid: '1.2.3.4.5', name: '' },
  ],
  crlDistributionPoints: ['http://crl.example.test/fixture-ca.crl'],
  ocspServers: ['http://ocsp.example.test'],
  caIssuers: ['http://ca.example.test/fixture-ca.cer'],
};

const root = {
  subject: 'CN=Fixture Root CA,O=UniDoc Fixtures',
  issuer: 'CN=Fixture Root CA,O=UniDoc Fixtures',
  serial: '77001',
  notBefore: '2025-01-01T00:00:00Z',
  notAfter: '2099-01-01T00:00:00Z',
  keyUsage: ['keyCertSign'],
  extKeyUsage: null,
  crlDistributionPoints: null,
  ocspServers: null,
  caIssuers: null,
};

function entryWith(certificates: Record<string, unknown>[]) {
  return {
    fieldName: 'Sig1',
    signed: true,
    signatureRef: '5 0 R',
    signatureNodeId: 'obj:0:5',
    fieldNodeId: 'obj:0:4',
    subFilter: 'adbe.pkcs7.detached',
    type: 'signature',
    digestAlgorithm: 'SHA-256',
    signatureAlgorithm: 'RSA',
    signer: leaf,
    signerIdentified: true,
    certificates,
    notes: ['trust not verified - structural decomposition only'],
    decomposeError: '',
    byteRange: [0, 501, 6647, 287],
    coversWholeFile: true,
    trailingGap: 0,
    holeMatchesContents: true,
    coverageError: '',
    holeCheckError: '',
    cmsEncoding: 'DER',
  };
}

/** Renders one signature with the given chain, expands it, and returns the chain items. */
async function renderChain(certificates: Record<string, unknown>[]) {
  mockGetSignatures.mockResolvedValue([entryWith(certificates)]);
  render(<SignaturesView tabId="tab-1" active onNavigate={vi.fn()} />);
  await waitFor(() => screen.getByTestId('signature-cert-chain'));
  fireEvent.click(screen.getByTestId('signature-cert-chain'));
  const list = await screen.findByTestId('signature-cert-chain-list');
  return within(list).getAllByRole('listitem');
}

const ROWS = [
  ['signature-cert-key-usage', 'Key usage'],
  ['signature-cert-ext-key-usage', 'Ext key usage'],
  ['signature-cert-crl-dp', 'CRL DP'],
  ['signature-cert-ocsp', 'OCSP'],
  ['signature-cert-ca-issuers', 'CA Issuers'],
] as const;

/** Returns the value text of one extension row and checks its label. */
function rowText(item: HTMLElement, testid: string, label: string): string {
  const value = within(item).getByTestId(testid);
  expect(value.previousElementSibling?.textContent).toBe(label);
  return value.textContent ?? '';
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('SignaturesView certificate extension rows', () => {
  test('a certificate with every extension shows the five values', async () => {
    const [item] = await renderChain([leaf]);
    expect(rowText(item, 'signature-cert-key-usage', 'Key usage')).toBe('digitalSignature, nonRepudiation');
    expect(rowText(item, 'signature-cert-ext-key-usage', 'Ext key usage')).toBe(
      'emailProtection (1.3.6.1.5.5.7.3.4), adobeAuthenticDocumentsTrust (1.2.840.113583.1.1.5), 1.2.3.4.5'
    );
    expect(rowText(item, 'signature-cert-crl-dp', 'CRL DP')).toBe('http://crl.example.test/fixture-ca.crl');
    expect(rowText(item, 'signature-cert-ocsp', 'OCSP')).toBe('http://ocsp.example.test');
    expect(rowText(item, 'signature-cert-ca-issuers', 'CA Issuers')).toBe('http://ca.example.test/fixture-ca.cer');
  });

  test('rows sit under each certificate of the chain', async () => {
    const items = await renderChain([root, leaf]);
    expect(items).toHaveLength(2);
    expect(rowText(items[0], 'signature-cert-key-usage', 'Key usage')).toBe('keyCertSign');
    expect(rowText(items[1], 'signature-cert-key-usage', 'Key usage')).toBe('digitalSignature, nonRepudiation');
    for (const item of items) {
      for (const [testid] of ROWS) {
        expect(within(item).getAllByTestId(testid)).toHaveLength(1);
      }
    }
  });

  test('null values read absent', async () => {
    const [item] = await renderChain([{ ...root, keyUsage: null }]);
    for (const [testid, label] of ROWS) {
      expect(rowText(item, testid, label)).toBe('absent');
    }
  });

  test('a payload without the extension keys reads absent', async () => {
    const [item] = await renderChain([
      {
        subject: root.subject,
        issuer: root.issuer,
        serial: root.serial,
        notBefore: root.notBefore,
        notAfter: root.notAfter,
      },
    ]);
    for (const [testid, label] of ROWS) {
      expect(rowText(item, testid, label)).toBe('absent');
    }
  });

  test('present but empty lists read present, no URI and present, no bits set', async () => {
    const [item] = await renderChain([
      { ...root, keyUsage: [], extKeyUsage: [], crlDistributionPoints: [], ocspServers: ['http://ocsp.example.test'], caIssuers: [] },
    ]);
    expect(rowText(item, 'signature-cert-key-usage', 'Key usage')).toBe('present, no bits set');
    expect(rowText(item, 'signature-cert-crl-dp', 'CRL DP')).toBe('present, no URI');
    expect(rowText(item, 'signature-cert-ca-issuers', 'CA Issuers')).toBe('present, no URI');
    expect(rowText(item, 'signature-cert-ocsp', 'OCSP')).toBe('http://ocsp.example.test');
  });

  test('several URLs join with a comma and are not links', async () => {
    const [item] = await renderChain([
      { ...leaf, crlDistributionPoints: ['http://crl.example.test/a.crl', 'ldap://dir.example.test/cn=CA'] },
    ]);
    expect(rowText(item, 'signature-cert-crl-dp', 'CRL DP')).toBe(
      'http://crl.example.test/a.crl, ldap://dir.example.test/cn=CA'
    );
    expect(within(item).queryAllByRole('link')).toHaveLength(0);
  });
});
