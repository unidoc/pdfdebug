import { OpenFile, GetTreeRoot, GetChildren, CloseDocument, OpenFileDialog as _OpenFileDialog } from '../../bindings/unipdf-debugger/internal/pdfservice/pdfservice.js';
import type { TreeNode } from './useDocumentState';

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

export async function openFileDialog(): Promise<string> {
  return _OpenFileDialog();
}

export interface OpenPDFResult {
  tabId: string;
  fileName: string;
  filePath: string;
  pageCount: number;
  fileSize: number;
  rootNode: TreeNode | null;
  rootChildren: TreeNode[] | null;
}

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
  };
}

