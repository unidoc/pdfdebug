/**
 * @file Reload confirmation pill: a fixed, non-interactive status message at
 * top-center that disappears after about 3 seconds. Driven by its own module
 * store, so showing it never dispatches to AppState.
 */
import { useEffect, useSyncExternalStore } from 'react';

const VISIBLE_MS = 3000;

interface Toast {
  id: number;
  message: string;
}

type Listener = () => void;
const listeners = new Set<Listener>();
let current: Toast | null = null;
let nextId = 1;
let hideTimer: ReturnType<typeof setTimeout> | null = null;

function notify() {
  for (const l of listeners) l();
}

function subscribe(l: Listener): () => void {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

function getSnapshot(): Toast | null {
  return current;
}

/** Shows message in the pill, replacing any message on screen and restarting the timer. */
export function showReloadToast(message: string): void {
  if (hideTimer) clearTimeout(hideTimer);
  current = { id: nextId++, message };
  notify();
  hideTimer = setTimeout(() => {
    hideTimer = null;
    current = null;
    notify();
  }, VISIBLE_MS);
}

/** Removes the pill and its timer. */
function clearToast(): void {
  if (hideTimer) clearTimeout(hideTimer);
  hideTimer = null;
  current = null;
  notify();
}

/** Mount-once host for the reload confirmation pill. A message never outlives the host. */
export function ReloadToast() {
  const toast = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  useEffect(() => clearToast, []);
  return (
    <div role="status" className="pointer-events-none fixed left-1/2 top-10 z-40 -translate-x-1/2">
      {toast && (
        // key restarts the fade when a new message replaces the old one.
        <div
          key={toast.id}
          className="reload-toast whitespace-nowrap rounded-full border border-border bg-surface px-2.5 py-1 text-xs font-ui text-text shadow-sm"
        >
          {toast.message}
        </div>
      )}
    </div>
  );
}
