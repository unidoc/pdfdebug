/**
 * Hook for the "check for updates automatically" preference, persisted in
 * window.localStorage under UPDATE_PREF_STORAGE_KEY (the same mechanism as
 * window-geometry persistence). Defaults to ON when unset or unreadable.
 */
import { useState, useCallback } from 'react';
import { UPDATE_PREF_STORAGE_KEY } from '../lib/updateConstants';

interface UpdatePrefs {
  autoCheck: boolean;
  /** The latest release tag the user has already been shown, to avoid re-nagging. */
  seenVersion?: string;
}

function readPrefs(): UpdatePrefs {
  try {
    const raw = window.localStorage.getItem(UPDATE_PREF_STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<UpdatePrefs>;
      return {
        autoCheck: parsed.autoCheck !== false,
        seenVersion: typeof parsed.seenVersion === 'string' ? parsed.seenVersion : undefined,
      };
    }
  } catch {
    // Unreadable storage falls back to the default.
  }
  return { autoCheck: true };
}

function writePrefs(prefs: UpdatePrefs): void {
  try {
    window.localStorage.setItem(UPDATE_PREF_STORAGE_KEY, JSON.stringify(prefs));
  } catch {
    // Ignore write failures (private mode, blocked storage).
  }
}

/** Returns the auto-check flag, the last-seen version, and setters that persist. */
export function useUpdatePreference() {
  const [prefs, setPrefs] = useState<UpdatePrefs>(readPrefs);

  const setAutoCheck = useCallback((autoCheck: boolean) => {
    setPrefs((prev) => {
      const next = { ...prev, autoCheck };
      writePrefs(next);
      return next;
    });
  }, []);

  const setSeenVersion = useCallback((seenVersion: string) => {
    setPrefs((prev) => {
      const next = { ...prev, seenVersion };
      writePrefs(next);
      return next;
    });
  }, []);

  return {
    autoCheck: prefs.autoCheck,
    seenVersion: prefs.seenVersion,
    setAutoCheck,
    setSeenVersion,
  };
}
