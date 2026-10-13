/**
 * Command palette grammar parser.
 *
 * Grammar contract:
 *   - `847`              -> numeric, any gen
 *   - `847 0`            -> numeric, gen=0
 *   - `847 0 R`          -> numeric, gen=0 (PDF syntax)
 *   - `(847 0 R)`        -> numeric, gen=0 (paste from string literal)
 *   - `/Font`            -> exact /Type match (case-insensitive)
 *   - `Font`             -> prefix /Type match (case-insensitive)
 *   - whitespace tolerant on all forms
 *   - empty input        -> { kind: 'empty' }
 *   - non-matching junk  -> { kind: 'invalid' }
 *
 * Run: cd frontend && npx vitest run src/lib/palette/parseQuery.test.ts
 */
import { describe, test, expect } from 'vitest';
import { parseQuery, type PaletteQuery } from './parseQuery';

describe('parseQuery', () => {
  describe('numeric form', () => {
    const cases: Array<[string, PaletteQuery]> = [
      ['847', { kind: 'numeric', objNum: 847, gen: null }],
      ['  847  ', { kind: 'numeric', objNum: 847, gen: null }],
      ['847 0', { kind: 'numeric', objNum: 847, gen: 0 }],
      ['847   0', { kind: 'numeric', objNum: 847, gen: 0 }],
      ['847 0 R', { kind: 'numeric', objNum: 847, gen: 0 }],
      ['847 0 r', { kind: 'numeric', objNum: 847, gen: 0 }],
      ['(847 0 R)', { kind: 'numeric', objNum: 847, gen: 0 }],
      [' (  847   0   R  ) ', { kind: 'numeric', objNum: 847, gen: 0 }],
      ['847 3 R', { kind: 'numeric', objNum: 847, gen: 3 }],
      ['1', { kind: 'numeric', objNum: 1, gen: null }],
    ];

    test.each(cases)('parses %j', (input, expected) => {
      expect(parseQuery(input)).toEqual(expected);
    });
  });

  describe('type-filter form', () => {
    test('leading slash routes to exact-match type filter', () => {
      expect(parseQuery('/Font')).toEqual({
        kind: 'type',
        value: 'Font',
        exact: true,
      });
    });

    test('leading slash is case-insensitive', () => {
      // Implementation normalizes the stored value lowercase OR preserves
      // case; either is acceptable as long as the consumer can match
      // case-insensitively. We assert on a lowercase round-trip.
      const q = parseQuery('/font');
      expect(q.kind).toBe('type');
      expect(q.exact).toBe(true);
      expect(q.value.toLowerCase()).toBe('font');
    });

    test('bare non-numeric routes to prefix-match type filter', () => {
      expect(parseQuery('Font')).toEqual({
        kind: 'type',
        value: 'Font',
        exact: false,
      });
    });

    test('bare non-numeric prefix is case-insensitive', () => {
      const q = parseQuery('PaGe');
      expect(q.kind).toBe('type');
      expect(q.exact).toBe(false);
      expect(q.value.toLowerCase()).toBe('page');
    });

    test('whitespace around bare type name is tolerated', () => {
      expect(parseQuery('  Pages  ')).toEqual({
        kind: 'type',
        value: 'Pages',
        exact: false,
      });
    });
  });

  describe('empty input', () => {
    test('empty string yields empty kind', () => {
      expect(parseQuery('')).toEqual({ kind: 'empty' });
    });
    test('all-whitespace yields empty kind', () => {
      expect(parseQuery('   ')).toEqual({ kind: 'empty' });
    });
  });

  describe('invalid input', () => {
    test('non-grammatical junk yields invalid kind', () => {
      expect(parseQuery('!@#')).toEqual({ kind: 'invalid', raw: '!@#' });
    });

    test('leading slash with empty body is invalid', () => {
      expect(parseQuery('/')).toEqual({ kind: 'invalid', raw: '/' });
    });

    test('digits with stray non-R suffix is invalid', () => {
      // "847 0 X" -- not a legal PDF ref tail
      expect(parseQuery('847 0 X')).toEqual({ kind: 'invalid', raw: '847 0 X' });
    });

    test('negative number is invalid', () => {
      expect(parseQuery('-5')).toEqual({ kind: 'invalid', raw: '-5' });
    });
  });
});
