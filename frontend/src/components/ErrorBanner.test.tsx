/**
 * ErrorBanner unit tests -- verifies rendering, accessibility, dismiss behavior.
 * Pushes coverage to lowest viable layer (unit) per testing rules.
 */
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, test, expect, vi } from 'vitest';
import { ErrorBanner } from './ErrorBanner';

describe('ErrorBanner', () => {
  test('renders message with role="alert"', () => {
    render(
      <ErrorBanner message="Something broke" severity="error" onDismiss={() => {}} />
    );
    const banner = screen.getByTestId('error-banner');
    expect(banner).toHaveAttribute('role', 'alert');
    expect(screen.getByTestId('error-banner-message').textContent).toBe(
      'Something broke'
    );
  });

  test('uses red styling for error severity', () => {
    render(
      <ErrorBanner message="err" severity="error" onDismiss={() => {}} />
    );
    const banner = screen.getByTestId('error-banner');
    expect(banner.className).toContain('bg-red-100');
    expect(banner.className).toContain('text-red-900');
  });

  test('uses amber styling for warning severity', () => {
    render(
      <ErrorBanner message="warn" severity="warning" onDismiss={() => {}} />
    );
    const banner = screen.getByTestId('warning-banner');
    expect(banner.className).toContain('bg-amber-100');
    expect(banner.className).toContain('text-amber-900');
  });

  test('warning banner has correct dismiss aria-label', () => {
    render(
      <ErrorBanner message="warn" severity="warning" onDismiss={() => {}} />
    );
    expect(screen.getByTestId('error-banner-dismiss')).toHaveAttribute(
      'aria-label',
      'Dismiss warning'
    );
  });

  test('error banner shows (x) icon', () => {
    render(
      <ErrorBanner message="err" severity="error" onDismiss={() => {}} />
    );
    const banner = screen.getByTestId('error-banner');
    expect(banner.textContent).toContain('(x)');
  });

  test('warning banner shows (!) icon', () => {
    render(
      <ErrorBanner message="warn" severity="warning" onDismiss={() => {}} />
    );
    const banner = screen.getByTestId('warning-banner');
    expect(banner.textContent).toContain('(!)');
  });

  test('calls onDismiss when dismiss button is clicked', async () => {
    const onDismiss = vi.fn();
    render(
      <ErrorBanner message="err" severity="error" onDismiss={onDismiss} />
    );
    await userEvent.click(screen.getByTestId('error-banner-dismiss'));
    expect(onDismiss).toHaveBeenCalledOnce();
  });

  test('dismiss button has aria-label', () => {
    render(
      <ErrorBanner message="err" severity="error" onDismiss={() => {}} />
    );
    expect(screen.getByTestId('error-banner-dismiss')).toHaveAttribute(
      'aria-label',
      'Dismiss error'
    );
  });
});
