/**
 * @file In-app update notification: a quiet badge, a version-grouped changelog
 * dialog, and a checksum-verified assisted download. Mounts once at the app root.
 *
 * The badge appears only when a newer release is available and has not already
 * been dismissed for that version. Release notes are rendered through a
 * sanitizing markdown pipeline (never raw HTML). The download delegates to the
 * backend, which verifies the asset against SHA256SUMS.txt before it reaches
 * Downloads; a failed verification never yields a saved file.
 */
import { useCallback, useEffect, useRef, useState } from 'react';
import * as Dialog from '@radix-ui/react-dialog';
import ReactMarkdown from 'react-markdown';
import type { Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeSanitize from 'rehype-sanitize';
import { Browser, Events } from '@wailsio/runtime';
import { ArrowDownToLine, ArrowUpCircle, ChevronDown, ChevronRight, X } from 'lucide-react';
import { CheckForUpdate, CheckForUpdateAtStartup, DownloadUpdate, SetDownloadPaused } from '../../bindings/unidoc-pdf-debugger/internal/updateservice/service';
import type { Result } from '../../bindings/unidoc-pdf-debugger/internal/updatecheck/models';
import { useUpdatePreference } from '../hooks/useUpdatePreference';
import { UPDATE_VERIFY_ESCALATE_AFTER } from '../lib/updateConstants';

/**
 * Markdown element renderers. Links open in the system browser (the WebView must
 * never navigate to an external URL); other elements carry Tailwind classes so
 * no custom CSS is introduced.
 */
const markdownComponents: Components = {
  a: ({ href, children }) => (
    <button
      type="button"
      className="text-info underline"
      onClick={(e) => {
        e.preventDefault();
        if (href) void Browser.OpenURL(href).catch(() => {});
      }}
    >
      {children}
    </button>
  ),
  p: ({ children }) => <p className="my-1">{children}</p>,
  ul: ({ children }) => <ul className="ml-4 list-disc">{children}</ul>,
  ol: ({ children }) => <ol className="ml-4 list-decimal">{children}</ol>,
  li: ({ children }) => <li className="my-0.5">{children}</li>,
  code: ({ children }) => <code className="rounded bg-surface-hover px-1 text-xs">{children}</code>,
  h1: ({ children }) => <h3 className="mb-1 mt-2 font-medium">{children}</h3>,
  h2: ({ children }) => <h3 className="mb-1 mt-2 font-medium">{children}</h3>,
  h3: ({ children }) => <h4 className="mb-1 mt-2 font-medium">{children}</h4>,
};

type DownloadState =
  | { kind: 'idle' }
  | { kind: 'downloading'; phase: string; received: number; total: number }
  | { kind: 'success'; savedPath: string }
  | { kind: 'verify-failed'; detail: string }
  | { kind: 'error'; detail: string };

interface DownloadProgress {
  phase: string;
  received: number;
  total: number;
}

// Friendly label per backend download phase (updatecheck PhaseDownloading/Verifying/Saving).
function phaseLabel(phase: string, received: number, total: number): string {
  if (phase === 'verifying') return 'Verifying download...';
  if (phase === 'saving') return 'Saving to Downloads...';
  if (total > 0) {
    return `Downloading update... ${Math.floor((received / total) * 100)}%`;
  }
  return 'Downloading update...';
}

// Substrings of the Go errors ErrChecksumVerify / ErrChecksumMissing
// (internal/updatecheck/updatecheck.go), matched on the rejected-promise message
// to classify a download failure. Keep in sync with those error strings.
const CHECKSUM_VERIFY_MARKER = 'checksum verification';
const CHECKSUM_MISSING_MARKER = 'checksum is unavailable';

// displayVersion normalizes a version to a single leading "v" for display. The
// ldflag build version has no "v" (release.yml strips it) while GitHub tags carry
// one, so they are unified at the render site (comparisons normalize separately).
function displayVersion(v: string): string {
  if (!v) return v;
  return v.startsWith('v') ? v : `v${v}`;
}

function formatDate(iso: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString();
}

// The containing folder's name from a saved file path, across / and \ separators.
// Reflects the real per-platform Downloads directory (localized on Linux).
function folderName(savedPath: string): string {
  const parts = savedPath.split(/[/\\]/).filter(Boolean);
  return parts.length >= 2 ? parts[parts.length - 2] : 'Downloads';
}

/** Badge + dialog orchestrator for in-app update notifications. */
export function UpdateNotifier(): JSX.Element | null {
  const { autoCheck, seenVersion, setAutoCheck, setSeenVersion } = useUpdatePreference();
  const [result, setResult] = useState<Result | null>(null);
  const [open, setOpen] = useState(false);
  const [checkError, setCheckError] = useState(false);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [download, setDownload] = useState<DownloadState>({ kind: 'idle' });
  const [showSteps, setShowSteps] = useState(false);
  const [showDetail, setShowDetail] = useState(false);
  const [confirmCancelOpen, setConfirmCancelOpen] = useState(false);
  // "Remind me later" hides the badge for this run only (not persisted); it
  // returns on the next launch. "Skip this version" persists (seenVersion).
  const [remindedThisSession, setRemindedThisSession] = useState(false);
  const failCountRef = useRef(0);
  const downloadingRef = useRef(false);
  const cancelledRef = useRef(false);
  const downloadRef = useRef<{ cancel?: () => void } | null>(null);

  const runCheck = useCallback(async (isManual: boolean) => {
    setCheckError(false);
    try {
      // The automatic check may be answered from the app's cache record;
      // the Help menu check always goes live.
      const res = await (isManual ? CheckForUpdate() : CheckForUpdateAtStartup());
      setResult(res);
      if (res.updateAvailable && res.releases.length > 0) {
        setExpanded({ [res.releases[0].tagName]: true });
      }
      if (isManual) setOpen(true);
    } catch {
      setCheckError(true);
      setResult(null);
      if (isManual) setOpen(true);
    }
  }, []);

  // Automatic check once on mount, only when the preference allows it.
  useEffect(() => {
    if (autoCheck) {
      void runCheck(false);
    }
    // Intentionally runs once; the preference is read at startup.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Manual check from the Help menu.
  useEffect(() => {
    const off = Events.On('update:check-requested', () => {
      void runCheck(true);
    });
    return () => off();
  }, [runCheck]);

  // Live download progress from the backend. Only updates while a download is in
  // flight so a late event cannot revive a finished (success/failed) state.
  useEffect(() => {
    const off = Events.On('update:download-progress', (event: { data?: DownloadProgress }) => {
      const p = event?.data;
      if (!p) return;
      setDownload((prev) =>
        prev.kind === 'downloading'
          ? { kind: 'downloading', phase: p.phase, received: p.received, total: p.total }
          : prev,
      );
    });
    return () => off();
  }, []);

  const updateReady = !!result?.updateAvailable && result.releases.length > 0;
  const badgeVisible = updateReady && result?.latestVersion !== seenVersion && !remindedThisSession;
  // While a download is in flight, lock the footer controls so the user can't
  // skip/dismiss/toggle mid-download (e.g. Download then Skip this version).
  const isDownloading = download.kind === 'downloading';

  const openFromBadge = useCallback(() => {
    setOpen(true);
  }, []);

  const handleOpenChange = useCallback((next: boolean) => {
    setOpen(next);
    if (!next) {
      // A plain close (button, Escape, overlay) is "remind me later": the badge
      // is NOT marked seen, so it returns on the next launch while the update
      // exists. "Skip this version" is the only path that records the seen
      // version (handleSkip).
      setShowSteps(false);
      setShowDetail(false);
      // Dismissing hides the badge for this run (it returns next launch).
      setRemindedThisSession(true);
      // An IN-FLIGHT download persists across close (reopening shows its live
      // progress and no second download starts). A FINISHED download
      // (success/failed/error) resets so reopening offers a fresh Download.
      if (!downloadingRef.current) {
        setDownload({ kind: 'idle' });
        failCountRef.current = 0;
      }
    }
  }, []);

  const handleSkip = useCallback(() => {
    if (result?.latestVersion) setSeenVersion(result.latestVersion);
    setOpen(false);
  }, [result, setSeenVersion]);

  const openReleasePage = useCallback(() => {
    const url = result?.releases[0]?.htmlUrl;
    if (url) void Browser.OpenURL(url).catch(() => {});
  }, [result]);

  const runDownload = useCallback(async () => {
    if (!result?.downloadUrl) return;
    // Re-entry guard: never start a second download while one is in flight
    // (a rapid double-click, or reopening the dialog and clicking again).
    if (downloadingRef.current) return;
    downloadingRef.current = true;
    cancelledRef.current = false;
    setDownload({ kind: 'downloading', phase: 'downloading', received: 0, total: 0 });
    try {
      const promise = DownloadUpdate(result.downloadUrl, result.downloadName, result.sumsUrl);
      downloadRef.current = promise;
      const saved = await promise;
      failCountRef.current = 0;
      setDownload({ kind: 'success', savedPath: saved });
    } catch (err) {
      // A user cancel rejects the promise; leave the state at idle (set by
      // cancelDownload) instead of surfacing an error.
      if (cancelledRef.current) {
        cancelledRef.current = false;
        return;
      }
      const detail = err instanceof Error ? err.message : String(err);
      if (detail.includes(CHECKSUM_MISSING_MARKER)) {
        openReleasePage();
        setDownload({ kind: 'error', detail });
      } else if (detail.includes(CHECKSUM_VERIFY_MARKER)) {
        failCountRef.current += 1;
        setDownload({ kind: 'verify-failed', detail });
      } else {
        setDownload({ kind: 'error', detail });
      }
    } finally {
      downloadingRef.current = false;
      downloadRef.current = null;
    }
  }, [result, openReleasePage]);

  const cancelDownload = useCallback(() => {
    // No-op if the download already finished (e.g. it completed while the confirm
    // prompt was open) so a stale confirm can't wipe a success state.
    if (!downloadingRef.current) return;
    cancelledRef.current = true;
    downloadRef.current?.cancel?.();
    downloadingRef.current = false;
    setDownload({ kind: 'idle' });
  }, []);

  const requestCancel = useCallback(() => {
    // Pause the transfer while the user decides, then show the confirm prompt.
    void SetDownloadPaused(true);
    setConfirmCancelOpen(true);
  }, []);

  const handleConfirmOpenChange = useCallback((next: boolean) => {
    setConfirmCancelOpen(next);
    // Closing the prompt without cancelling (Keep downloading, Escape, overlay)
    // resumes the still-running download.
    if (!next && downloadingRef.current) {
      void SetDownloadPaused(false);
    }
  }, []);

  const confirmCancel = useCallback(() => {
    cancelDownload();
    setConfirmCancelOpen(false);
  }, [cancelDownload]);

  const toggleExpanded = useCallback((tag: string) => {
    setExpanded((prev) => ({ ...prev, [tag]: !prev[tag] }));
  }, []);

  if (!badgeVisible && !open) return null;

  return (
    <>
      {badgeVisible && (
        <button
          type="button"
          onClick={openFromBadge}
          data-testid="update-badge"
          title="A new version is available"
          className="fixed top-1 right-2 z-40 flex items-center gap-1.5 rounded-full border border-border bg-surface px-2.5 py-1 text-xs font-ui text-text shadow-sm hover:bg-surface-hover"
        >
          <ArrowUpCircle className="h-3.5 w-3.5 shrink-0 text-info" aria-hidden="true" />
          Update
        </button>
      )}

      <Dialog.Root open={open} onOpenChange={handleOpenChange}>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 z-50 bg-black/40" />
          <Dialog.Content
            data-testid="update-dialog"
            className="fixed left-1/2 top-1/2 z-50 flex max-h-[80vh] w-[32rem] max-w-[90vw] -translate-x-1/2 -translate-y-1/2 flex-col rounded-md border border-border bg-surface shadow-lg"
            onEscapeKeyDown={(e) => { if (isDownloading) e.preventDefault(); }}
            onPointerDownOutside={(e) => { if (isDownloading) e.preventDefault(); }}
          >
            <Dialog.Title className="border-b border-border px-4 py-3 text-sm font-ui text-text">
              {updateReady ? 'Update available' : checkError ? 'Update check' : 'You are up to date'}
            </Dialog.Title>

            <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
              {updateReady ? (
                <div className="flex flex-col gap-3" data-testid="update-changelog">
                  <p className="text-xs text-text-muted">
                    Installed {displayVersion(result?.installedVersion ?? '')} - latest{' '}
                    {displayVersion(result?.latestVersion ?? '')}
                  </p>
                  {result?.releases.map((rel, i) => {
                    const isExpanded = expanded[rel.tagName] ?? i === 0;
                    return (
                      <section key={rel.tagName} className="rounded border border-border">
                        <button
                          type="button"
                          onClick={() => toggleExpanded(rel.tagName)}
                          className="flex w-full items-center gap-1 px-2 py-1.5 text-left text-sm font-ui text-text hover:bg-surface-hover"
                        >
                          {isExpanded ? (
                            <ChevronDown className="h-3.5 w-3.5" aria-hidden="true" />
                          ) : (
                            <ChevronRight className="h-3.5 w-3.5" aria-hidden="true" />
                          )}
                          <span className="font-medium">{rel.name || rel.tagName}</span>
                          <span className="ml-auto text-xs text-text-muted">{formatDate(rel.publishedAt)}</span>
                        </button>
                        {isExpanded && (
                          <div data-testid="update-release-notes" className="px-3 py-2 text-sm text-text">
                            <ReactMarkdown
                              remarkPlugins={[remarkGfm]}
                              rehypePlugins={[rehypeSanitize]}
                              components={markdownComponents}
                            >
                              {rel.body || '_No release notes._'}
                            </ReactMarkdown>
                          </div>
                        )}
                      </section>
                    );
                  })}
                </div>
              ) : checkError ? (
                <p className="text-sm text-text">Couldn&apos;t reach GitHub to check for updates. Try again later.</p>
              ) : (
                <p className="text-sm text-text">
                  You&apos;re running the latest version{result?.installedVersion ? ` (${displayVersion(result.installedVersion)})` : ''}.
                </p>
              )}
            </div>

            <div className="border-t border-border px-4 py-3">
              {updateReady && <DownloadArea
                download={download}
                hasAsset={!!result?.downloadUrl}
                failCount={failCountRef.current}
                showSteps={showSteps}
                showDetail={showDetail}
                onDownload={runDownload}
                onCancel={requestCancel}
                onOpenReleasePage={openReleasePage}
                onToggleSteps={() => setShowSteps((s) => !s)}
                onToggleDetail={() => setShowDetail((s) => !s)}
              />}

              <div className="mt-3 flex items-center justify-between gap-3">
                <label className="flex items-center gap-1.5 text-xs text-text-muted">
                  <input
                    type="checkbox"
                    data-testid="update-autocheck-toggle"
                    checked={autoCheck}
                    disabled={isDownloading}
                    onChange={(e) => setAutoCheck(e.target.checked)}
                  />
                  Check for updates automatically
                </label>
                <div className="flex items-center gap-2">
                  {updateReady && (
                    <button
                      type="button"
                      data-testid="update-skip-button"
                      onClick={handleSkip}
                      disabled={isDownloading}
                      className="rounded border border-border px-3 py-1 text-sm font-ui text-text hover:bg-surface-hover disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      Skip this version
                    </button>
                  )}
                  <Dialog.Close asChild>
                    <button
                      type="button"
                      data-testid="update-close-button"
                      disabled={isDownloading}
                      className="rounded border border-border px-3 py-1 text-sm font-ui text-text hover:bg-surface-hover disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      {updateReady ? 'Remind me later' : 'Close'}
                    </button>
                  </Dialog.Close>
                </div>
              </div>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>

      <Dialog.Root open={confirmCancelOpen} onOpenChange={handleConfirmOpenChange}>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 z-[60] bg-black/40" />
          <Dialog.Content
            data-testid="update-cancel-confirm"
            className="fixed left-1/2 top-1/2 z-[60] w-80 max-w-[90vw] -translate-x-1/2 -translate-y-1/2 rounded-md border border-border bg-surface p-4 shadow-lg"
          >
            <Dialog.Title className="text-sm font-ui text-text">Cancel download?</Dialog.Title>
            <Dialog.Description className="mt-1 text-xs text-text-muted">
              The update download will stop and the partial file will be discarded.
            </Dialog.Description>
            <div className="mt-4 flex justify-end gap-2">
              <button
                type="button"
                data-testid="update-keep-download-button"
                onClick={() => handleConfirmOpenChange(false)}
                className="rounded border border-border px-3 py-1 text-sm font-ui text-text hover:bg-surface-hover"
              >
                Keep downloading
              </button>
              <button
                type="button"
                data-testid="update-confirm-cancel-button"
                onClick={confirmCancel}
                className="rounded bg-error px-3 py-1 text-sm font-ui text-white"
              >
                Cancel download
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </>
  );
}

interface DownloadAreaProps {
  download: DownloadState;
  hasAsset: boolean;
  failCount: number;
  showSteps: boolean;
  showDetail: boolean;
  onDownload: () => void;
  onCancel: () => void;
  onOpenReleasePage: () => void;
  onToggleSteps: () => void;
  onToggleDetail: () => void;
}

/** The download action row: primary button plus success/failure states. */
function DownloadArea(props: DownloadAreaProps): JSX.Element {
  const { download, hasAsset, failCount } = props;

  if (download.kind === 'downloading') {
    const { phase, received, total } = download;
    const downloading = phase === 'downloading';
    // Pulse only when actively downloading a stream of unknown size. At the very
    // start (0 / 0) the bar is a determinate 0% so it fills up from empty rather
    // than animating down from a stale/indeterminate width. Verifying/saving show
    // a full bar since the download itself is complete.
    const receiving = downloading && total <= 0 && received > 0;
    let pct = 0;
    if (!downloading) pct = 100;
    else if (total > 0) pct = Math.min(100, Math.floor((received / total) * 100));
    return (
      <div data-testid="update-download-progress" className="flex flex-col gap-1.5 text-sm">
        <div className="flex items-center gap-1.5 text-text">
          <ArrowDownToLine className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
          <span>{phaseLabel(phase, received, total)}</span>
          {downloading && (
            <button
              type="button"
              data-testid="update-cancel-button"
              onClick={props.onCancel}
              aria-label="Cancel download"
              title="Cancel download"
              className="ml-auto shrink-0 rounded p-0.5 text-text-muted hover:bg-surface-hover hover:text-text"
            >
              <X className="h-3.5 w-3.5" aria-hidden="true" />
            </button>
          )}
        </div>
        <div className="h-1.5 w-full overflow-hidden rounded-full bg-surface-hover">
          <div
            data-testid="update-progress-bar"
            className={`h-1.5 rounded-full bg-info ${receiving ? 'w-1/3 animate-pulse' : 'transition-[width] duration-150'}`}
            style={receiving ? undefined : { width: `${pct}%` }}
          />
        </div>
      </div>
    );
  }

  if (download.kind === 'success') {
    return (
      <div data-testid="update-download-success" className="text-sm text-text">
        Saved to {folderName(download.savedPath)} folder.
      </div>
    );
  }

  if (download.kind === 'verify-failed') {
    const escalate = failCount >= UPDATE_VERIFY_ESCALATE_AFTER;
    return (
      <div data-testid="update-download-verify-failed" className="flex flex-col gap-2 text-sm">
        <p className="text-text">
          The download didn&apos;t verify - it may have been interrupted. We removed it.{' '}
          {escalate ? 'Please download it from the releases page instead.' : 'Try again.'}
        </p>
        <div className="flex items-center gap-2">
          {!escalate && (
            <button
              type="button"
              onClick={props.onDownload}
              className="rounded bg-info px-3 py-1 text-sm font-ui text-white"
            >
              Retry
            </button>
          )}
          <button
            type="button"
            onClick={props.onOpenReleasePage}
            className="text-xs text-text-muted underline hover:text-text"
          >
            Download from the releases page
          </button>
        </div>
        <button type="button" onClick={props.onToggleDetail} className="self-start text-xs text-text-muted underline">
          {props.showDetail ? 'Hide details' : 'Show details'}
        </button>
        {props.showDetail && <p className="break-words text-xs text-text-muted">{download.detail}</p>}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        {hasAsset ? (
          <button
            type="button"
            data-testid="update-download-button"
            onClick={props.onDownload}
            className="flex items-center gap-1.5 rounded bg-info px-3 py-1 text-sm font-ui text-white"
          >
            <ArrowDownToLine className="h-3.5 w-3.5" aria-hidden="true" />
            Download
          </button>
        ) : (
          <button
            type="button"
            data-testid="update-release-page-button"
            onClick={props.onOpenReleasePage}
            className="rounded bg-info px-3 py-1 text-sm font-ui text-white"
          >
            View release page
          </button>
        )}
      </div>
      <p className="text-xs text-text-muted">
        This is an unsigned build, so the OS will warn you the first time you open it.{' '}
        <button type="button" onClick={props.onToggleSteps} className="underline hover:text-text">
          {props.showSteps ? 'Hide steps' : 'How to open it'}
        </button>
      </p>
      {props.showSteps && (
        <ul className="ml-4 list-disc text-xs text-text-muted">
          <li>macOS: right-click the app, choose Open, then confirm Open in the Gatekeeper prompt.</li>
          <li>Windows: on the SmartScreen prompt, choose More info, then Run anyway.</li>
        </ul>
      )}
      {download.kind === 'error' && (
        <p data-testid="update-download-error" className="break-words text-xs text-error">
          Something went wrong: {download.detail}
        </p>
      )}
    </div>
  );
}
