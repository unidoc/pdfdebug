/**
 * formatBytes unifies on 1 decimal place across KB / MB / GB.
 *
 * Expected outputs:
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
import { formatBytes } from './PlainTextView';

describe('formatBytes precision (1 decimal across KB/MB/GB)', () => {
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

describe('formatBytes negative / non-finite inputs collapse to "0 B"', () => {
  test.each([
    [-1, '0 B'],
    [NaN, '0 B'],
    [Infinity, '0 B'],
    [-Infinity, '0 B'],
  ])('formatBytes(%s) === "0 B"', (input, expected) => {
    expect(formatBytes(input)).toBe(expected);
  });
});
