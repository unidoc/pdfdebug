/**
 * @file On-demand disk check that marks a tab at once.
 */
import { GetDiskStatus } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { diskStatusSeq, setDiskStatusFromCheck, type DiskStatusInfo } from './diskStatusStore';

/**
 * Asks the backend for tabId's disk status and writes it to the store, so a
 * refused read marks the tab at once (or clears it when the file went back).
 * A watcher event that arrives first wins over the result. Resolves with the
 * status, or rejects with the binding's error.
 */
export function checkDiskStatus(tabId: string): Promise<DiskStatusInfo | null> {
  const seqAtRequest = diskStatusSeq(tabId);
  return Promise.resolve(GetDiskStatus(tabId)).then((res: unknown) => {
    const st = (res ?? null) as DiskStatusInfo | null;
    if (st) setDiskStatusFromCheck(tabId, st, seqAtRequest);
    return st;
  });
}

/** checkDiskStatus with failures ignored: the watcher reports the change later. */
export function markTabFromDiskCheck(tabId: string): void {
  checkDiskStatus(tabId).catch(() => {});
}
