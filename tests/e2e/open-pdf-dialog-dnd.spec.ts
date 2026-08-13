/**
 * E2E Acceptance Tests for Story 2.4: Open PDF via File Dialog and Drag-and-Drop
 *
 * Only the critical happy path is tested at E2E level. All
 * structural/state/error tests are handled by Go integration tests in
 * tests/open-pdf-dialog-dnd/open_pdf_dialog_dnd_test.go.
 *
 * Test IDs:
 * Run: npx playwright test tests/e2e/open-pdf-dialog-dnd.spec.ts
 */
import { test, expect } from '../support/fixtures';
import { waitForWailsReady } from '../support/helpers/wails-helpers';

test.describe('Open PDF via File Dialog and Drag-and-Drop', () => {
  // ---------------------------------------------------------------------------
  // User opens PDF via drag-and-drop, sees tree Catalog: Given the empty
  // state, When the user drags a PDF onto the window
  //       and releases, Then the PDF is opened and the tree panel shows the
  //       document structure with the root "Catalog" node visible and
  //       auto-expanded to show its immediate children.
  // The root Catalog node is visible and auto-expanded.
  // ---------------------------------------------------------------------------
  test('should open PDF via drag-and-drop and show Catalog in tree', async ({ appPage }) => {
    await waitForWailsReady(appPage);

    // Given: application shows empty state
    await expect(appPage.getByTestId('empty-state')).toBeVisible();

    // When: user drops a PDF file onto the window
    // Wails file drop goes through Go-side OnWindowEvent handler, which emits
    // document:opened event to frontend. Simulate by dropping a test PDF.
    // Note: real file system drag-and-drop requires the Wails runtime and a
    // real file path. In the Playwright context against the dev server, we
    // trigger the drop by dispatching a file drop on the drop target element
    // which has data-file-drop-target attribute.
    const dropZone = appPage.getByTestId('drop-zone');
    await expect(dropZone).toBeVisible();

    // data-file-drop-target is on the root app container (window-wide drop)
    // The drop zone is the visual indicator only.

    // Simulate file drop via Wails runtime -- in a real Wails app, the Go-side
    // handler fires and emits document:opened. For E2E, we verify the UI
    // transition by programmatically emitting the event the way Go would.
    await appPage.evaluate(() => {
      // Simulate the document:opened event that Go backend emits after processing
      // a dropped PDF file. This mirrors the Go-side OnWindowEvent handler flow.
      const event = new CustomEvent('wails:event:document:opened', {
        detail: {
          data: {
            tabId: 'test-tab-001',
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
              {
                id: 'obj:0:2',
                label: 'Pages',
                rawKey: '/Pages',
                nodeType: 'dict',
                valueType: 'reference',
                hasChildren: true,
                childCount: 2,
                iconHint: 'page',
                error: '',
              },
            ],
          },
        },
      });
      window.dispatchEvent(event);
    });

    // Then: empty state disappears
    await expect(appPage.getByTestId('empty-state')).not.toBeVisible({ timeout: 5000 });

    // And: main layout with tree panel is visible
    await expect(appPage.getByTestId('main-layout')).toBeVisible();

    // And: root Catalog node is visible in the left panel
    await expect(appPage.getByTestId('left-panel')).toContainText('Catalog');

    // And: immediate children are visible (auto-expanded)
    await expect(appPage.getByTestId('left-panel')).toContainText('Type');
    await expect(appPage.getByTestId('left-panel')).toContainText('Pages');
  });

  // ---------------------------------------------------------------------------
  // Error banner appears for malformed PDF and is dismissible: Given a PDF that
  // cannot be parsed, When the user opens it,
  //       Then an ErrorBanner appears with a clear message, And the error
  //       banner is dismissible, And the user can open a different file.
  // ---------------------------------------------------------------------------
  test('should show error banner for malformed PDF and allow dismiss', async ({ appPage }) => {
    await waitForWailsReady(appPage);

    // Given: application shows empty state
    await expect(appPage.getByTestId('empty-state')).toBeVisible();

    // When: a malformed PDF triggers a document:error event from Go backend
    await appPage.evaluate(() => {
      const event = new CustomEvent('wails:event:document:error', {
        detail: {
          data: {
            message: 'malformed PDF structure',
          },
        },
      });
      window.dispatchEvent(event);
    });

    // Then: error banner appears
    const errorBanner = appPage.getByTestId('error-banner');
    await expect(errorBanner).toBeVisible({ timeout: 5000 });

    // And: error message is displayed
    const errorMessage = appPage.getByTestId('error-banner-message');
    await expect(errorMessage).toBeVisible();

    // And: error banner has role="alert" for accessibility
    await expect(errorBanner).toHaveAttribute('role', 'alert');

    // And: dismiss button is visible
    const dismissButton = appPage.getByTestId('error-banner-dismiss');
    await expect(dismissButton).toBeVisible();

    // And: empty state is still visible (error does not block UI)
    await expect(appPage.getByTestId('empty-state')).toBeVisible();

    // When: user clicks dismiss
    await dismissButton.click();

    // Then: error banner disappears
    await expect(errorBanner).not.toBeVisible({ timeout: 3000 });

    // And: empty state is still visible -- user can open another file
    await expect(appPage.getByTestId('empty-state')).toBeVisible();
  });
});
