/**
 * @file Muted note in the detail panel's inner-tab row for an active tab whose
 * file changed on disk or went missing. The only clickable surface of the
 * on-disk change indicator. Subscribes to its own tab's disk status, so a
 * status update re-renders the note and nothing else.
 */
import { useDiskStatus } from '../lib/diskStatusStore';
import { formatTimestamp } from '../lib/formatTimestamp';
import { FILE_MISSING_MESSAGE } from '../lib/fileChange';

/** Props for {@link StaleNote}. */
export interface StaleNoteProps {
  /** Active document tab id. */
  tabId: string | null;
  /** Reloads the active tab from disk. */
  onReload: () => void;
  /** Compares the active tab with the file on disk. */
  onCompare: () => void;
}

const NOTE_BUTTON =
  'flex-shrink-0 min-h-[24px] min-w-[24px] px-1.5 text-xs text-text-secondary rounded hover:bg-surface-hover hover:text-text cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-border-focus';

/**
 * Shows `Document updated` with Reload and Compare for a changed file, or
 * `File not found` with no buttons for a missing one; renders nothing while
 * the file is current. The label truncates first; the buttons stay whole.
 */
export function StaleNote({ tabId, onReload, onCompare }: StaleNoteProps) {
  const status = useDiskStatus(tabId);
  if (!status) return null;
  if (status.state === 'missing') {
    return (
      <div className="ml-auto flex min-w-0 items-center pl-2 pr-2 text-xs text-text-muted" data-testid="stale-note">
        <span className="truncate" title={FILE_MISSING_MESSAGE}>
          File not found
        </span>
      </div>
    );
  }
  // Computed on render so "today" is judged against the clock now.
  const tooltip = `Showing version from ${formatTimestamp(status.loadedModTime)}. Changed on disk ${formatTimestamp(status.diskModTime)}.`;
  return (
    <div className="ml-auto flex min-w-0 items-center pl-2 pr-1 text-xs text-text-muted" data-testid="stale-note">
      <span className="min-w-0 truncate pr-1.5" title={tooltip}>
        Document updated
      </span>
      <span className="flex flex-shrink-0 items-center border-l border-border pl-1">
        <button type="button" className={NOTE_BUTTON} onClick={onReload}>
          Reload
        </button>
      </span>
      <span className="flex flex-shrink-0 items-center border-l border-border pl-1 ml-1">
        <button type="button" className={NOTE_BUTTON} onClick={onCompare}>
          Compare
        </button>
      </span>
    </div>
  );
}
