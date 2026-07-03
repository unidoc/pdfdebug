/**
 * @file Signatures view -- document-level decomposition of every digital
 * signature field (Story 13.4). One key/value card per signature: signer,
 * issuer, validity window, algorithms, SubFilter, signing time, and the
 * ByteRange coverage facts, plus an expandable certificate chain and a
 * "Reveal in tree" jump. Structural decomposition ONLY: the view never makes
 * a trust claim (AC4) -- the expiry cue is a certificate DATE fact and every
 * card carries the explicit "trust not verified" note.
 */
import { useEffect, useState } from 'react';
import { GetSignatures } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { extractErrorMessage } from '../lib/extractErrorMessage';

/** One decomposed X.509 certificate, mirroring `pdfcore.CertInfo`. */
export interface SignatureCertInfo {
  subject: string;
  issuer: string;
  serial: string;
  notBefore: string;
  notAfter: string;
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
}

/** Extracts the CN component from an X.509 DN string, falling back to the
 *  full DN. Card rows show the CN; the expanded chain shows full DNs. */
function cnOf(dn: string): string {
  const m = /(?:^|,)CN=([^,]+)/.exec(dn);
  return m ? m[1] : dn;
}

/** Returns the date part (YYYY-MM-DD) of an RFC 3339 timestamp. */
function datePart(ts: string): string {
  return ts.slice(0, 10);
}

/** A small key/value row in a signature card. */
function Row({ label, value, testid }: { label: string; value: string; testid?: string }) {
  return (
    <>
      <dt className="text-text-muted">{label}</dt>
      <dd className="text-text break-all" data-testid={testid}>{value}</dd>
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
  // direct /V; the button is omitted when neither resolves (AC6).
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
          <Row label="SubFilter" value={entry.subFilter || '-'} />
          {entry.type && <Row label="Type" value={entry.type} />}
          {entry.signingTime || entry.signingTimeRaw ? (
            <Row label="Signing time" value={entry.signingTime || entry.signingTimeRaw || ''} />
          ) : null}
          {entry.name && <Row label="Name" value={entry.name} />}
          {entry.reason && <Row label="Reason" value={entry.reason} />}
          {entry.location && <Row label="Location" value={entry.location} />}
          {entry.contactInfo && <Row label="Contact" value={entry.contactInfo} />}
          {entry.coverageError ? (
            <Row label="Coverage error" value={entry.coverageError} testid="signature-coverage-error" />
          ) : entry.byteRange && entry.byteRange.length === 4 ? (
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
                value={entry.holeMatchesContents ? 'yes' : 'no'}
              />
            </>
          ) : null}
          {entry.decomposeError && (
            <Row label="Decompose error" value={entry.decomposeError} testid="signature-decompose-error" />
          )}
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
            className="px-2 py-1 text-xs rounded border border-border text-text-secondary hover:bg-surface-hover cursor-pointer"
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
                  <div className="text-text">{c.subject}</div>
                  <div className="text-text-muted">issued by {c.issuer}</div>
                  <div className="text-text-muted">
                    serial {c.serial}, {datePart(c.notBefore)} to {datePart(c.notAfter)}
                  </div>
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
          className="mt-2 px-2 py-1 text-xs rounded border border-border text-text-secondary hover:bg-surface-hover cursor-pointer"
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
 * `data` prop is provided (DetailPanel's one-fetch-per-document contract);
 * self-fetches on tabId change otherwise.
 */
export function SignaturesView({ tabId, active: _active, onNavigate, data }: SignaturesViewProps) {
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
  if (error) {
    return (
      <div className="p-3 text-error text-sm" data-testid="signatures-error">
        {error}
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
