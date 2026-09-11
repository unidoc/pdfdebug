/**
 * Behavior contract for the custom About surface.
 *
 * The About window must render the unidoc.io URL as a real link that opens in
 * the user's default external browser (via the Wails Browser service) rather
 * than navigating the app's own WebView away from the UI, and it must show the
 * exact running version string it is handed, including any prerelease suffix
 * (no stripping of `-rc`).
 *
 * The Go menu wiring and the real overlay/WebView rendering are verified
 * manually on macOS; this suite covers only the component behavior that jsdom
 * can observe: the anchor href, the external-open wiring, and the verbatim
 * version text.
 */
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { AboutDialog } from './AboutDialog';

const mockOpenURL = vi.hoisted(() => vi.fn<(url: string) => Promise<void>>());
vi.mock('@wailsio/runtime', () => ({
  Browser: {
    OpenURL: (url: string) => mockOpenURL(url),
  },
}));

const UNIDOC_URL = 'https://unidoc.io';

beforeEach(() => {
  mockOpenURL.mockReset();
  mockOpenURL.mockResolvedValue(undefined);
});

describe('AboutDialog', () => {
  test('renders the unidoc.io URL as an anchor pointing at the site', () => {
    render(<AboutDialog version="v0.4.0-rc1" />);

    const link = screen.getByRole('link');
    expect(link).toHaveAttribute('href', UNIDOC_URL);
  });

  test('activating the link opens it in the external browser and does not navigate the WebView', () => {
    render(<AboutDialog version="v0.4.0-rc1" />);

    const link = screen.getByRole('link');
    // fireEvent.click returns false when the handler called preventDefault,
    // i.e. the WebView is not navigated to the href.
    const notPrevented = fireEvent.click(link);

    expect(mockOpenURL).toHaveBeenCalledTimes(1);
    expect(mockOpenURL).toHaveBeenCalledWith(UNIDOC_URL);
    expect(notPrevented).toBe(false);
  });

  test('a middle-click opens the site externally and does not navigate the WebView', () => {
    render(<AboutDialog version="v0.4.0-rc1" />);

    const link = screen.getByRole('link');
    const notPrevented = fireEvent(
      link,
      new MouseEvent('auxclick', { button: 1, bubbles: true, cancelable: true }),
    );

    expect(mockOpenURL).toHaveBeenCalledTimes(1);
    expect(mockOpenURL).toHaveBeenCalledWith(UNIDOC_URL);
    expect(notPrevented).toBe(false);
  });

  test('a context-menu activation navigates nowhere and does not open the browser', () => {
    render(<AboutDialog version="v0.4.0-rc1" />);

    const link = screen.getByRole('link');
    const notPrevented = fireEvent.contextMenu(link);

    expect(mockOpenURL).not.toHaveBeenCalled();
    expect(notPrevented).toBe(false);
  });

  test('a non-middle side-button click navigates nowhere and does not open the browser', () => {
    render(<AboutDialog version="v0.4.0-rc1" />);

    const link = screen.getByRole('link');
    const notPrevented = fireEvent(
      link,
      new MouseEvent('auxclick', { button: 3, bubbles: true, cancelable: true }),
    );

    expect(mockOpenURL).not.toHaveBeenCalled();
    expect(notPrevented).toBe(false);
  });

  test('shows the full prerelease version verbatim, suffix included', () => {
    render(<AboutDialog version="v0.4.0-rc1" />);

    expect(screen.getByText(/v0\.4\.0-rc1/)).toBeInTheDocument();
  });

  test('shows a stable version verbatim', () => {
    render(<AboutDialog version="v0.4.0" />);

    expect(screen.getByText(/(^|\s)v0\.4\.0(\s|$)/)).toBeInTheDocument();
  });

  test('shows a dev build version verbatim without transformation', () => {
    render(<AboutDialog version="dev" />);

    expect(screen.getByText(/\bdev\b/)).toBeInTheDocument();
  });
});
