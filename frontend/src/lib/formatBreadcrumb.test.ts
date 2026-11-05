import { describe, test, expect } from 'vitest';
import { formatBreadcrumb } from './formatBreadcrumb';

describe('formatBreadcrumb', () => {
  test('joins path elements with " > "', () => {
    expect(formatBreadcrumb(['root', 'obj:0:2', 'obj:0:4'])).toBe('root > obj:0:2 > obj:0:4');
  });

  test('an empty path is an empty string', () => {
    expect(formatBreadcrumb([])).toBe('');
  });
});
