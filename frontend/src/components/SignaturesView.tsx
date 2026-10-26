/**
 * @file Signatures view -- document-level decomposition of every digital
 * signature field. One key/value card per signature: signer,
 * issuer, validity window, algorithms, SubFilter, signing time, and the
 * ByteRange coverage facts, plus an expandable certificate chain and a
 * "Reveal in tree" jump. Structural decomposition ONLY: the view never makes
 * a trust claim -- the expiry cue is a certificate DATE fact and every card
 * carries the explicit "trust not verified" note.
 */
import { useEffect, useState } from 'react';
import { GetSignatures } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { extractErrorMessage } from '../lib/extractErrorMessage';
import { FILE_BUSY_MESSAGE, FILE_CHANGED_MESSAGE, isFileBusyMessage, isFileChangedMessage } from '../lib/fileChange';
import { SMALL_BUTTON } from './buttonStyles';
import { ProblemCallout, type ProblemTone } from './ProblemCallout';

/**
 * One decomposed X.509 certificate, mirroring `pdfcore.CertInfo`. Each
 * extension list is null when the extension is absent and an array (possibly
 * empty) when it is present; optional because the IPC payload is not
 * validated.
 */
export interface SignatureCertInfo {
  subject: string;
  issuer: string;
  serial: string;
  notBefore: string;
  notAfter: string;
  keyUsage?: string[] | null;
  extKeyUsage?: { oid: string; name: string }[] | null;
  crlDistributionPoints?: string[] | null;
  ocspServers?: string[] | null;
  caIssuers?: string[] | null;
}

/** One decomposed signature field, mirroring `pdfcore.SignatureField`. */
export interface SignatureEntryData {
  fieldName: string;
  signed: boolean;
  signatureRef: string;
  signatureNodeId: string;
  fieldNodeId: string;
  subFilter: string;
  type?: string;
  signingTimeRaw?: string;
  signingTime?: string;
  name?: string;
  reason?: string;
  location?: string;
  contactInfo?: string;
  digestAlgorithm?: string;
  signatureAlgorithm?: string;
  signer?: SignatureCertInfo | null;
  signerIdentified?: boolean;
  certificates: SignatureCertInfo[];
  notes: string[];
  decomposeError: string;
  byteRange?: number[];
  coversWholeFile?: boolean;
  trailingGap?: number;
  holeMatchesContents?: boolean;
  coverageError?: string;
  /** Set when the hole could not be read from disk (file changed or busy). */
  holeCheckError?: string;
  /** Encoding of the /Contents CMS as found: "DER", "BER", or empty. */
  cmsEncoding?: string;
  /** Non-zero bytes after the CMS in /Contents; absent when there are none. */
  cmsTrailingBytes?: { offset: number; count: number } | null;
}

/** Props for {@link SignaturesView}. */
export interface SignaturesViewProps {
  /** Active document tab ID. Empty string renders the no-document empty state. */
  tabId: string;
  /** True when the Signatures tab is active (data is cached regardless). */
  active: boolean;
  /** Dispatches NAVIGATE_TO_REF to jump TreePanel to the /V dict (or field). */
  onNavigate: (nodeId: string) => void;
  /**
   * Pre-fetched signature list from the parent (DetailPanel's one-fetch-per-
   * document cache). When provided the view renders it directly and issues NO
   * fetch of its own; when omitted the view self-fetches on tabId change.
   */
  data?: SignatureEntryData[] | null;
  /**
   * Parent-side fetch failure message. When set the view renders it in place
   * of the cards, as it does for its own failed fetch.
   */
  fetchError?: string | null;
}

/** Tab-level problem summary for a signature list: tone plus hover text. */
export interface SignaturesIssue {
  tone: ProblemTone;
  title: string;
}

/** Formats "N signature(s) <rest>" with the right plural. */
function countPhrase(n: number, rest: string): string {
  return `${n} signature${n === 1 ? '' : 's'} ${rest}`;
}

/**
 * Summarizes the problems in a signature list for the tab marker. Error tone
 * when any entry has a decompose or coverage error; warning tone when none
 * does but any has non-zero trailing bytes after the CMS; null when clean.
 */
export function signaturesIssue(list: SignatureEntryData[]): SignaturesIssue | null {
  const decompose = list.filter((s) => s.decomposeError).length;
  const coverage = list.filter((s) => s.coverageError).length;
  if (decompose > 0 || coverage > 0) {
    const parts: string[] = [];
    if (decompose > 0) parts.push(countPhrase(decompose, 'could not be decomposed'));
    if (coverage > 0) parts.push(countPhrase(coverage, 'could not be checked for ByteRange coverage'));
    return { tone: 'error', title: parts.join('; ') };
  }
  const trailing = list.filter((s) => s.cmsTrailingBytes).length;
  if (trailing > 0) {
    return { tone: 'warning', title: countPhrase(trailing, trailing === 1 ? 'has non-zero bytes after the CMS' : 'have non-zero bytes after the CMS') };
  }
  return null;
}

