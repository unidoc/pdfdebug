/**
 * @file Reload from disk: opens the tab's file again under a new tab id and
 * replaces the tab in place. Shared by every reload trigger.
 */
import type { Dispatch } from 'react';
import { CloseDocument } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import type { AppAction, TabState } from '../hooks/useDocumentState';
import { openErrorKind, openPDFFile } from '../hooks/usePDFService';
import { dispatchOpenedDocument } from './openedDocuments';
import { extractErrorMessage } from './extractErrorMessage';
import { isFileBusyMessage } from './fileChange';
import { showReloadToast } from '../components/ReloadToast';
import { markTabFromDiskCheck } from './diskCheck';

/** Tab ids with a reload in flight; a second trigger for one of them is ignored. */
const reloading = new Set<string>();

/** Short reason for a failed reload, from the extracted backend message. */
export function reloadFailureReason(message: string): string {
  switch (openErrorKind(message)) {
    case 'notFound':
      return 'file not found';
    case 'encrypted':
      return 'file is encrypted';
    case 'malformed':
      return 'file is damaged or not a valid PDF';
    default:
      return 'unexpected error';
  }
}

/** Reloads activeTabId when there is one: the shared path of every reload trigger. */
export function reloadActiveTab(
  dispatch: Dispatch<AppAction>,
  getTabs: () => readonly TabState[],
  activeTabId: string | null,
): void {
  if (activeTabId) void reloadTab(dispatch, getTabs, activeTabId);
}

/** Pill text after a successful reload of fileName. */
export function reloadedMessage(fileName: string): string {
  return `${fileName} reloaded - view reset`;
}

/**
 * Reloads tabId from disk. The tab is replaced only after the new parse fully
 * succeeds. getTabs must return the live tab list: when the tab left it while
 * the parse ran, the new document is closed and nothing changes. A reload
 * that lands after the user switched tabs replaces the tab without taking
 * focus. A busy file shows the pill; any other failure sets the document error
 * and keeps the old tab untouched.
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
      dispatch({ type: 'SET_DOCUMENT_ERROR', payload: { message: `Could not reload ${tab.fileName}: ${reloadFailureReason(msg)}`, reload: true } });
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
      warning: result.warning ?? null,
    });
    // The file can change again while the parse runs; the watcher may already
    // have reported that for the new id before its tab existed.
    markTabFromDiskCheck(result.tabId);
    showReloadToast(reloadedMessage(result.fileName || tab.fileName));
  } finally {
    reloading.delete(tabId);
  }
}
