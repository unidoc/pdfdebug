/**
 * @file Visually hidden polite live region that announces a file changed on
 * disk once per new change. Driven by its own module store so an
 * announcement never dispatches to AppState.
 */
import { useEffect, useSyncExternalStore } from 'react';

type Listener = () => void;
const listeners = new Set<Listener>();
interface Announcement {
  id: number;
  text: string;
}

let message: Announcement = { id: 0, text: '' };

function subscribe(l: Listener): () => void {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

function getSnapshot(): Announcement {
  return message;
}

/** Sets the text the live region announces; the same text again is announced again. */
export function announceDiskChange(text: string): void {
  message = { id: message.id + 1, text };
  for (const l of listeners) l();
}

/** Mount-once live region for on-disk change announcements. Its text never outlives it. */
export function DiskChangeAnnouncer() {
  const announcement = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  useEffect(() => () => announceDiskChange(''), []);
  return (
    <div className="sr-only" aria-live="polite" data-testid="disk-change-announcer">
      {/* A new node per announcement, so a repeated text is announced again. */}
      <span key={announcement.id}>{announcement.text}</span>
    </div>
  );
}
