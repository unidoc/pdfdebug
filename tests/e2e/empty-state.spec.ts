/**
 * E2E Acceptance Tests for Story 1.3: Empty State with Drag-and-Drop Zone
 *
 * TDD GREEN PHASE: Implementation is complete. Tests un-skipped for validation.
 *
 * These E2E tests cover acceptance criteria that genuinely require full browser
 * interaction (drag-and-drop visual feedback, timeout behavior). Component
 * structure and source-level validation is handled by Go integration tests
 * in tests/empty-state/empty_state_test.go.
 *
 * Test IDs: 1.3-E2E-001, 1.3-E2E-002, 1.3-E2E-003, 1.3-E2E-004
 * Run: npx playwright test tests/e2e/empty-state.spec.ts
 */
import { test, expect } from '../support/fixtures';
import { waitForWailsReady } from '../support/helpers/wails-helpers';

test.describe('Story 1.3: Empty State with Drag-and-Drop Zone (ATDD)', () => {
  // ---------------------------------------------------------------------------
  // 1.3-E2E-001 (P0): Application launches and displays empty state
  // AC#1: Centered empty state with title and subtitle
  // AC#2: Drop zone with "Drop a PDF file here" text
  // AC#3: "Open File..." button visible
  // AC#4: Platform-aware shortcut hint displayed
  // ---------------------------------------------------------------------------
  test('[P0] should display empty state with all elements on launch', async ({ appPage }) => {
    await waitForWailsReady(appPage);

    // AC#1: Title and subtitle visible
    await expect(
      appPage.getByTestId('empty-state'),
    ).toBeVisible();

    await expect(
      appPage.getByTestId('empty-state-title'),
    ).toHaveText('UniDoc PDF Debugger');

    await expect(
      appPage.getByTestId('empty-state-subtitle'),
    ).toHaveText('Inspect PDF internal structure');

    // AC#2: Drop zone visible with hint text
    await expect(
      appPage.getByTestId('drop-zone'),
    ).toBeVisible();

    await expect(
      appPage.getByTestId('drop-zone-hint'),
    ).toContainText('Drop a PDF file here');

    // AC#3: Open File button visible
    await expect(
      appPage.getByTestId('open-file-button'),
    ).toBeVisible();

    await expect(
      appPage.getByTestId('open-file-button'),
    ).toHaveText('Open File...');

    // AC#4: Shortcut hint visible (Cmd+O on macOS, Ctrl+O elsewhere)
    await expect(
      appPage.getByTestId('shortcut-hint'),
    ).toBeVisible();

    await expect(
      appPage.getByTestId('shortcut-hint'),
    ).toContainText(/(?:Cmd|Ctrl)\+O/);
  });

  // ---------------------------------------------------------------------------
  // 1.3-E2E-002 (P1): Drop zone highlights on drag-over
  // AC#6: Dragging file over window highlights drop zone with blue border
  //       and background highlight
  // ---------------------------------------------------------------------------
  test('[P1] should highlight drop zone when file is dragged over window', async ({ appPage }) => {
    await waitForWailsReady(appPage);

    const dropZone = appPage.getByTestId('drop-zone');

    // Verify default state: no blue border highlight
    await expect(dropZone).toBeVisible();

    // Simulate dragenter on the empty state container (entire window area)
    const emptyState = appPage.getByTestId('empty-state');
    await emptyState.dispatchEvent('dragenter', {
      dataTransfer: { items: [], types: [] },
    });

    // AC#6: Drop zone border should turn blue (border-border-focus applies)
    // We check for the visual change by verifying the CSS class was applied
    // The exact assertion depends on how Tailwind applies the class dynamically
    await expect(dropZone).toHaveCSS('border-color', 'rgb(59, 130, 246)'); // Blue 500 = #3b82f6

    // Simulate dragleave to reset
    await emptyState.dispatchEvent('dragleave');

    // Border should return to default
    await expect(dropZone).not.toHaveCSS('border-color', 'rgb(59, 130, 246)');
  });

  // ---------------------------------------------------------------------------
  // 1.3-E2E-003 (P1): Non-PDF drop shows "PDF files only" error for 2 seconds
  // AC#7: Drop a non-PDF file -> "PDF files only" hint in red for 2s -> reset
  // ---------------------------------------------------------------------------
  test('[P1] should show error hint for 2 seconds when non-PDF file is dropped', async ({ appPage }) => {
    await waitForWailsReady(appPage);

    const dropZoneHint = appPage.getByTestId('drop-zone-hint');

    // Verify default state shows normal hint text
    await expect(dropZoneHint).toContainText('Drop a PDF file here');

    // Simulate dropping a non-PDF file on the empty state container
    const emptyState = appPage.getByTestId('empty-state');

    // Create a DataTransfer-like object with a non-PDF file
    await appPage.evaluate(() => {
      const dropEvent = new DragEvent('drop', {
        bubbles: true,
        cancelable: true,
      });

      // Create a mock file list with a non-PDF file
      const file = new File(['content'], 'document.txt', { type: 'text/plain' });
      const dataTransfer = new DataTransfer();
      dataTransfer.items.add(file);

      Object.defineProperty(dropEvent, 'dataTransfer', {
        value: dataTransfer,
      });

      const target = document.querySelector('[data-testid="empty-state"]');
      if (target) {
        target.dispatchEvent(dropEvent);
      }
    });

    // AC#7: "PDF files only" hint should appear in error color
    await expect(dropZoneHint).toContainText('PDF files only');

    // AC#7: After 2 seconds, hint should reset to default
    // Wait slightly more than 2 seconds to account for timing
    await appPage.waitForTimeout(2500);

    await expect(dropZoneHint).toContainText('Drop a PDF file here');
  });

  // ---------------------------------------------------------------------------
  // 1.3-E2E-004 (P1): Open File button click logs to console
  // AC#8: Clicking button without onOpenFile prop logs to console
  // ---------------------------------------------------------------------------
  test('[P1] should log to console when Open File button is clicked without handler', async ({ appPage }) => {
    await waitForWailsReady(appPage);

    // Capture console messages
    const consoleMessages: string[] = [];
    appPage.on('console', (msg) => {
      consoleMessages.push(msg.text());
    });

    // AC#8: Click the Open File button
    await appPage.getByTestId('open-file-button').click();

    // AC#8: Since App.jsx passes no onOpenFile prop, clicking should log to console
    expect(consoleMessages.some((msg) => msg.toLowerCase().includes('open file'))).toBe(true);
  });
});
