/**
 * Behavior contract for the in-app update notifier.
 *
 * Covers the observable jsdom behavior: badge visibility keyed on an available
 * update, the version-grouped changelog dialog, release-note sanitization,
 * the download success and verify-failure states, and the persisted auto-check
 * preference. The real OS reveal, browser-open, and native menu wiring are
 * verified manually.
 */
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { UpdateNotifier } from './UpdateNotifier';
import { UPDATE_PREF_STORAGE_KEY } from '../lib/updateConstants';

const mockCheck = vi.hoisted(() => vi.fn());
const mockLiveCall = vi.hoisted(() => vi.fn());
const mockStartupCall = vi.hoisted(() => vi.fn());
const mockDownload = vi.hoisted(() => vi.fn());
const mockSetPaused = vi.hoisted(() => vi.fn());
const eventHandlers = vi.hoisted(() => new Map<string, (data: unknown) => void>());
const mockOpenURL = vi.hoisted(() => vi.fn<(url: string) => Promise<void>>());

vi.mock('../../bindings/unidoc-pdf-debugger/internal/updateservice/service', () => ({
  CheckForUpdate: () => {
    mockLiveCall();
    return mockCheck();
  },
  CheckForUpdateAtStartup: () => {
    mockStartupCall();
    return mockCheck();
  },
  DownloadUpdate: (url: string, name: string, sums: string) => mockDownload(url, name, sums),
  SetDownloadPaused: (paused: boolean) => mockSetPaused(paused),
}));

vi.mock('@wailsio/runtime', () => ({
  Browser: { OpenURL: (url: string) => mockOpenURL(url) },
  Events: {
    On: (name: string, cb: (data: unknown) => void) => {
      eventHandlers.set(name, cb);
      return () => eventHandlers.delete(name);
    },
  },
}));

function result(overrides: Record<string, unknown> = {}) {
  return {
    updateAvailable: true,
    installedVersion: 'v1.2.0',
    latestVersion: 'v1.4.0',
    releases: [
      { tagName: 'v1.4.0', name: '1.4.0', body: '## New\n\n- shiny thing', htmlUrl: 'https://x/1.4.0', publishedAt: '2026-09-01T00:00:00Z' },
      { tagName: 'v1.3.0', name: '1.3.0', body: '- older thing', htmlUrl: 'https://x/1.3.0', publishedAt: '2026-08-01T00:00:00Z' },
    ],
    downloadUrl: 'https://x/asset',
    downloadName: 'unidoc-pdf-debugger-1.4.0-linux-amd64.tar.gz',
    sumsUrl: 'https://x/sums',
    ...overrides,
  };
}

beforeEach(() => {
  mockCheck.mockReset();
  mockLiveCall.mockReset();
  mockStartupCall.mockReset();
  mockDownload.mockReset();
  mockOpenURL.mockReset().mockResolvedValue(undefined);
  mockSetPaused.mockReset();
  eventHandlers.clear();
  window.localStorage.clear();
});

