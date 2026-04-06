/**
 * @file PDF service layer. Wraps Wails-generated bindings with
 * user-friendly error mapping and a structured open-file workflow.
 */
import { OpenFile, GetTreeRoot, GetChildren, CloseDocument, OpenFileDialog as _OpenFileDialog } from '../../bindings/unipdf-debugger/internal/pdfservice/pdfservice.js';
import type { TreeNode } from './useDocumentState';

/**
 * Map raw backend error strings to user-facing messages.
 * Matches common failure categories (encrypted, malformed, not found).
 */
export function mapErrorMessage(rawMessage: string): string {
  if (/encrypted|password/i.test(rawMessage)) {
    return 'This PDF could not be opened. The file appears to be encrypted.';
  }
  if (/malformed/i.test(rawMessage)) {
    return 'This PDF could not be opened. The file appears to be damaged or corrupt.';
  }
  if (/not found/i.test(rawMessage)) {
    return 'The file could not be found. It may have been moved or deleted.';
  }
  return 'This PDF could not be opened. An unexpected error occurred.';
}

/** Show the native OS file picker and return the selected path (or empty string). */
export async function openFileDialog(): Promise<string> {
  return _OpenFileDialog();
}

/** Result of opening a PDF: document metadata plus the pre-fetched tree root. */
export interface OpenPDFResult {
  tabId: string;
  fileName: string;
  filePath: string;
  pageCount: number;
  fileSize: number;
  rootNode: TreeNode | null;
  rootChildren: TreeNode[] | null;
  warning: string | null;
}

/**
 * Open a PDF file at the given path, fetch its tree root and first-level
 * children, and return a structured result. Cleans up the backend document
 * if tree fetching fails to avoid resource leaks.
 */
export async function openPDFFile(path: string): Promise<OpenPDFResult> {
  if (!path) {
    throw new Error('No file path provided');
  }
  const docInfo = await OpenFile(path);
  if (!docInfo) {
    throw new Error('Failed to open PDF file');
  }
  let rootNode: TreeNode | null = null;
  let rootChildren: TreeNode[] | null = null;
  try {
    rootNode = await GetTreeRoot(docInfo.tabId) as TreeNode | null;
    rootChildren = (await GetChildren(docInfo.tabId, 'root')) as TreeNode[] | null;
  } catch (err) {
    // Clean up the backend document so it doesn't leak
    CloseDocument(docInfo.tabId).catch(() => {});
    throw err;
  }
  return {
    tabId: docInfo.tabId,
    fileName: docInfo.fileName,
    filePath: docInfo.filePath,
    pageCount: docInfo.pageCount,
    fileSize: docInfo.fileSize,
    rootNode,
    rootChildren,
    warning: docInfo.error || null,
  };
}

