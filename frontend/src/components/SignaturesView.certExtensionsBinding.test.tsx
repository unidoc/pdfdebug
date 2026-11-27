/**
 * SignaturesView certificate extension rows through the generated Wails
 * binding.
 *
 * Only the runtime call is stubbed: GetSignatures and the generated
 * SignatureField model run as built, so a binding that turned a null
 * extension list into [] would show "present, no URI" where the backend
 * said the extension is absent.
 *
 * Run: cd frontend && npx vitest run src/components/SignaturesView.certExtensionsBinding.test.tsx
 */
import { fireEvent, render, screen, within } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { SignaturesView } from './SignaturesView';

const mockCallByID = vi.fn();
vi.mock('@wailsio/runtime', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@wailsio/runtime')>();
  return {
    ...actual,
    Call: { ...actual.Call, ByID: (...a: unknown[]) => mockCallByID(...a) },
  };
});

const cert = {
  subject: 'CN=Fixture Root CA,O=UniDoc Fixtures',
  issuer: 'CN=Fixture Root CA,O=UniDoc Fixtures',
  serial: '77001',
  notBefore: '2025-01-01T00:00:00Z',
  notAfter: '2099-01-01T00:00:00Z',
};

/** The backend's JSON for one signed field whose chain is certs. */
function payload(certs: Record<string, unknown>[]) {
  return [
    {
      fieldName: 'Sig1',
      signed: true,
      signatureRef: '5 0 R',
      signatureNodeId: 'obj:0:5',
      fieldNodeId: 'obj:0:4',
      subFilter: 'adbe.pkcs7.detached',
      type: 'signature',
      signer: certs[0],
      signerIdentified: true,
      certificates: certs,
      notes: [],
      decomposeError: '',
      byteRange: [0, 501, 6647, 287],
      coversWholeFile: true,
      trailingGap: 0,
      holeMatchesContents: true,
      coverageError: '',
      holeCheckError: '',
      cmsEncoding: 'DER',
    },
  ];
}

async function renderChainThroughBinding(certs: Record<string, unknown>[]) {
  mockCallByID.mockResolvedValue(payload(certs));
  render(<SignaturesView tabId="tab-1" active onNavigate={vi.fn()} />);
  fireEvent.click(await screen.findByTestId('signature-cert-chain'));
  const list = await screen.findByTestId('signature-cert-chain-list');
  return within(list).getAllByRole('listitem');
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('SignaturesView certificate extensions through the Wails binding', () => {
  test('null extension lists still read absent', async () => {
    const [item] = await renderChainThroughBinding([
      { ...cert, keyUsage: null, extKeyUsage: null, crlDistributionPoints: null, ocspServers: null, caIssuers: null },
    ]);
    expect(mockCallByID).toHaveBeenCalledTimes(1);
    for (const testid of [
      'signature-cert-key-usage',
      'signature-cert-ext-key-usage',
      'signature-cert-crl-dp',
      'signature-cert-ocsp',
      'signature-cert-ca-issuers',
    ]) {
      expect(within(item).getByTestId(testid).textContent).toBe('absent');
    }
  });

  test('empty extension lists still read present', async () => {
    const [item] = await renderChainThroughBinding([
      { ...cert, keyUsage: [], extKeyUsage: [], crlDistributionPoints: [], ocspServers: [], caIssuers: [] },
    ]);
    expect(within(item).getByTestId('signature-cert-key-usage').textContent).toBe('present, no bits set');
    expect(within(item).getByTestId('signature-cert-ext-key-usage').textContent).toBe('present, no OID');
    expect(within(item).getByTestId('signature-cert-crl-dp').textContent).toBe('present, no URI');
    expect(within(item).getByTestId('signature-cert-ocsp').textContent).toBe('present, no URI');
    expect(within(item).getByTestId('signature-cert-ca-issuers').textContent).toBe('present, no URI');
  });
});
