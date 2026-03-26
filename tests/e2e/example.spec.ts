/**
 * Example E2E test for UniPDF Debugger.
 *
 * Demonstrates:
 * - Fixture usage (appPage)
 * - data-testid selector strategy
 * - Given/When/Then structure
 * - Factory usage for test data expectations
 */
import { test, expect } from '../support/fixtures';
import { waitForWailsReady } from '../support/helpers/wails-helpers';

test.describe('Application Shell', () => {
  test('displays empty state on launch', async ({ appPage }) => {
    // Given: the application has launched
    await waitForWailsReady(appPage);

    // When: no document is open
    // (initial state)

    // Then: the empty state guidance is shown
    await expect(
      appPage.locator('[data-testid="empty-state"]'),
    ).toBeVisible();

    await expect(
      appPage.locator('[data-testid="empty-state-message"]'),
    ).toContainText(/open.*pdf/i);
  });

  test('shows the document structure tree panel', async ({ appPage }) => {
    // Given: the application has launched
    await waitForWailsReady(appPage);

    // Then: the structure tree panel exists (may be empty)
    await expect(
      appPage.locator('[data-testid="structure-tree-panel"]'),
    ).toBeVisible();
  });
});
