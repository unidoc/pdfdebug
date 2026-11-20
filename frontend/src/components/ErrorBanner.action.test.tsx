/**
 * ErrorBanner optional action button and root test id.
 *
 * actionLabel and onAction render a button before Dismiss; testId replaces the
 * severity-derived root test id. Without them the banner renders as before.
 *
 * Run: cd frontend && npx vitest run src/components/ErrorBanner.action.test.tsx
 */
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, test, expect, vi } from 'vitest';
import { ErrorBanner } from './ErrorBanner';

describe('ErrorBanner action and testId props', () => {
  test('renders the action button before Dismiss and calls onAction', () => {
    const onAction = vi.fn();
    const onDismiss = vi.fn();
    render(
      <ErrorBanner
        message="pdfcpu rejects this file: bad"
        severity="warning"
        onDismiss={onDismiss}
        actionLabel="Details"
        onAction={onAction}
      />,
    );
    const buttons = screen.getAllByRole('button');
    const names = buttons.map((b) => b.textContent);
    expect(names).toEqual(['Details', 'X']);
    fireEvent.click(screen.getByRole('button', { name: 'Details' }));
    expect(onAction).toHaveBeenCalledTimes(1);
    expect(onDismiss).not.toHaveBeenCalled();
  });

  test('testId replaces the warning-banner root test id', () => {
    render(<ErrorBanner message="m" severity="warning" onDismiss={() => {}} testId="pdfcpu-banner" />);
    const root = screen.getByTestId('pdfcpu-banner');
    expect(root).toHaveAttribute('role', 'alert');
    expect(screen.queryByTestId('warning-banner')).toBeNull();
  });

  test('without the new props there is only the Dismiss button and the usual test id', () => {
    render(<ErrorBanner message="Structural errors" severity="warning" onDismiss={() => {}} />);
    expect(screen.getByTestId('warning-banner')).toBeInTheDocument();
    expect(screen.getAllByRole('button')).toHaveLength(1);
    expect(screen.getByTestId('error-banner-dismiss')).toBeInTheDocument();
  });
});
