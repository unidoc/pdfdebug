import { defineConfig, devices } from '@playwright/test';
import * as path from 'path';

/**
 * Playwright configuration for UniDOC PDF Debugger E2E tests.
 *
 * Default target: Chromium only.
 * Multi-browser testing is NOT enabled by default. To add Firefox or WebKit,
 * uncomment the relevant project entries below.
 *
 * @see https://playwright.dev/docs/test-configuration
 */

const baseURL = process.env.BASE_URL || 'http://localhost:34115';

export default defineConfig({
  testDir: path.resolve(__dirname, 'tests/e2e'),
  outputDir: path.resolve(__dirname, 'test-results'),

  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,

  timeout: 60_000,
  expect: { timeout: 10_000 },

  reporter: [
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
    ['junit', { outputFile: 'test-results/results.xml' }],
    ['list'],
  ],

  use: {
    baseURL,
    actionTimeout: 15_000,
    navigationTimeout: 30_000,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },

    // Multi-browser targets -- uncomment per-project when needed:
    //
    // {
    //   name: 'firefox',
    //   use: { ...devices['Desktop Firefox'] },
    // },
    // {
    //   name: 'webkit',
    //   use: { ...devices['Desktop Safari'] },
    // },
  ],
});
