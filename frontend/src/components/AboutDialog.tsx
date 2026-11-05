import { useEffect, useRef, useState, type MouseEvent } from 'react';
import * as Dialog from '@radix-ui/react-dialog';
import { Browser, Events } from '@wailsio/runtime';
import { AboutOpen } from '../../bindings/events/index.js';

const APP_NAME = 'UniDoc PDF Debugger';
const UNIDOC_URL = 'https://unidoc.io';

/**
 * In-app About dialog, opened by the About menu item (about:open, whose payload
 * is the running version). Shows the app name, the exact version it is handed
 * (no stripping of a `-rc` suffix), and the unidoc.io URL as a link that opens
 * in the user's default external browser via the Wails Browser service, so
 * activating it never navigates the app's own WebView.
 */
export function AboutDialog() {
  const [version, setVersion] = useState<string | null>(null);
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const off = Events.On(AboutOpen, (event) => {
      const data = event?.data;
      setVersion(typeof data === 'string' && data !== '' ? data : 'dev');
    });
    return () => off();
  }, []);

  const openSite = (event: MouseEvent<HTMLAnchorElement>) => {
    // Keep every activation path (primary click, middle-click, context-menu
    // "Open Link", side buttons) from navigating the app WebView to the href;
    // route the real open through the external browser, but only for a
    // primary click or a true middle-click.
    event.preventDefault();
    if (event.type !== 'click' && event.button !== 1) {
      return;
    }
    // Swallow a rejected open (no default browser / OS handler failure) so it
    // does not surface as an unhandled promise rejection.
    void Browser.OpenURL(UNIDOC_URL).catch(() => {});
  };

  return (
    <Dialog.Root
      open={version !== null}
      onOpenChange={(open) => {
        if (!open) setVersion(null);
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/40" />
        <Dialog.Content
          data-testid="about-dialog"
          // Focus Close rather than the link, so the link opens without a focus ring.
          onOpenAutoFocus={(e) => {
            e.preventDefault();
            closeRef.current?.focus();
          }}
          className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 bg-surface border border-border rounded-md shadow-lg w-80 px-6 pt-8 pb-4 flex flex-col items-center gap-2 text-center text-text"
        >
          <Dialog.Title className="text-base font-semibold">{APP_NAME}</Dialog.Title>
          <Dialog.Description className="text-sm text-text-secondary">
            Version {version}
          </Dialog.Description>
          <a
            href={UNIDOC_URL}
            onClick={openSite}
            onAuxClick={openSite}
            onContextMenu={openSite}
            className="text-sm text-info hover:underline"
          >
            unidoc.io
          </a>
          <Dialog.Close asChild>
            <button
              ref={closeRef}
              type="button"
              className="mt-4 self-end text-sm px-3 py-1 rounded border border-border bg-surface hover:bg-surface-hover cursor-pointer focus-visible:ring-2 focus-visible:ring-border-focus focus-visible:outline-none"
            >
              Close
            </button>
          </Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
