/**
 * @file Backend error markers for a file that changed on disk or is busy, and
 * the UI strings shown for them.
 */

/** Text carried by pdfcore.ErrFileChanged. */
export const FILE_CHANGED_MARKER = 'file changed on disk';

/** Text carried by pdfcore.ErrFileBusy. */
export const FILE_BUSY_MARKER = 'file is busy';

/** Shown in place of a view whose read was refused because the file changed. */
export const FILE_CHANGED_MESSAGE = 'File changed on disk. Reload to view.';

/** Shown in place of a view whose read was refused because another app has the file locked. */
export const FILE_BUSY_MESSAGE = 'File is in use by another app. Reload to try again.';

/** Shown when the file is no longer at its path. */
export const FILE_MISSING_MESSAGE = 'File no longer found at this path';

/** True when an extracted backend error message carries the file-changed marker. */
export function isFileChangedMessage(message: string): boolean {
  return message.includes(FILE_CHANGED_MARKER);
}

/** True when an extracted backend error message carries the busy marker. */
export function isFileBusyMessage(message: string): boolean {
  return message.includes(FILE_BUSY_MARKER);
}
