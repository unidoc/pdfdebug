/**
 * 2.9-UNIT: ErrorBanner severity-specific enhancements.
 * TDD RED PHASE -- these tests MUST fail until Story 2-9 is implemented.
 *
 * Tests severity-aware data-testid, severity icons, and severity-aware
 * dismiss aria-label. Existing ErrorBanner.test.tsx covers baseline behavior.
 */
import { render, screen } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import { ErrorBanner } from './ErrorBanner';

describe('2.9-UNIT: ErrorBanner severity enhancements', () => {
  // 2.9-UNIT-002 [P2]: ErrorBanner severity-aware data-testid
  test('[P1] warning severity uses data-testid="warning-banner"', () => {
    render(
      <ErrorBanner message="Structural errors" severity="warning" onDismiss={() => {}} />
    );
    // After 2-9, warning severity should use warning-banner testid
    expect(screen.getByTestId('warning-banner')).toBeTruthy();
  });

  test('[P1] error severity still uses data-testid="error-banner"', () => {
    render(
      <ErrorBanner message="Fatal error" severity="error" onDismiss={() => {}} />
    );
    expect(screen.getByTestId('error-banner')).toBeTruthy();
  });

  // Severity icons
  test('[P1] error severity shows (x) icon', () => {
    render(
      <ErrorBanner message="Fatal error" severity="error" onDismiss={() => {}} />
    );
    const banner = screen.getByTestId('error-banner');
    expect(banner.textContent).toContain('(x)');
  });

  test('[P1] warning severity shows (!) icon', () => {
    render(
      <ErrorBanner message="Structural errors" severity="warning" onDismiss={() => {}} />
    );
    const banner = screen.getByTestId('warning-banner');
    expect(banner.textContent).toContain('(!)');
  });

  // Severity-aware dismiss aria-label
  test('[P1] warning dismiss button has aria-label "Dismiss warning"', () => {
    render(
      <ErrorBanner message="warn" severity="warning" onDismiss={() => {}} />
    );
    const dismissBtn = screen.getByRole('button', { name: 'Dismiss warning' });
    expect(dismissBtn).toBeTruthy();
  });

  test('[P1] error dismiss button has aria-label "Dismiss error"', () => {
    render(
      <ErrorBanner message="err" severity="error" onDismiss={() => {}} />
    );
    const dismissBtn = screen.getByRole('button', { name: 'Dismiss error' });
    expect(dismissBtn).toBeTruthy();
  });

  // Dark mode variants
  test('[P2] error severity has dark mode bg variant', () => {
    render(
      <ErrorBanner message="err" severity="error" onDismiss={() => {}} />
    );
    const banner = screen.getByTestId('error-banner');
    expect(banner.className).toContain('dark:bg-red-900');
  });

  test('[P2] warning severity has dark mode bg variant', () => {
    render(
      <ErrorBanner message="warn" severity="warning" onDismiss={() => {}} />
    );
    const banner = screen.getByTestId('warning-banner');
    expect(banner.className).toContain('dark:bg-amber-900');
  });
});
