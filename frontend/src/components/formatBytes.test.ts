/**
 * Story 10.8 AC4: formatBytes unifies on 1 decimal place across KB / MB / GB.
 *
 * TDD RED PHASE: this test fails against the current PlainTextView.tsx for two
 * reasons:
 *   1. `formatBytes` is module-private (no `export`) -- the import below fails
 *      to resolve until it is changed to `export function formatBytes`.
 *   2. The MB branch uses `Math.round(n / (1024*1024))` (integer, no decimal)
 *      and the GB branch uses `.toFixed(2)`; both must become `.toFixed(1)`.
 *
 * Expected outputs per AC4:
 *   0            -> "0 B"
 *   512          -> "512 B"
 *   1024         -> "1.0 KB"
 *   5500         -> "5.4 KB"
 *   1_048_576    -> "1.0 MB"
 *   5_500_000    -> "5.2 MB"
 *   1_073_741_824-> "1.0 GB"
 *   5_500_000_000-> "5.1 GB"
 *
 * Negative and non-finite inputs collapse to "0 B".
 *
 * Run: cd frontend && npx vitest run src/components/formatBytes.test.ts
 */
import { describe, test, expect } from 'vitest';
// RED PHASE: fails until `formatBytes` is exported from PlainTextView.tsx.
import { formatBytes } from './PlainTextView';

describe('10-8-UNIT-005: formatBytes precision (1 decimal across KB/MB/GB)', () => {
  test.each([
    [0, '0 B'],
    [512, '512 B'],
    [1024, '1.0 KB'],
    [5500, '5.4 KB'],
    [1_048_576, '1.0 MB'],
    [5_500_000, '5.2 MB'],
    [1_073_741_824, '1.0 GB'],
    [5_500_000_000, '5.1 GB'],
  ])('formatBytes(%d) === %s', (input, expected) => {
    expect(formatBytes(input)).toBe(expected);
  });
});

describe('10-8-UNIT-006: formatBytes negative / non-finite inputs collapse to "0 B"', () => {
  test.each([
    [-1, '0 B'],
    [NaN, '0 B'],
    [Infinity, '0 B'],
    [-Infinity, '0 B'],
  ])('formatBytes(%s) === "0 B"', (input, expected) => {
    expect(formatBytes(input)).toBe(expected);
  });
});
