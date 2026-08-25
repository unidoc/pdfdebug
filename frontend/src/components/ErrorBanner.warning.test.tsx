/**
 * ErrorBanner severity-specific enhancements.
 *
 * Tests severity-aware data-testid, severity icons, and severity-aware
 * dismiss aria-label. Existing ErrorBanner.test.tsx covers baseline behavior.
 */
import { render, screen } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import { ErrorBanner } from './ErrorBanner';

describe('ErrorBanner severity enhancements', () => {
  // ErrorBanner severity-aware data-testid
  test('warning severity uses data-testid="warning-banner"', () => {
    render(
      <ErrorBanner message="Structural errors" severity="warning" onDismiss={() => {}} />
    );
    // After 2-9, warning severity should use warning-banner testid
    expect(screen.getByTestId('warning-banner')).toBeTruthy();
  });

  test('error severity still uses data-testid="error-banner"', () => {
    render(
      <ErrorBanner message="Fatal error" severity="error" onDismiss={() => {}} />
    );
    expect(screen.getByTestId('error-banner')).toBeTruthy();
  });

  // Severity icons
  test('error severity shows (x) icon', () => {
    render(
      <ErrorBanner message="Fatal error" severity="error" onDismiss={() => {}} />
    );
    const banner = screen.getByTestId('error-banner');
    expect(banner.textContent).toContain('(x)');
  });

  test('warning severity shows (!) icon', () => {
    render(
      <ErrorBanner message="Structural errors" severity="warning" onDismiss={() => {}} />
    );
    const banner = screen.getByTestId('warning-banner');
    expect(banner.textContent).toContain('(!)');
  });

  // Severity-aware dismiss aria-label
  test('warning dismiss button has aria-label "Dismiss warning"', () => {
    render(
      <ErrorBanner message="warn" severity="warning" onDismiss={() => {}} />
    );
    const dismissBtn = screen.getByRole('button', { name: 'Dismiss warning' });
    expect(dismissBtn).toBeTruthy();
  });

  test('error dismiss button has aria-label "Dismiss error"', () => {
    render(
      <ErrorBanner message="err" severity="error" onDismiss={() => {}} />
    );
    const dismissBtn = screen.getByRole('button', { name: 'Dismiss error' });
    expect(dismissBtn).toBeTruthy();
  });

  // Banner intentionally has no `dark:` variants -- the surrounding app shell
  // uses design-token CSS that does not flip on prefers-color-scheme, so a
  // dark-system-theme user otherwise gets a dark banner on a light shell.
  test('error severity does not include dark: variants', () => {
    render(
      <ErrorBanner message="err" severity="error" onDismiss={() => {}} />
    );
    const banner = screen.getByTestId('error-banner');
    expect(banner.className).not.toContain('dark:');
  });

  test('warning severity does not include dark: variants', () => {
    render(
      <ErrorBanner message="warn" severity="warning" onDismiss={() => {}} />
    );
    const banner = screen.getByTestId('warning-banner');
    expect(banner.className).not.toContain('dark:');
  });
});
