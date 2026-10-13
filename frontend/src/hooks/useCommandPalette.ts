/**
 * @file useCommandPalette -- owns open/close state for the Cmd+K palette.
 * Designed to be mounted once at App level so the keyboard
 * listener does not double-fire. Reads activeTabId from AppState to
 * force-close the palette on tab switch.
 *
 * Open/close state is held in a module-level subscribable so the
 * <CommandPalette /> component (rendered separately, also at App level)
 * can read it without prop drilling. Subscriptions use a small EventTarget
 * pattern -- one subscriber list, no library.
 */
import { useEffect, useRef, useSyncExternalStore } from 'react';
import { useAppState } from './useDocumentState';
import { getPlatformModifier } from '../lib/platform';

type Listener = () => void;
const listeners = new Set<Listener>();
let isOpen = false;

function notify() {
  for (const l of listeners) l();
}

function subscribe(l: Listener): () => void {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

function getSnapshot(): boolean {
  return isOpen;
}

/** Imperatively open the palette. */
export function openPalette() {
  if (isOpen) return;
  isOpen = true;
  notify();
}

/** Imperatively close the palette. */
export function closePalette() {
  if (!isOpen) return;
  isOpen = false;
  notify();
}

/**
 * Mount-once hook that wires the global Cmd+K / Ctrl+K listener and
 * force-closes the palette when activeTabId changes. Returns the current
 * open state so callers can also drive UI from the hook directly.
 */
export function useCommandPalette(): { isOpen: boolean; open: () => void; close: () => void } {
  const open = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  const { tabActivationVersion } = useAppState();
  // Skip the first effect run so we don't trigger a no-op close before the
  // palette has ever been opened.
  const initialVersionRef = useRef(tabActivationVersion);

  // Cmd+K (mac) or Ctrl+K (other). Skip when focus is in a text input so
  // the shortcut doesn't steal typing.
  useEffect(() => {
    function isInTextField(target: EventTarget | null): boolean {
      if (!(target instanceof HTMLElement)) return false;
      const tag = target.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA') return true;
      return target.isContentEditable;
    }
    function handler(e: KeyboardEvent) {
      if (e.key !== 'k' && e.key !== 'K') return;
      // Platform-aware modifier: Cmd on macOS, Ctrl elsewhere. Mirrors
      // useFindBar so Ctrl+K on macOS falls through to readline kill-to-EOL.
      const wantsMeta = getPlatformModifier() === 'Cmd';
      const mod = wantsMeta ? e.metaKey : e.ctrlKey;
      if (!mod) return;
      if (isInTextField(e.target)) return;
      e.preventDefault();
      openPalette();
    }
    window.addEventListener('keydown', handler);
    // On unmount, force the palette closed. Production never unmounts this
    // hook (it lives on App), but Vitest's RTL cleanup unmounts between
    // tests -- without resetting the module-level isOpen, the next test
    // would inherit a stale "open" state.
    return () => {
      window.removeEventListener('keydown', handler);
      isOpen = false;
      notify();
    };
  }, []);

  // ACTIVATE_TAB dispatch -> close. Watches a monotonic counter (not just
  // activeTabId) so same-tab activation also closes the palette, matching
  // the "switching tabs while the palette is open closes it" rule.
  useEffect(() => {
    if (tabActivationVersion === initialVersionRef.current) return;
    closePalette();
  }, [tabActivationVersion]);

  return { isOpen: open, open: openPalette, close: closePalette };
}
