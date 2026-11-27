/**
 * Reload confirmation pill: a fixed, non-interactive status message at
 * top-center that disappears after about 3 seconds. Driven by its own module
 * store (showReloadToast), so showing it never dispatches to AppState.
 *
 * Run: cd frontend && npx vitest run src/components/ReloadToast.test.tsx
 */
import { render, screen, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import { ReloadToast, showReloadToast } from './ReloadToast';

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  act(() => {
    vi.runOnlyPendingTimers();
  });
  vi.useRealTimers();
});

describe('ReloadToast', () => {
  test('shows the message in a polite status region', () => {
    render(<ReloadToast />);
    act(() => showReloadToast('a.pdf reloaded - view reset'));
    const status = screen.getByRole('status');
    expect(status).toHaveTextContent('a.pdf reloaded - view reset');
    expect(status).not.toHaveAttribute('aria-live', 'assertive');
  });

  test('is a fixed overlay that ignores the pointer and takes no focus', () => {
    render(<ReloadToast />);
    act(() => showReloadToast('a.pdf reloaded - view reset'));
    const pill = screen.getByText('a.pdf reloaded - view reset');
    const box = pill.closest('.fixed') as HTMLElement | null;
    expect(box).not.toBeNull();
    expect(box?.className).toMatch(/pointer-events-none/);
    expect(box?.querySelector('button, a, input, [tabindex]')).toBeNull();
    expect(box?.hasAttribute('tabindex')).toBe(false);
  });

  test('disappears after about 3 seconds', () => {
    render(<ReloadToast />);
    act(() => showReloadToast('a.pdf reloaded - view reset'));
    act(() => {
      vi.advanceTimersByTime(2500);
    });
    expect(screen.getByText('a.pdf reloaded - view reset')).toBeInTheDocument();
    act(() => {
      vi.advanceTimersByTime(1500);
    });
    expect(screen.queryByText('a.pdf reloaded - view reset')).not.toBeInTheDocument();
  });

  test('a second message within the window replaces the text and restarts the timer', () => {
    render(<ReloadToast />);
    act(() => showReloadToast('a.pdf reloaded - view reset'));
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    act(() => showReloadToast('b.pdf is in use by another app - try again'));
    expect(screen.queryByText('a.pdf reloaded - view reset')).not.toBeInTheDocument();
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(screen.getByText('b.pdf is in use by another app - try again')).toBeInTheDocument();
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(screen.queryByText('b.pdf is in use by another app - try again')).not.toBeInTheDocument();
  });

  test('renders nothing visible before any message', () => {
    render(<ReloadToast />);
    expect(screen.queryByText(/reloaded/)).not.toBeInTheDocument();
  });
});
