/**
 * Behavior contract for the in-app About dialog.
 *
 * The dialog opens on about:open and shows the exact running version it is
 * handed, including any prerelease suffix (no stripping of `-rc`). The
 * unidoc.io URL must render as a real link that opens in the user's default
 * external browser (via the Wails Browser service) rather than navigating the
 * app's own WebView away from the UI.
 *
 * The Go menu wiring is verified manually on each OS; this suite covers the
 * component behavior jsdom can observe.
 */
import { act, render, screen, fireEvent } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { AboutDialog } from './AboutDialog';

type Handler = (event: { data?: unknown }) => void;

const mockOpenURL = vi.hoisted(() => vi.fn<(url: string) => Promise<void>>());
const handlers = vi.hoisted(() => new Map<string, Handler>());
vi.mock('@wailsio/runtime', () => ({
  Browser: {
    OpenURL: (url: string) => mockOpenURL(url),
  },
  Events: {
    On: (name: string, handler: Handler) => {
      handlers.set(name, handler);
      return () => handlers.delete(name);
    },
  },
}));

const UNIDOC_URL = 'https://unidoc.io';

function openAbout(data: unknown) {
  render(<AboutDialog />);
  act(() => handlers.get('about:open')?.({ data }));
}

beforeEach(() => {
  handlers.clear();
  mockOpenURL.mockReset();
  mockOpenURL.mockResolvedValue(undefined);
});

describe('AboutDialog', () => {
  test('stays closed until about:open arrives', () => {
    render(<AboutDialog />);

    expect(screen.queryByTestId('about-dialog')).not.toBeInTheDocument();
  });

  test('opens as a modal dialog with the app name on about:open', () => {
    openAbout('v0.4.0-rc1');

    expect(screen.getByRole('dialog', { name: 'UniDoc PDF Debugger' })).toBeInTheDocument();
  });

  test('the Close button closes the dialog', () => {
    openAbout('v0.4.0-rc1');

    fireEvent.click(screen.getByRole('button', { name: 'Close' }));

    expect(screen.queryByTestId('about-dialog')).not.toBeInTheDocument();
  });

  test('Escape closes the dialog', () => {
    openAbout('v0.4.0-rc1');

    fireEvent.keyDown(screen.getByTestId('about-dialog'), { key: 'Escape' });

    expect(screen.queryByTestId('about-dialog')).not.toBeInTheDocument();
  });

  test('focus starts on Close, not on the link', () => {
    openAbout('v0.4.0-rc1');

    expect(screen.getByRole('button', { name: 'Close' })).toHaveFocus();
  });

  test('renders the unidoc.io URL as an anchor pointing at the site', () => {
    openAbout('v0.4.0-rc1');

    expect(screen.getByRole('link')).toHaveAttribute('href', UNIDOC_URL);
  });

  test('activating the link opens it in the external browser and does not navigate the WebView', () => {
    openAbout('v0.4.0-rc1');

    // fireEvent.click returns false when the handler called preventDefault,
    // i.e. the WebView is not navigated to the href.
    const notPrevented = fireEvent.click(screen.getByRole('link'));

    expect(mockOpenURL).toHaveBeenCalledTimes(1);
    expect(mockOpenURL).toHaveBeenCalledWith(UNIDOC_URL);
    expect(notPrevented).toBe(false);
  });

  test('a middle-click opens the site externally and does not navigate the WebView', () => {
    openAbout('v0.4.0-rc1');

    const notPrevented = fireEvent(
      screen.getByRole('link'),
      new MouseEvent('auxclick', { button: 1, bubbles: true, cancelable: true }),
    );

    expect(mockOpenURL).toHaveBeenCalledTimes(1);
    expect(mockOpenURL).toHaveBeenCalledWith(UNIDOC_URL);
    expect(notPrevented).toBe(false);
  });

  test('a context-menu activation navigates nowhere and does not open the browser', () => {
    openAbout('v0.4.0-rc1');

    const notPrevented = fireEvent.contextMenu(screen.getByRole('link'));

    expect(mockOpenURL).not.toHaveBeenCalled();
    expect(notPrevented).toBe(false);
  });

  test('a non-middle side-button click navigates nowhere and does not open the browser', () => {
    openAbout('v0.4.0-rc1');

    const notPrevented = fireEvent(
      screen.getByRole('link'),
      new MouseEvent('auxclick', { button: 3, bubbles: true, cancelable: true }),
    );

    expect(mockOpenURL).not.toHaveBeenCalled();
    expect(notPrevented).toBe(false);
  });

  test('shows the full prerelease version verbatim, suffix included', () => {
    openAbout('v0.4.0-rc1');

    expect(screen.getByText(/v0\.4\.0-rc1/)).toBeInTheDocument();
  });

  test('shows a stable version verbatim', () => {
    openAbout('v0.4.0');

    expect(screen.getByText(/(^|\s)v0\.4\.0(\s|$)/)).toBeInTheDocument();
  });

  test('shows a dev build version verbatim without transformation', () => {
    openAbout('dev');

    expect(screen.getByText(/\bdev\b/)).toBeInTheDocument();
  });

  test('falls back to dev when the event carries no version', () => {
    openAbout(undefined);

    expect(screen.getByText('Version dev')).toBeInTheDocument();
  });
});
