/**
 * E2E Acceptance Tests for Story 4.3: Close Document and Tab Management
 *
 * Test IDs:
 * Run: npx playwright test tests/e2e/close-document.spec.ts
 *
 * This test validates the full close-to-empty-state path: opening a PDF,
 * closing the tab via the close button, and verifying the empty state
 * reappears with the drag-and-drop zone.
 */
import { test, expect } from '../support/fixtures';
import { waitForWailsReady } from '../support/helpers/wails-helpers';

test.describe('Close Document and Tab Management', () => {
  // ---------------------------------------------------------------------------
  // Close last tab returns to empty state: Document is
  // closed, tab is removed from tab bar.
  // When no documents remain open, empty state is shown again with
  //       the drag-and-drop zone, and the user can immediately open a new PDF.
  // ---------------------------------------------------------------------------
  test('should close last tab and return to empty state', async ({ appPage }) => {
    await waitForWailsReady(appPage);

    // Given: application shows empty state
    await expect(appPage.getByTestId('empty-state')).toBeVisible();

    // When: user opens a PDF via simulated Wails event (same pattern as
    // open-pdf-dialog-dnd.spec.ts -- Go backend emits document:opened)
    await appPage.evaluate(() => {
      const event = new CustomEvent('wails:event:document:opened', {
        detail: {
          data: {
            tabId: 'test-close-tab-001',
            fileName: 'minimal.pdf',
            filePath: '/tmp/minimal.pdf',
            pageCount: 1,
            fileSize: 1024,
            rootNode: {
              id: 'root',
              label: 'Catalog',
              rawKey: '',
              nodeType: 'dict',
              valueType: '',
              hasChildren: true,
              childCount: 3,
              iconHint: 'catalog',
              error: '',
            },
            rootChildren: [
              {
                id: 'dict:root:Type',
                label: 'Type',
                rawKey: '/Type',
                nodeType: 'scalar',
                valueType: 'name',
                hasChildren: false,
                childCount: 0,
                iconHint: 'default',
                error: '',
              },
            ],
          },
        },
      });
      window.dispatchEvent(event);
    });

    // Then: tab bar appears with the open document
    await expect(appPage.getByTestId('tab-bar')).toBeVisible({ timeout: 5000 });

    // And: at least one tab is visible
    const tab = appPage.getByTestId('tab-test-close-tab-001');
    await expect(tab).toBeVisible();

    // When: user closes the tab via the close button
    const closeButton = appPage.getByTestId('tab-close-test-close-tab-001');
    await closeButton.click();

    // Then: empty state appears again
    await expect(appPage.getByTestId('empty-state')).toBeVisible({ timeout: 5000 });

    // And: tab bar is not in the DOM (App.jsx conditionally renders TabBar
    // only when tabs.length > 0, so the element is unmounted, not hidden)
    await expect(appPage.getByTestId('tab-bar')).toHaveCount(0);
  });
});
