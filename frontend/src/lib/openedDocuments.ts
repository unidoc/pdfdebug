/**
 * @file The one path that puts a parsed document into app state and the one
 * path that takes a tab out of it. A re-open of a path already open replaces
 * its tab in place, so the earlier open's backend document is closed here.
 */
import type { Dispatch } from 'react';
import { CloseDocument } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import type { AppAction, TabState } from '../hooks/useDocumentState';

/** Payload of the OPEN_DOCUMENT action. */
export type OpenedDocument = Extract<AppAction, { type: 'OPEN_DOCUMENT' }>['payload'];

/**
 * Module-level map from file path to the tab id of its latest open in this JS
 * session. Updated synchronously on every open, so two opens of the same path
 * that land before a re-render still see each other, and pruned when the tab
 * closes. It survives a re-mount within the same JS context (a dev-mode reload
 * mounts a fresh reducer while the previous session's documents are still
 * open) and is cleared by a true page reload, where drain-on-read already
 * returns an empty drain.
 */
const sessionOpenTabs = new Map<string, string>();

/**
 * Closes the backend document of tabId. A document already gone is not an
 * error; any other failure is logged.
 */
function releaseDocument(tabId: string): void {
  Promise.resolve(CloseDocument(tabId)).catch((err: unknown) => {
    const msg = err instanceof Error ? err.message : String(err);
    // eslint-disable-next-line no-console -- a failed release has no UI surface
    if (!/document not found/i.test(msg)) console.warn(`CloseDocument ${tabId}: ${msg}`);
  });
}

/**
 * Dispatches OPEN_DOCUMENT for doc. A tab id seen before (already a tab, or
 * already recorded as its path's open) is a repeated delivery of the same
 * open: nothing is released and the reducer only activates its tab. Otherwise
 * doc is recorded as the open of its path and the backend document of an
 * earlier open of the same path is closed first. That earlier id comes from
 * the tab holding the path and from the session map (that tab may not be
 * rendered yet, or belong to a previous session). Every caller that adds a
 * parsed file to app state goes through here.
 */
export function dispatchOpenedDocument(dispatch: Dispatch<AppAction>, tabs: readonly Pick<TabState, 'tabId' | 'filePath'>[], doc: OpenedDocument): void {
  const repeated = tabs.some((t) => t.tabId === doc.tabId) || (doc.filePath !== '' && sessionOpenTabs.get(doc.filePath) === doc.tabId);
  if (doc.filePath && !repeated) {
    const earlier = new Set([tabs.find((t) => t.filePath === doc.filePath)?.tabId, sessionOpenTabs.get(doc.filePath)]);
    for (const id of earlier) {
      if (id && id !== doc.tabId) releaseDocument(id);
    }
    sessionOpenTabs.set(doc.filePath, doc.tabId);
  }
  dispatch({ type: 'OPEN_DOCUMENT', payload: doc });
}

/**
 * Closes the tab tabId: dispatches CLOSE_DOCUMENT, forgets the tab as its
 * path's open so a later open of the path does not close it again, and closes
 * its backend document.
 */
export function closeDocumentTab(dispatch: Dispatch<AppAction>, tabId: string): void {
  dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId } });
  for (const [path, id] of sessionOpenTabs) {
    if (id === tabId) sessionOpenTabs.delete(path);
  }
  releaseDocument(tabId);
}