describe('UpdateNotifier', () => {
  test('shows no badge when no update is available', async () => {
    mockCheck.mockResolvedValue(result({ updateAvailable: false, releases: [] }));
    render(<UpdateNotifier />);
    await waitFor(() => expect(mockCheck).toHaveBeenCalled());
    expect(screen.queryByTestId('update-badge')).toBeNull();
  });

  test('stays silent when the automatic check fails', async () => {
    mockCheck.mockRejectedValue(new Error('offline'));
    render(<UpdateNotifier />);
    await waitFor(() => expect(mockCheck).toHaveBeenCalled());
    expect(screen.queryByTestId('update-badge')).toBeNull();
    expect(screen.queryByTestId('update-dialog')).toBeNull();
  });

  test('shows the badge and opens a version-grouped changelog', async () => {
    mockCheck.mockResolvedValue(result());
    render(<UpdateNotifier />);
    const badge = await screen.findByTestId('update-badge');
    fireEvent.click(badge);

    expect(screen.getByTestId('update-changelog')).toBeInTheDocument();
    expect(screen.getByText('1.4.0')).toBeInTheDocument();
    expect(screen.getByText('1.3.0')).toBeInTheDocument();
    // Newest expanded by default, older collapsed.
    expect(screen.getByText('shiny thing')).toBeInTheDocument();
    expect(screen.queryByText('older thing')).toBeNull();
  });

  test('"Remind me later" hides the badge for the session without persisting', async () => {
    mockCheck.mockResolvedValue(result());
    render(<UpdateNotifier />);
    fireEvent.click(await screen.findByTestId('update-badge'));
    fireEvent.click(screen.getByTestId('update-close-button')); // labeled "Remind me later"
    // Badge hidden for this run (returns next launch); not persisted as seen.
    await waitFor(() => expect(screen.queryByTestId('update-badge')).toBeNull());
    expect(window.localStorage.getItem(UPDATE_PREF_STORAGE_KEY)).toBeNull();
  });

  test('"Skip this version" hides the badge for that version', async () => {
    mockCheck.mockResolvedValue(result());
    render(<UpdateNotifier />);
    fireEvent.click(await screen.findByTestId('update-badge'));
    fireEvent.click(screen.getByTestId('update-skip-button'));
    await waitFor(() => expect(screen.queryByTestId('update-badge')).toBeNull());
    // Persisted so a relaunch stays quiet until a newer version appears.
    expect(JSON.parse(window.localStorage.getItem(UPDATE_PREF_STORAGE_KEY) as string).seenVersion).toBe('v1.4.0');
  });

  test('normalizes mixed version formats in the header', async () => {
    mockCheck.mockResolvedValue(result({ installedVersion: '0.2.0', latestVersion: 'v0.4.0' }));
    render(<UpdateNotifier />);
    fireEvent.click(await screen.findByTestId('update-badge'));
    expect(screen.getByText(/Installed v0\.2\.0 - latest v0\.4\.0/)).toBeInTheDocument();
  });

  test('a running download cannot be dismissed with Escape', async () => {
    mockCheck.mockResolvedValue(result());
    mockDownload.mockReturnValue(new Promise<string>(() => {}));
    render(<UpdateNotifier />);
    fireEvent.click(await screen.findByTestId('update-badge'));
    fireEvent.click(screen.getByTestId('update-download-button'));
    expect(await screen.findByTestId('update-download-progress')).toBeInTheDocument();

    fireEvent.keyDown(document, { key: 'Escape' });
    // Dialog stays open (dismiss is blocked while downloading).
    expect(screen.getByTestId('update-dialog')).toBeInTheDocument();
  });

  test('strips script tags from release notes', async () => {
    mockCheck.mockResolvedValue(
      result({
        releases: [
          { tagName: 'v1.4.0', name: '1.4.0', body: 'safe text <script>window.__pwned = true</script>', htmlUrl: 'h', publishedAt: '2026-09-01T00:00:00Z' },
        ],
      }),
    );
    const { container } = render(<UpdateNotifier />);
    fireEvent.click(await screen.findByTestId('update-badge'));

    expect(container.querySelector('script')).toBeNull();
    expect(screen.getByText(/safe text/)).toBeInTheDocument();
  });

  test('reports a saved download on success', async () => {
    mockCheck.mockResolvedValue(result());
    mockDownload.mockResolvedValue('/Users/x/Downloads/unidoc-pdf-debugger-1.4.0-linux-amd64.tar.gz');
    render(<UpdateNotifier />);
    fireEvent.click(await screen.findByTestId('update-badge'));
    fireEvent.click(screen.getByTestId('update-download-button'));

    expect(await screen.findByTestId('update-download-success')).toBeInTheDocument();
    expect(screen.getByText(/Saved to Downloads folder/)).toBeInTheDocument();
    expect(mockDownload).toHaveBeenCalledWith('https://x/asset', 'unidoc-pdf-debugger-1.4.0-linux-amd64.tar.gz', 'https://x/sums');
  });

  test('reopening after a finished download resets to a fresh Download button', async () => {
    mockCheck.mockResolvedValue(result());
    mockDownload.mockResolvedValue('/Users/x/Downloads/app.dmg');
    render(<UpdateNotifier />);
    fireEvent.click(await screen.findByTestId('update-badge'));
    fireEvent.click(screen.getByTestId('update-download-button'));
    expect(await screen.findByTestId('update-download-success')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('update-close-button'));
    act(() => {
      eventHandlers.get('update:check-requested')?.(null);
    });
    await waitFor(() => expect(screen.getByTestId('update-dialog')).toBeInTheDocument());

    expect(screen.getByTestId('update-download-button')).toBeInTheDocument();
    expect(screen.queryByTestId('update-download-success')).toBeNull();
  });

  test('renders live progress phases during a download', async () => {
    mockCheck.mockResolvedValue(result());
    let resolveDownload: (v: string) => void = () => {};
    mockDownload.mockReturnValue(new Promise<string>((res) => { resolveDownload = res; }));
    render(<UpdateNotifier />);
    fireEvent.click(await screen.findByTestId('update-badge'));
    fireEvent.click(screen.getByTestId('update-download-button'));

    expect(await screen.findByTestId('update-download-progress')).toBeInTheDocument();
    act(() => {
      eventHandlers.get('update:download-progress')?.({ data: { phase: 'downloading', received: 50, total: 100 } });
    });
    expect(screen.getByText(/Downloading update\.\.\. 50%/)).toBeInTheDocument();
    expect(screen.getByTestId('update-progress-bar')).toHaveStyle({ width: '50%' });
    act(() => {
      eventHandlers.get('update:download-progress')?.({ data: { phase: 'verifying', received: 0, total: 0 } });
    });
    expect(screen.getByText(/Verifying download/)).toBeInTheDocument();

    resolveDownload('/Users/x/Downloads/app.dmg');
    expect(await screen.findByTestId('update-download-success')).toBeInTheDocument();
  });

  test('locks the footer controls while a download is in flight', async () => {
    mockCheck.mockResolvedValue(result());
    mockDownload.mockReturnValue(new Promise<string>(() => {})); // never resolves
    render(<UpdateNotifier />);
    fireEvent.click(await screen.findByTestId('update-badge'));
    fireEvent.click(screen.getByTestId('update-download-button'));
    expect(await screen.findByTestId('update-download-progress')).toBeInTheDocument();

    // No fresh Download button (progress replaced it) and the dismiss/skip/toggle
    // controls are disabled so a second download or a mid-download skip is impossible.
    expect(screen.queryByTestId('update-download-button')).toBeNull();
    expect(screen.getByTestId('update-skip-button')).toBeDisabled();
    expect(screen.getByTestId('update-close-button')).toBeDisabled();
    expect(screen.getByTestId('update-autocheck-toggle')).toBeDisabled();
    expect(mockDownload).toHaveBeenCalledTimes(1);
  });

  test('cancel asks for confirmation, then aborts and returns to a fresh Download button', async () => {
    mockCheck.mockResolvedValue(result());
    const cancel = vi.fn();
    const pending = new Promise<string>(() => {}) as Promise<string> & { cancel: () => void };
    pending.cancel = cancel;
    mockDownload.mockReturnValue(pending);
    render(<UpdateNotifier />);
    fireEvent.click(await screen.findByTestId('update-badge'));
    fireEvent.click(screen.getByTestId('update-download-button'));
    expect(await screen.findByTestId('update-download-progress')).toBeInTheDocument();

    // The X asks first and pauses the transfer; nothing is cancelled yet.
    fireEvent.click(screen.getByTestId('update-cancel-button'));
    expect(await screen.findByTestId('update-cancel-confirm')).toBeInTheDocument();
    expect(cancel).not.toHaveBeenCalled();
    expect(mockSetPaused).toHaveBeenCalledWith(true);

    fireEvent.click(screen.getByTestId('update-confirm-cancel-button'));
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(await screen.findByTestId('update-download-button')).toBeInTheDocument();
    expect(screen.queryByTestId('update-download-progress')).toBeNull();
    expect(screen.getByTestId('update-skip-button')).not.toBeDisabled();
  });

  test('keeping the download from the confirm leaves it running', async () => {
    mockCheck.mockResolvedValue(result());
    const cancel = vi.fn();
    const pending = new Promise<string>(() => {}) as Promise<string> & { cancel: () => void };
    pending.cancel = cancel;
    mockDownload.mockReturnValue(pending);
    render(<UpdateNotifier />);
    fireEvent.click(await screen.findByTestId('update-badge'));
    fireEvent.click(screen.getByTestId('update-download-button'));
    expect(await screen.findByTestId('update-download-progress')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('update-cancel-button'));
    fireEvent.click(await screen.findByTestId('update-keep-download-button'));

    expect(cancel).not.toHaveBeenCalled();
    expect(screen.getByTestId('update-download-progress')).toBeInTheDocument();
    // Paused on prompt, resumed on keep.
    expect(mockSetPaused).toHaveBeenCalledWith(true);
    expect(mockSetPaused).toHaveBeenLastCalledWith(false);
  });

  test('shows a retry and releases-page fallback on verify failure', async () => {
    mockCheck.mockResolvedValue(result());
    mockDownload.mockRejectedValue(new Error('update download failed checksum verification'));
    render(<UpdateNotifier />);
    fireEvent.click(await screen.findByTestId('update-badge'));
    fireEvent.click(screen.getByTestId('update-download-button'));

    const failed = await screen.findByTestId('update-download-verify-failed');
    expect(failed).toBeInTheDocument();
    expect(screen.getByText('Retry')).toBeInTheDocument();
    expect(screen.getByText('Download from the releases page')).toBeInTheDocument();
  });

  test('opens the release page when no platform asset matched', async () => {
    mockCheck.mockResolvedValue(result({ downloadUrl: '', downloadName: '' }));
    render(<UpdateNotifier />);
    fireEvent.click(await screen.findByTestId('update-badge'));
    fireEvent.click(screen.getByTestId('update-release-page-button'));

    expect(mockOpenURL).toHaveBeenCalledWith('https://x/1.4.0');
  });

  test('persists the auto-check preference to localStorage', async () => {
    mockCheck.mockResolvedValue(result());
    render(<UpdateNotifier />);
    fireEvent.click(await screen.findByTestId('update-badge'));

    fireEvent.click(screen.getByTestId('update-autocheck-toggle'));
    await waitFor(() => {
      const raw = window.localStorage.getItem(UPDATE_PREF_STORAGE_KEY);
      expect(raw).not.toBeNull();
      expect(JSON.parse(raw as string).autoCheck).toBe(false);
    });
  });

  test('does not run the automatic check when the preference is off', async () => {
    window.localStorage.setItem(UPDATE_PREF_STORAGE_KEY, JSON.stringify({ autoCheck: false }));
    mockCheck.mockResolvedValue(result());
    render(<UpdateNotifier />);
    // Give any effect a chance to run.
    await Promise.resolve();
    expect(mockCheck).not.toHaveBeenCalled();
    expect(screen.queryByTestId('update-badge')).toBeNull();
  });

  test('a manual check shows the up-to-date message', async () => {
    mockCheck.mockResolvedValue(result({ updateAvailable: false, releases: [], latestVersion: '' }));
    render(<UpdateNotifier />);
    await waitFor(() => expect(mockCheck).toHaveBeenCalledTimes(1));

    eventHandlers.get('update:check-requested')?.(null);
    expect(await screen.findByText(/running the latest version/)).toBeInTheDocument();
  });

  test('the automatic check uses the startup binding and the Help menu uses the live one', async () => {
    mockCheck.mockResolvedValue(result({ updateAvailable: false, releases: [], latestVersion: '' }));
    render(<UpdateNotifier />);
    await waitFor(() => expect(mockStartupCall).toHaveBeenCalledTimes(1));
    expect(mockLiveCall).not.toHaveBeenCalled();

    eventHandlers.get('update:check-requested')?.(null);
    await waitFor(() => expect(mockLiveCall).toHaveBeenCalledTimes(1));
    expect(mockStartupCall).toHaveBeenCalledTimes(1);
  });
});
