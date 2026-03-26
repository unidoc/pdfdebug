/**
 * Application-level fixture for UniPDF Debugger.
 *
 * Provides helpers scoped to the Wails desktop application context.
 * The Wails dev server exposes the app at BASE_URL (default http://localhost:34115).
 */
import { test as base, type Page } from '@playwright/test';

export type AppFixtures = {
  /** Navigate to the app and wait for it to be ready. */
  appPage: Page;
};

export const test = base.extend<AppFixtures>({
  appPage: async ({ page }, use) => {
    await page.goto('/');
    // Wait for the Wails app shell to render
    await page.waitForLoadState('domcontentloaded');
    await use(page);
  },
});
