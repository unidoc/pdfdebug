/**
 * Platform detection utility for keyboard shortcut hints.
 *
 * Uses navigator.platform as the primary detection method because Wails WebViews
 * (WebKit on macOS, WebKit2GTK on Linux, Chromium-based on Windows) do not
 * reliably support navigator.userAgentData.
 *
 * Note: navigator.platform is deprecated but remains the most reliable
 * cross-WebView approach.
 */

/**
 * Returns the platform-specific modifier key name.
 * "Cmd" on macOS/iOS, "Ctrl" on Windows/Linux.
 */
export function getPlatformModifier(): string {
  // Guard: navigator may be undefined in SSR or non-browser test environments
  if (typeof navigator === 'undefined') {
    return 'Ctrl';
  }

  // Primary check: navigator.platform (deprecated but most reliable in WebViews)
  if (typeof navigator.platform === 'string' && /Mac|iPhone|iPad/.test(navigator.platform)) {
    return 'Cmd';
  }

  // Secondary check: navigator.userAgentData (modern browsers, not all WebViews)
  const uaPlatform = (navigator as { userAgentData?: { platform?: string } }).userAgentData?.platform;
  if (uaPlatform === 'macOS') {
    return 'Cmd';
  }
  if (uaPlatform === 'Windows' || uaPlatform === 'Linux') {
    return 'Ctrl';
  }

  return 'Ctrl';
}

/**
 * Returns a formatted keyboard shortcut hint string.
 * e.g. getShortcutHint('O') => "Cmd+O" on macOS, "Ctrl+O" on Windows/Linux
 */
export function getShortcutHint(key: string): string {
  return `${getPlatformModifier()}+${key}`;
}