/** UI text for a hole that could not be read from disk. */
function holeCheckMessage(raw: string): string {
  if (isFileChangedMessage(raw)) return FILE_CHANGED_MESSAGE;
  if (isFileBusyMessage(raw)) return FILE_BUSY_MESSAGE;
  return raw;
}

/** Splits an RFC 4514 DN fragment on sep where sep is not backslash-escaped. */
function splitUnescaped(s: string, sep: string): string[] {
  const out: string[] = [];
  let cur = '';
  for (let i = 0; i < s.length; i++) {
    if (s[i] === '\\' && i + 1 < s.length) {
      cur += s[i] + s[++i];
    } else if (s[i] === sep) {
      out.push(cur);
      cur = '';
    } else {
      cur += s[i];
    }
  }
  out.push(cur);
  return out;
}

/** Removes RFC 4514 backslash escapes from an attribute value. */
function unescapeDNValue(v: string): string {
  return v.replace(/\\(.)/g, '$1');
}

/** Extracts the CN component from an X.509 DN string, falling back to the
 *  full DN. Card rows show the CN; the expanded chain shows full DNs. */
function cnOf(dn: string): string {
  for (const rdn of splitUnescaped(dn, ',')) {
    for (const atv of splitUnescaped(rdn, '+')) {
      if (atv.startsWith('CN=')) return unescapeDNValue(atv.slice(3));
    }
  }
  return dn;
}

/** Renders an RFC 4514 DN for reading: RDNs joined by ", ", multi-valued RDN
 *  parts by " + ", escapes removed, and a value that holds a separator, a quote
 *  or edge spaces put in double quotes so the parts stay unambiguous. */
