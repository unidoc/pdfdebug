/**
 * @file Overlay banner for the background pdfcpu check of the active tab. It
 * shows when pdfcpu rejected the file or recorded notices, sits over the
 * bottom-right corner of the content container so it takes no layout space,
 * and its Details button opens the Validate tab on the pdfcpu profile and
 * hides the banner for that tab.
 */
import { ErrorBanner } from './ErrorBanner';
import type { ValidateResult } from './ValidateView';
import { PDFCPU_PROFILE, dismissPdfcpuBanner, requestValidateRun, usePdfcpuCheck } from '../lib/pdfcpuCheck';

/**
 * Returns the banner text for a pdfcpu result, or null when it has neither a
 * fatal problem nor a notice. Not-evaluated (info) problems show nothing.
 */
export function pdfcpuBannerText(result: ValidateResult): string | null {
  const fatal = result.problems.find((p) => p.ruleId === 'pdfcpu-validator' && p.severity === 'error');
  if (fatal) return `pdfcpu rejects this file: ${fatal.message}`;
  const notices = result.problems.filter((p) => p.ruleId === 'pdfcpu-notices' && p.severity === 'warning').length;
  if (notices === 0) return null;
  return `pdfcpu repaired or accepted ${notices} ${notices === 1 ? 'problem' : 'problems'} in this file`;
}

/** Props for {@link PdfcpuBanner}. */
export interface PdfcpuBannerProps {
  /** The active document tab id, or null with no document. */
  tabId: string | null;
}

/** The pdfcpu check banner for tabId; renders nothing when there is nothing to show. */
export function PdfcpuBanner({ tabId }: PdfcpuBannerProps) {
  const check = usePdfcpuCheck(tabId);
  if (!tabId || !check?.result || check.dismissed) return null;
  const text = pdfcpuBannerText(check.result);
  if (!text) return null;
  return (
    <div className="absolute bottom-3 right-3 z-30 max-w-xl shadow-md rounded overflow-hidden">
      <ErrorBanner
        message={text}
        severity="warning"
        testId="pdfcpu-banner"
        singleLine
        actionLabel="Details"
        onAction={() => {
          // The Validate tab now lists the findings, and the banner would cover it.
          dismissPdfcpuBanner(tabId);
          requestValidateRun(tabId, PDFCPU_PROFILE);
        }}
        onDismiss={() => dismissPdfcpuBanner(tabId)}
      />
    </div>
  );
}
