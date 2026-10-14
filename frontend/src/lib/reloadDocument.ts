/**
 * @file Reload from disk: opens the tab's file again under a new tab id and
 * replaces the tab in place. Shared by every reload trigger.
 */
import type { Dispatch } from 'react';
import { CloseDocument } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import type { AppAction, TabState } from '../hooks/useDocumentState';
import { openPDFFile } from '../hooks/usePDFService';
import { dispatchOpenedDocument } from './openedDocuments';
import { extractErrorMessage } from './extractErrorMessage';
import { isFileBusyMessage } from './fileChange';
import { showReloadToast } from '../components/ReloadToast';

/** Tab ids with a reload in flight; a second trigger for one of them is ignored. */
const reloading = new Set<string>();

/** Short reason for a failed reload, from the extracted backend message. */
export function reloadFailureReason(message: string): string {
  if (/document not found/i.test(message)) return 'file not found';
  if (/encrypted|password/i.test(message)) return 'file is encrypted';
  if (/malformed/i.test(message)) return 'file is damaged or not a valid PDF';
  return 'unexpected error';
}

/** Pill text after a successful reload of fileName. */
export function reloadedMessage(fileName: string): string {
  return `${fileName} reloaded - view reset`;
}

/**
 * Reloads tabId from disk. The tab is replaced only after the new parse fully
 * succeeds. getTabs must return the live tab list: when the tab left it while
 * the parse ran, the new document is closed and nothing changes. A busy file
 * shows the pill; any other failure sets the document error and keeps the old
 * tab untouched.
 */
export async function reloadTab(
  dispatch: Dispatch<AppAction>,
  getTabs: () => readonly TabState[],
  tabId: string,
): Promise<void> {
  if (reloading.has(tabId)) return;
  const tab = getTabs().find((t) => t.tabId === tabId);
  if (!tab || !tab.filePath) return;
  reloading.add(tabId);
  try {
    let result: Awaited<ReturnType<typeof openPDFFile>>;
    try {
      result = await openPDFFile(tab.filePath);
    } catch (err) {
      if (!getTabs().some((t) => t.tabId === tabId)) return;
      const msg = extractErrorMessage(err);
      if (isFileBusyMessage(msg)) {
        showReloadToast(`${tab.fileName} is in use by another app - try again`);
        return;
      }
      dispatch({ type: 'SET_DOCUMENT_ERROR', payload: { message: `Could not reload ${tab.fileName}: ${reloadFailureReason(msg)}` } });
      return;
    }
    if (!getTabs().some((t) => t.tabId === tabId)) {
      CloseDocument(result.tabId).catch(() => {});
      return;
    }
    dispatchOpenedDocument(dispatch, getTabs(), {
      tabId: result.tabId,
      fileName: result.fileName,
      filePath: result.filePath,
      pageCount: result.pageCount,
      rootNode: result.rootNode,
      rootChildren: result.rootChildren,
      reload: true,
    });
    if (result.warning) {
      dispatch({ type: 'SET_DOCUMENT_WARNING', payload: { message: result.warning } });
    }
    showReloadToast(reloadedMessage(result.fileName || tab.fileName));
  } finally {
    reloading.delete(tabId);
  }
}
