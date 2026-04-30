# UniDoc PDF Debugger - Test Framework

End-to-end test framework for the UniDoc PDF Debugger desktop application, built with Playwright targeting Chromium.

## Setup

```bash
# Install Node.js (version specified in .nvmrc)
nvm use

# Install dependencies (run from project root, once package.json exists)
npm install

# Install Playwright browsers (Chromium only by default)
npx playwright install chromium --with-deps

# Copy environment config
cp .env.example .env
```

## Running Tests

```bash
# Run all E2E tests (headless, Chromium)
npm run test:e2e

# Run in headed mode (see the browser)
npx playwright test --headed

# Run in debug mode (step through tests)
npx playwright test --debug

# Run a specific test file
npx playwright test tests/e2e/example.spec.ts

# View the HTML test report
npx playwright show-report
```

## Architecture

```
tests/
  e2e/                          # E2E test specs
    example.spec.ts             # Sample test demonstrating patterns
  support/
    fixtures/                   # Playwright fixtures (composable)
      index.ts                  # Merged fixture index (import from here)
      app-fixture.ts            # App-level fixture (appPage)
      factories/                # Data factory functions
        pdf-document-factory.ts # PDF document test data factories
    helpers/                    # Pure helper functions
      wails-helpers.ts          # Wails desktop app interaction helpers
    page-objects/               # Page object modules (optional)
```

### Key Patterns

- **Fixtures**: Composable via `mergeTests`. Import `{ test, expect }` from `tests/support/fixtures/index.ts`.
- **Factories**: Use `createPdfDocumentInfo()` and `createPdfObjectNode()` with overrides for test data.
- **Helpers**: Pure functions (`waitForWailsReady`, `expandTreeNode`, `selectTreeNode`) -- framework-agnostic where possible.
- **Selectors**: Always use `data-testid` attributes for resilient selectors.

### Test Structure

Tests follow the Given/When/Then pattern:

```typescript
test('meaningful description', async ({ appPage }) => {
  // Given: setup / preconditions
  await waitForWailsReady(appPage);

  // When: action under test
  await appPage.click('[data-testid="open-file-button"]');

  // Then: assertions
  await expect(appPage.locator('[data-testid="document-tab"]')).toBeVisible();
});
```

## Browser Targets

**Default: Chromium only.** This is a desktop application rendered in an OS WebView. Multi-browser testing (Firefox, WebKit) is not enabled by default and should be configured per-project based on audience needs.

To add additional browsers, uncomment entries in `playwright.config.ts` under `projects`.

## Best Practices

- Use `data-testid` selectors -- never CSS classes or DOM structure
- Seed data via factories, not hardcoded values
- No `page.waitForTimeout()` -- use event-based waits
- Keep tests under 300 lines and under 1.5 minutes
- Each test should be independent and self-cleaning
- Network interception should happen before navigation

## CI Integration

Tests are configured for CI with:

- `forbidOnly: true` to prevent `.only()` from blocking pipelines
- `retries: 2` for stability
- `workers: 1` for deterministic execution
- JUnit XML output at `test-results/results.xml`
- HTML report at `playwright-report/`
- Artifacts (screenshots, video, trace) captured on failure

## Knowledge Base References

- `fixture-architecture.md` -- Composable fixture patterns
- `data-factories.md` -- Factory functions with overrides
- `playwright-config.md` -- Configuration guardrails
- `test-quality.md` -- Definition of done for tests
- `network-first.md` -- Network interception patterns
