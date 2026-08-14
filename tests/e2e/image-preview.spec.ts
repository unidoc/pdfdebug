/**
 * E2E Acceptance Test for Image Preview in the Detail Panel
 *
 * Only the critical full-stack happy path is tested at E2E level. All
 * component/state/loading/error tests are handled by Vitest in
 * frontend/src/components/ImagePreview.test.tsx and
 * frontend/src/components/DetailPanel.test.tsx.
 *
 * Test IDs:
 * Run: npx playwright test tests/e2e/image-preview.spec.ts
 */
import { test, expect } from '../support/fixtures';
import { waitForWailsReady } from '../support/helpers/wails-helpers';

test.describe('Image Preview in Detail Panel', () => {
  // ---------------------------------------------------------------------------
  // User selects XObject image node in tree, sees image
  //                    preview rendered in the detail panel with metadata.
  // Given an XObject image node is selected in the tree, When the
  //       DetailPanel updates, Then it switches to image preview mode showing
  //       the rendered image, And image metadata is displayed below the image.
  // ---------------------------------------------------------------------------
  test('should show image preview when selecting an XObject image node', async ({ appPage }) => {
    await waitForWailsReady(appPage);

    // Given: application shows empty state
    await expect(appPage.getByTestId('empty-state')).toBeVisible();

    // When: a PDF with an image XObject is opened (simulate via backend event)
    await appPage.evaluate(() => {
      const event = new CustomEvent('wails:event:document:opened', {
        detail: {
          data: {
            tabId: 'test-tab-image',
            fileName: 'image-xobject.pdf',
            filePath: '/tmp/image-xobject.pdf',
            pageCount: 1,
            fileSize: 5000,
            rootNode: {
              id: 'root',
              label: 'Catalog',
              rawKey: '',
              nodeType: 'dict',
              valueType: '',
              hasChildren: true,
              childCount: 2,
              iconHint: 'catalog',
              error: '',
            },
            rootChildren: [
              {
                id: 'dict:root:Pages',
                label: 'Pages',
                rawKey: '/Pages',
                nodeType: 'dict',
                valueType: 'reference',
                hasChildren: true,
                childCount: 1,
                iconHint: 'page',
                error: '',
              },
              {
                id: 'obj:0:20',
                label: 'Image1',
                rawKey: '/Image1',
                nodeType: 'stream',
                valueType: '',
                hasChildren: false,
                childCount: 0,
                iconHint: 'image',
                error: '',
              },
            ],
          },
        },
      });
      window.dispatchEvent(event);
    });

    // Then: main layout appears with tree panel
    await expect(appPage.getByTestId('main-layout')).toBeVisible({ timeout: 5000 });
    await expect(appPage.getByTestId('tree-panel')).toBeVisible();

    // And: the image node is visible in the tree
    const imageNode = appPage.locator(
      '[data-testid="tree-node"][data-node-id="obj:0:20"]'
    );
    await expect(imageNode).toBeVisible();

    // When: user clicks the image node
    await imageNode.click();

    // Then: the detail panel shows image preview mode
    // Wait for the image to render (full IPC round-trip: tree selection ->
    // Go GetImageData -> base64 transfer -> React rendering)
    await expect(
      appPage.getByTestId('image-preview-img')
    ).toBeVisible({ timeout: 15000 });

    // And: the detail panel header shows "Image Preview"
    const header = appPage.getByTestId('detail-panel-header');
    await expect(header).toContainText('Image Preview');

    // And: image metadata is displayed below the image
    const metadata = appPage.getByTestId('image-preview-metadata');
    await expect(metadata).toBeVisible();
    // Metadata should contain dimension text (e.g., "320 x 240 px")
    await expect(metadata).toContainText(/\d+ x \d+ px/);
  });
});
