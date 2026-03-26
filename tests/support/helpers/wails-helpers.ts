/**
 * Wails-specific test helpers for UniPDF Debugger.
 *
 * Pure functions that wrap common interactions with the Wails desktop app
 * running in a browser context via Playwright.
 */
import type { Page } from '@playwright/test';

/**
 * Wait for the Wails runtime to be fully initialized.
 * Wails injects a global `runtime` object once the Go backend is ready.
 */
export async function waitForWailsReady(page: Page, timeoutMs = 10_000): Promise<void> {
  await page.waitForFunction(
    () => typeof (window as Record<string, unknown>).runtime !== 'undefined',
    { timeout: timeoutMs },
  );
}

/**
 * Open a PDF file using the Wails file dialog simulation.
 * In E2E tests against the dev server, file open is triggered via
 * the application UI rather than native OS dialogs.
 */
export async function openPdfViaUI(page: Page, fileName: string): Promise<void> {
  // Trigger the file open action through the app menu or button
  await page.click('[data-testid="open-file-button"]');
  // The actual file dialog interaction depends on Wails test mode configuration
  // This is a placeholder for the real implementation once the app is built
  await page.waitForSelector(`[data-testid="document-tab"][data-filename="${fileName}"]`);
}

/**
 * Expand a tree node in the document structure panel.
 */
export async function expandTreeNode(page: Page, nodeLabel: string): Promise<void> {
  const node = page.locator(`[data-testid="tree-node"][data-label="${nodeLabel}"]`);
  const expander = node.locator('[data-testid="tree-expander"]');
  await expander.click();
  await node.locator('[data-testid="tree-children"]').waitFor({ state: 'visible' });
}

/**
 * Select a node in the document structure tree and wait for the
 * property panel to update.
 */
export async function selectTreeNode(page: Page, nodeLabel: string): Promise<void> {
  const node = page.locator(`[data-testid="tree-node"][data-label="${nodeLabel}"]`);
  await node.click();
  await page.waitForSelector('[data-testid="property-panel"][data-loaded="true"]');
}
