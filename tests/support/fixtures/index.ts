/**
 * Merged fixture index for UniDOC PDF Debugger E2E tests.
 *
 * Compose all project fixtures here using mergeTests.
 * Tests import { test, expect } from this module.
 */
import { test as base, mergeTests, expect } from '@playwright/test';
import { test as appFixture } from './app-fixture';

export const test = mergeTests(base, appFixture);
export { expect };
