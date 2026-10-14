/**
 * @file On-demand disk check that marks a tab at once.
 */
import { GetDiskStatus } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { setDiskStatusFromCheck, type DiskStatusInfo } from './diskStatusStore';

/**
 * Asks the backend for tabId's disk status and writes it to the store, so a
 * refused read marks the tab at once (or clears it when the file went back).
 * Failures are ignored: the watcher reports the change later.
 */
export function markTabFromDiskCheck(tabId: string): void {
  Promise.resolve(GetDiskStatus(tabId))
    .then((st: unknown) => {
      if (st) setDiskStatusFromCheck(tabId, st as DiskStatusInfo);
    })
    .catch(() => {});
}
