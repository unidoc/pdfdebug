/**
 * @file The one path that puts a parsed document into app state. A re-open of
 * a path already open replaces its tab in place, so the earlier open's backend
 * document is closed here.
 */
import type { Dispatch } from 'react';
import { CloseDocument } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import type { AppAction, TabState } from '../hooks/useDocumentState';

/** Payload of the OPEN_DOCUMENT action. */
export type OpenedDocument = Extract<AppAction, { type: 'OPEN_DOCUMENT' }>['payload'];

/**
 * Module-level map from file path to the tab id of its latest open in this JS
 * session. Updated synchronously on every open, so two opens of the same path
 * that land before a re-render still see each other. It survives a re-mount
 * within the same JS context (a dev-mode reload mounts a fresh reducer while
 * the previous session's documents are still open) and is cleared by a true
 * page reload, where drain-on-read already returns an empty drain.
 */
const sessionOpenTabs = new Map<string, string>();

/**
 * Records tabId as the open of filePath and closes the backend document of
 * the earlier open of the same path. The earlier id comes from the tab in tabs
 * holding the path and from the session map (that tab may not be rendered
 * yet, or belong to a previous session). An id whose tab was already closed
 * gets a no-op close.
 */
function releaseReplacedDocument(filePath: string, tabId: string, tabs: readonly Pick<TabState, 'tabId' | 'filePath'>[]): void {
  if (!filePath) return;
  const earlier = new Set([tabs.find((t) => t.filePath === filePath)?.tabId, sessionOpenTabs.get(filePath)]);
  for (const id of earlier) {
    if (id && id !== tabId) Promise.resolve(CloseDocument(id)).catch(() => {});
  }
  sessionOpenTabs.set(filePath, tabId);
}

/**
 * Dispatches OPEN_DOCUMENT for doc after releasing the backend document of an
 * earlier open of the same path. Every caller that adds a parsed file to app
 * state goes through here.
 */
export function dispatchOpenedDocument(dispatch: Dispatch<AppAction>, tabs: readonly Pick<TabState, 'tabId' | 'filePath'>[], doc: OpenedDocument): void {
  releaseReplacedDocument(doc.filePath, doc.tabId, tabs);
  dispatch({ type: 'OPEN_DOCUMENT', payload: doc });
}
