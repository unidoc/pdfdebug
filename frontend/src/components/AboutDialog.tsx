import type { MouseEvent } from 'react';
import { Browser } from '@wailsio/runtime';

const APP_NAME = 'UniDoc PDF Debugger';
const UNIDOC_URL = 'https://unidoc.io';

interface AboutDialogProps {
  /** Running version string, shown verbatim (prerelease suffix included). */
  version: string;
}

/**
 * About surface for the desktop app. Renders the app name, the exact running
 * version it is handed (no stripping of a `-rc` suffix), and the unidoc.io URL
 * as a link that opens in the user's default external browser via the Wails
 * Browser service, so activating it never navigates the app's own WebView.
 */
export function AboutDialog({ version }: AboutDialogProps) {
  const openSite = (event: MouseEvent<HTMLAnchorElement>) => {
    // Keep every activation path (primary click, middle-click, context-menu
    // "Open Link", side buttons) from navigating the small About WebView to the
    // href, which has no way back; route the real open through the external
    // browser, but only for a primary click or a true middle-click.
    event.preventDefault();
    if (event.type !== 'click' && event.button !== 1) {
      return;
    }
    // Swallow a rejected open (no default browser / OS handler failure) so it
    // does not surface as an unhandled promise rejection.
    void Browser.OpenURL(UNIDOC_URL).catch(() => {});
  };

  return (
    <div className="h-full flex flex-col items-center justify-center gap-2 px-6 py-8 text-center text-text">
      <h1 className="text-base font-semibold">{APP_NAME}</h1>
      <p className="text-sm text-text-secondary">Version {version}</p>
      <a
        href={UNIDOC_URL}
        onClick={openSite}
        onAuxClick={openSite}
        onContextMenu={openSite}
        className="text-sm text-info hover:underline"
      >
        unidoc.io
      </a>
    </div>
  );
}
