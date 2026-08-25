/**
 * E2E Acceptance Tests for the Empty State with Drag-and-Drop Zone
 *
 * TDD GREEN PHASE: Implementation is complete. Tests un-skipped for validation.
 *
 * These E2E tests cover acceptance criteria that genuinely require full browser
 * interaction (drag-and-drop visual feedback, timeout behavior). Component
 * structure and source-level validation is handled by Go integration tests
 * in tests/empty-state/empty_state_test.go.
 *
 * Run: npx playwright test tests/e2e/empty-state.spec.ts
 */
import { test, expect } from '../support/fixtures';
import { waitForWailsReady } from '../support/helpers/wails-helpers';

test.describe('Empty State with Drag-and-Drop Zone', () => {
  // ---------------------------------------------------------------------------
  // Application launches and displays the empty state, which shows:
  //   - a centered block with title and subtitle
  //   - a drop zone reading "Drop a PDF file here"
  //   - an "Open File..." button
  //   - a platform-aware shortcut hint
  // ---------------------------------------------------------------------------
  test('should display empty state with all elements on launch', async ({ appPage }) => {
    await waitForWailsReady(appPage);

    // Title and subtitle visible
    await expect(
      appPage.getByTestId('empty-state'),
    ).toBeVisible();

    await expect(
      appPage.getByTestId('empty-state-title'),
    ).toHaveText('UniDoc PDF Debugger');

    await expect(
      appPage.getByTestId('empty-state-subtitle'),
    ).toHaveText('Inspect PDF internal structure');

    // Drop zone visible with hint text
    await expect(
      appPage.getByTestId('drop-zone'),
    ).toBeVisible();

    await expect(
      appPage.getByTestId('drop-zone-hint'),
    ).toContainText('Drop a PDF file here');

    // Open File button visible
    await expect(
      appPage.getByTestId('open-file-button'),
    ).toBeVisible();

    await expect(
      appPage.getByTestId('open-file-button'),
    ).toHaveText('Open File...');

    // Shortcut hint visible (Cmd+O on macOS, Ctrl+O elsewhere)
    await expect(
      appPage.getByTestId('shortcut-hint'),
    ).toBeVisible();

    await expect(
      appPage.getByTestId('shortcut-hint'),
    ).toContainText(/(?:Cmd|Ctrl)\+O/);
  });

  // ---------------------------------------------------------------------------
  // Drop zone highlights on drag-over: Dragging file over window
  // highlights drop zone with blue border
  // and background highlight
  // ---------------------------------------------------------------------------
  test('should highlight drop zone when file is dragged over window', async ({ appPage }) => {
    await waitForWailsReady(appPage);

    const dropZone = appPage.getByTestId('drop-zone');

    // Verify default state: no blue border highlight
    await expect(dropZone).toBeVisible();

    // Simulate dragenter on the empty state container (entire window area)
    const emptyState = appPage.getByTestId('empty-state');
    await emptyState.dispatchEvent('dragenter', {
      dataTransfer: { items: [], types: [] },
    });

    // Drop zone border should turn blue (border-border-focus applies) We check
    // for the visual change by verifying the CSS class was applied The exact
    // assertion depends on how Tailwind applies the class dynamically
    await expect(dropZone).toHaveCSS('border-color', 'rgb(59, 130, 246)'); // Blue 500 = #3b82f6

    // Simulate dragleave to reset
    await emptyState.dispatchEvent('dragleave');

    // Border should return to default
    await expect(dropZone).not.toHaveCSS('border-color', 'rgb(59, 130, 246)');
  });

  // ---------------------------------------------------------------------------
  // Non-PDF drop shows "PDF files only" error for 2 seconds: Drop a non-PDF
  // file -> "PDF files only" hint in red for 2s -> reset
  // ---------------------------------------------------------------------------
  test('should show error hint for 2 seconds when non-PDF file is dropped', async ({ appPage }) => {
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

    // "PDF files only" hint should appear in error color
    await expect(dropZoneHint).toContainText('PDF files only');

    // After 2 seconds, hint should reset to default Use Playwright
    // auto-waiting instead of hard timeout for determinism
    await expect(dropZoneHint).toContainText('Drop a PDF file here', { timeout: 5000 });
  });

  // ---------------------------------------------------------------------------
  // Open File button click logs to console: Clicking button
  // without onOpenFile prop logs to console
  // ---------------------------------------------------------------------------
  test('should log to console when Open File button is clicked without handler', async ({ appPage }) => {
    await waitForWailsReady(appPage);

    // Capture console messages
    const consoleMessages: string[] = [];
    appPage.on('console', (msg) => {
      consoleMessages.push(msg.text());
    });

    // Click the Open File button
    await appPage.getByTestId('open-file-button').click();

    // Since App.jsx passes no onOpenFile prop, clicking should log to console
    expect(consoleMessages.some((msg) => msg.toLowerCase().includes('open file'))).toBe(true);
  });
});