function formatDN(dn: string): string {
  return splitUnescaped(dn, ',')
    .map((rdn) =>
      splitUnescaped(rdn, '+')
        .map((atv) => {
          const eq = atv.indexOf('=');
          if (eq < 0) return unescapeDNValue(atv);
          const v = unescapeDNValue(atv.slice(eq + 1));
          const quoted = /[,+"]|^\s|\s$/.test(v) ? `"${v.replace(/"/g, '\\"')}"` : v;
          return atv.slice(0, eq + 1) + quoted;
        })
        .join(' + ')
    )
    .join(', ');
}

/** Returns the date part (YYYY-MM-DD) of an RFC 3339 timestamp. */
function datePart(ts: string): string {
  return ts.slice(0, 10);
}

/** Renders an extension list: "absent" for null or missing, `empty` for []. */
function extensionText(values: string[] | null | undefined, empty: string): string {
  if (values == null) return 'absent';
  if (values.length === 0) return empty;
  return values.join(', ');
}

/** The five extension rows of one chain certificate, with the CLI's texts. URLs are plain text. */
function CertExtensionRows({ cert }: { cert: SignatureCertInfo }) {
  const eku = cert.extKeyUsage?.map((e) => (e.name ? `${e.name} (${e.oid})` : e.oid));
  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 mt-0.5">
      <Row label="Key usage" value={extensionText(cert.keyUsage, 'present, no bits set')} testid="signature-cert-key-usage" />
      <Row label="Ext key usage" value={extensionText(eku, 'present, no OID')} testid="signature-cert-ext-key-usage" />
      <Row label="CRL DP" value={extensionText(cert.crlDistributionPoints, 'present, no URI')} testid="signature-cert-crl-dp" />
      <Row label="OCSP" value={extensionText(cert.ocspServers, 'present, no URI')} testid="signature-cert-ocsp" />
      <Row label="CA Issuers" value={extensionText(cert.caIssuers, 'present, no URI')} testid="signature-cert-ca-issuers" />
    </dl>
  );
}

/** A small key/value row in a signature card; tone="warning" colors the value as a warning. */
function Row({
  label,
  value,
  testid,
  tone,
}: {
  label: string;
  value: string;
  testid?: string;
  tone?: 'warning';
}) {
  return (
    <>
      <dt className="text-text-muted">{label}</dt>
      <dd className={`${tone === 'warning' ? 'text-warning' : 'text-text'} break-all`} data-testid={testid}>
        {value}
      </dd>
    </>
  );
}

/**
 * Certificate-date cue: expired / not-yet-in-effect, derived from the signer
 * cert validity window ONLY. Deliberately a date fact -- no trust language.
 */
function ExpiryCue({ signer }: { signer: SignatureCertInfo }) {
  const now = new Date();
  const notAfter = new Date(signer.notAfter);
  const notBefore = new Date(signer.notBefore);
  if (!Number.isNaN(notAfter.getTime()) && notAfter < now) {
    return (
      <div
        className="mt-2 px-2 py-1 text-xs rounded border border-warning/60 text-warning"
        data-testid="signature-expiry-cue"
      >
        Signer cert expired {datePart(signer.notAfter)} (certificate date fact only)
      </div>
    );
  }
  if (!Number.isNaN(notBefore.getTime()) && notBefore > now) {
    return (
      <div
        className="mt-2 px-2 py-1 text-xs rounded border border-warning/60 text-warning"
        data-testid="signature-expiry-cue"
      >
        Signer cert dates start {datePart(signer.notBefore)} (certificate date fact only)
      </div>
    );
  }
  return null;
}

/** One signature card: decomposed facts + chain expander + reveal-in-tree. */
function SignatureCard({ entry, onNavigate }: { entry: SignatureEntryData; onNavigate: (nodeId: string) => void }) {
  const [chainOpen, setChainOpen] = useState(false);
  // Reveal target: the /V dict node for an indirect ref, the field node for a
  // direct /V; the button is omitted when neither resolves.
  const revealTarget = entry.signatureNodeId || entry.fieldNodeId;
  // Defend the IPC trust boundary: entry crosses Wails as `unknown` and is
  // cast without per-field validation, so treat notes/certificates as possibly
  // absent rather than trusting the backend's non-nil contract.
  const notes = entry.notes ?? [];
  const certificates = entry.certificates ?? [];
  const trustNote =
    notes.find((n) => n.toLowerCase().includes('trust not verified')) ??
    'trust not verified - structural decomposition only';
  const otherNotes = notes.filter((n) => !n.toLowerCase().includes('trust not verified'));

  return (
    <div
      className="border border-border rounded p-3 mb-3 text-xs font-mono"
      data-testid="signature-card"
    >
      <div className="flex items-center justify-between mb-2">
        <span className="text-sm font-medium text-text-secondary">{entry.fieldName}</span>
        <span className="text-text-muted">
          {entry.signed ? entry.signatureRef || '(direct /V)' : 'unsigned'}
        </span>
      </div>

      {!entry.signed && (
        <div className="text-text-muted" data-testid="signature-unsigned-note">
          unsigned placeholder field (no /V signature value)
        </div>
      )}

      {entry.signed && (entry.decomposeError || entry.coverageError) && (
        <ProblemCallout tone="error" testid="signature-problem" className="mb-2">
          {entry.decomposeError && <div>Could not decompose this signature: {entry.decomposeError}</div>}
          {entry.coverageError && <div>Could not check ByteRange coverage: {entry.coverageError}</div>}
        </ProblemCallout>
      )}

      {entry.signed && (
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5">
          {entry.signer && (
            <>
              <Row label="Signer" value={cnOf(entry.signer.subject)} />
              <Row label="Issuer" value={cnOf(entry.signer.issuer)} />
              <Row label="Serial" value={entry.signer.serial} />
              <Row
                label="Validity"
                value={`${datePart(entry.signer.notBefore)} to ${datePart(entry.signer.notAfter)}`}
              />
            </>
          )}
          {!entry.signer && !entry.decomposeError && (
            <Row label="Signer" value="(not identified)" />
          )}
          {entry.digestAlgorithm && <Row label="Digest algorithm" value={entry.digestAlgorithm} />}
          {entry.signatureAlgorithm && (
            <Row label="Signature algorithm" value={entry.signatureAlgorithm} />
          )}
          {entry.cmsEncoding && (
            <Row
              label="CMS encoding"
              value={entry.cmsEncoding === 'BER' ? 'BER (normalized to DER for parsing)' : entry.cmsEncoding}
              testid="signature-cms-encoding"
            />
          )}
          {entry.cmsTrailingBytes && (
            <Row
              label="Trailing bytes"
              value={`${entry.cmsTrailingBytes.count} ${entry.cmsTrailingBytes.count === 1 ? 'byte' : 'bytes'} after the CMS at offset ${entry.cmsTrailingBytes.offset} ${entry.cmsTrailingBytes.count === 1 ? 'contains' : 'contain'} non-zero data`}
              testid="signature-cms-trailing-bytes"
              tone="warning"
            />
          )}
          <Row label="SubFilter" value={entry.subFilter || '-'} />
          {entry.type && <Row label="Type" value={entry.type} />}
          {entry.signingTime || entry.signingTimeRaw ? (
            <Row label="Signing time" value={entry.signingTime || entry.signingTimeRaw || ''} />
          ) : null}
          {entry.name && <Row label="Name" value={entry.name} />}
          {entry.reason && <Row label="Reason" value={entry.reason} />}
          {entry.location && <Row label="Location" value={entry.location} />}
          {entry.contactInfo && <Row label="Contact" value={entry.contactInfo} />}
          {!entry.coverageError && entry.byteRange && entry.byteRange.length === 4 ? (
            <>
              <Row label="ByteRange" value={`[${entry.byteRange.join(' ')}]`} />
              <Row
                label="Coverage"
                value={
                  entry.coversWholeFile
                    ? 'covers the whole file except the /Contents hole'
                    : `signed range: bytes ${entry.byteRange[0]}..${entry.byteRange[2] + entry.byteRange[3]}` +
                      ((entry.trailingGap ?? 0) > 0 ? `; trailing ${entry.trailingGap} bytes not covered` : '')
                }
                testid="signature-coverage"
              />
              <Row
                label="Hole matches /Contents"
                value={entry.holeCheckError ? `not checked: ${holeCheckMessage(entry.holeCheckError)}` : entry.holeMatchesContents ? 'yes' : 'no'}
                testid="signature-hole"
              />
            </>
          ) : null}
          {otherNotes.map((n, i) => (
            <Row key={i} label="Note" value={n} />
          ))}
        </dl>
      )}

      {entry.signed && entry.signer && <ExpiryCue signer={entry.signer} />}

      {entry.signed && certificates.length > 0 && (
        <div className="mt-2">
          <button
            type="button"
            className={`px-2 py-1 ${SMALL_BUTTON}`}
            data-testid="signature-cert-chain"
            aria-expanded={chainOpen}
            onClick={() => setChainOpen((v) => !v)}
          >
            Certificate chain ({certificates.length}) {chainOpen ? '-' : '+'}
          </button>
          {chainOpen && (
            <ul className="mt-1 pl-2 border-l border-border" data-testid="signature-cert-chain-list">
              {certificates.map((c, i) => (
                <li key={i} className="py-1">
                  <div className="text-text">{formatDN(c.subject)}</div>
                  <div className="text-text-muted">issued by {formatDN(c.issuer)}</div>
                  <div className="text-text-muted">
                    serial {c.serial}, {datePart(c.notBefore)} to {datePart(c.notAfter)}
                  </div>
                  <CertExtensionRows cert={c} />
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      {entry.signed && (
        <div className="mt-2 text-text-muted italic" data-testid="signature-trust-note">
          {trustNote}
        </div>
      )}

      {revealTarget && (
        <button
          type="button"
          className={`mt-2 px-2 py-1 ${SMALL_BUTTON}`}
          data-testid="signature-reveal-in-tree"
          onClick={() => onNavigate(revealTarget)}
        >
          Reveal in tree
        </button>
      )}
    </div>
  );
}

/**
 * Document-level Signatures view. Renders the parent-cached list when the
 * `data` prop is provided (DetailPanel's one-fetch-per-document contract) and
 * the parent's `fetchError` when that fetch failed; self-fetches on tabId
 * change otherwise.
 */
export function SignaturesView({ tabId, active: _active, onNavigate, data, fetchError }: SignaturesViewProps) {
  const [entries, setEntries] = useState<SignatureEntryData[] | null>(data ?? null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (data !== undefined) {
      setEntries(data);
      return;
    }
    setEntries(null);
    setError(null);
    if (!tabId) return;
    let cancelled = false;
    GetSignatures(tabId)
      .then((result: unknown) => {
        if (cancelled) return;
        setEntries((Array.isArray(result) ? result : []) as SignatureEntryData[]);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(extractErrorMessage(err));
      });
    return () => {
      cancelled = true;
    };
  }, [tabId, data]);

  if (!tabId) {
    return (
      <div
        className="h-full flex items-center justify-center text-text-muted text-sm"
        data-testid="signatures-empty-nodoc"
      >
        No document open
      </div>
    );
  }
  const shownError = fetchError || error;
  if (shownError) {
    return (
      <div className="p-3 text-error text-sm" data-testid="signatures-error">
        {shownError}
      </div>
    );
  }
  if (entries === null) {
    return <div className="h-full" data-testid="signatures-empty-initial" />;
  }
  if (entries.length === 0) {
    return (
      <div
        className="h-full flex items-center justify-center text-text-muted text-sm"
        data-testid="signatures-empty"
      >
        No signature fields
      </div>
    );
  }

  return (
    <div className="h-full overflow-auto p-3" data-testid="signatures-view">
      {entries.map((entry, i) => (
        <SignatureCard key={entry.fieldNodeId || i} entry={entry} onNavigate={onNavigate} />
      ))}
    </div>
  );
}

export default SignaturesView;
