/**
 * Timestamp text for the tab tooltip, the stale note tooltip and the Compare
 * label: local 24-hour HH:MM when the timestamp's local date is today, the
 * full date ("12 January 2026 14:02") otherwise. Each timestamp is judged on
 * its own against the clock at call time.
 *
 * Run: cd frontend && npx vitest run src/lib/formatTimestamp.test.ts
 */
import { describe, test, expect, vi, afterEach } from 'vitest';
import { formatTimestamp } from './formatTimestamp';

function local(y: number, mo: number, d: number, h: number, mi: number): Date {
  return new Date(y, mo - 1, d, h, mi, 0, 0);
}

afterEach(() => {
  vi.useRealTimers();
});

describe('formatTimestamp', () => {
  test('same day shows time only', () => {
    const now = local(2026, 1, 12, 18, 30);
    expect(formatTimestamp(local(2026, 1, 12, 14, 2).getTime(), now)).toBe('14:02');
  });

  test('previous day shows the full date', () => {
    const now = local(2026, 1, 12, 18, 30);
    expect(formatTimestamp(local(2026, 1, 11, 14, 2).getTime(), now)).toBe('11 January 2026 14:02');
  });

  test('previous year shows the full date with the year', () => {
    const now = local(2026, 1, 12, 9, 0);
    expect(formatTimestamp(local(2025, 12, 31, 9, 5).getTime(), now)).toBe('31 December 2025 09:05');
  });

  test('across midnight: just before midnight is yesterday, just after is today', () => {
    const now = local(2026, 1, 12, 0, 1);
    expect(formatTimestamp(local(2026, 1, 11, 23, 59).getTime(), now)).toBe('11 January 2026 23:59');
    expect(formatTimestamp(local(2026, 1, 12, 0, 0).getTime(), now)).toBe('00:00');
  });

  test('day has no leading zero, hours and minutes do', () => {
    const now = local(2026, 3, 20, 12, 0);
    expect(formatTimestamp(local(2026, 3, 5, 7, 4).getTime(), now)).toBe('5 March 2026 07:04');
  });

  test('a future date on another day is not today', () => {
    const now = local(2026, 1, 12, 12, 0);
    expect(formatTimestamp(local(2026, 1, 13, 8, 0).getTime(), now)).toBe('13 January 2026 08:00');
  });

  test('defaults to the current clock when no reference time is given', () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(local(2026, 1, 12, 18, 30));
    expect(formatTimestamp(local(2026, 1, 12, 14, 2).getTime())).toBe('14:02');
    vi.setSystemTime(local(2026, 1, 13, 0, 5));
    expect(formatTimestamp(local(2026, 1, 12, 14, 2).getTime())).toBe('12 January 2026 14:02');
  });

  test('output is ASCII only', () => {
    const now = local(2026, 1, 12, 18, 30);
    const out = formatTimestamp(local(2025, 9, 1, 1, 1).getTime(), now);
    expect(out).toMatch(/^[\x20-\x7e]+$/);
  });
});
