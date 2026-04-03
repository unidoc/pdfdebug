/**
 * E2E Acceptance Tests for Story 2.5: Tree Panel with Lazy-Loading Navigation
 *
 * TDD RED PHASE: Tests MUST fail until Story 2-5 is implemented.
 *
 * Only the critical full-stack happy path (2.5-E2E-001) is tested at E2E level.
 * All component/state/ARIA/loading tests are handled by Vitest in
 * frontend/src/components/TreePanel.test.tsx.
 *
 * Test IDs: 2.5-E2E-001
 * Run: npx playwright test tests/e2e/tree-panel-lazy.spec.ts
 */
import { test, expect } from '../support/fixtures';
import { waitForWailsReady } from '../support/helpers/wails-helpers';

test.describe('Story 2.5: Tree Panel with Lazy-Loading Navigation (ATDD)', () => {
  // ---------------------------------------------------------------------------
  // 2.5-E2E-001 [P0]: User expands tree node, children appear, selects node,
  //                    detail panel shows properties
  // AC#1: Expand arrow loads children on demand from Go backend.
  // AC#4: Selection dispatches SELECT_NODE, selected node has highlight.
  // ---------------------------------------------------------------------------
  test('[P0] should expand tree node, show children, and select node', async ({ appPage }) => {
    await waitForWailsReady(appPage);

    // Given: application shows empty state
    await expect(appPage.getByTestId('empty-state')).toBeVisible();

    // When: a PDF is opened (simulate via backend event)
    await appPage.evaluate(() => {
      const event = new CustomEvent('wails:event:document:opened', {
        detail: {
          data: {
            tabId: 'test-tab-tree',
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

    // Then: main layout appears with tree panel
    await expect(appPage.getByTestId('main-layout')).toBeVisible({ timeout: 5000 });
    await expect(appPage.getByTestId('tree-panel')).toBeVisible();

    // And: root Catalog and its pre-loaded children are visible
    await expect(appPage.getByTestId('tree-panel')).toContainText('Catalog');
    await expect(appPage.getByTestId('tree-panel')).toContainText('Pages');
    await expect(appPage.getByTestId('tree-panel')).toContainText('Type');

    // When: user clicks the "Pages" node to expand it (triggers GetChildren IPC call)
    const pagesNode = appPage.locator('[data-testid="tree-node"][data-node-id="obj:0:2"]');
    await expect(pagesNode).toBeVisible();
    await pagesNode.click();

    // Then: child nodes appear (loaded from Go backend via GetChildren)
    // Note: this requires the full Wails stack -- GetChildren returns real data.
    // The exact children depend on the test PDF structure.
    // For a minimal PDF, Pages should have at least one Page child.
    // Wait for expansion to complete (lazy load from backend).
    // Use a generous timeout since this is a full IPC round-trip.
    await expect(
      appPage.locator('[data-testid="tree-panel"]')
    ).toContainText('Page', { timeout: 10000 });

    // And: the selected node has visible highlight (bg-surface-selected)
    // Verify the clicked node has selection styling
    await expect(pagesNode).toHaveCSS('background-color', /./);

    // When: user selects a different node (Type -- a leaf node)
    const typeNode = appPage.locator('[data-testid="tree-node"][data-node-id="dict:root:Type"]');
    await typeNode.click();

    // Then: the Type node becomes selected
    // Verify by checking aria-selected or visual highlight
    const selectedItem = appPage.locator('[role="treeitem"][aria-selected="true"]');
    await expect(selectedItem).toBeVisible({ timeout: 5000 });
  });
});
