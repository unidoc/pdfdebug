/**
 * Story 9-8: Command palette result ranker.
 *
 * TDD RED PHASE: This test imports `./rankResults` which does not exist yet.
 * The module must fail to resolve until Task 4.3 lands.
 *
 * Ordering contract (AC5/AC6/AC8):
 *   - Numeric: exact ObjNum match first, then gen-asc disambiguation
 *   - Type-filter: exact /Type match before prefix match (case-insensitive)
 *   - Within a match tier, free/orphan entries (Reachable=false) sort last
 *   - Result list capped at 8 entries (AC6)
 *
 * Run: cd frontend && npx vitest run src/lib/palette/rankResults.test.ts
 */
import { describe, test, expect } from 'vitest';
// RED: this import fails until rankResults.ts exists.
import { rankResults } from './rankResults';
// RED: type-only import from the same place as palette types -- both files
// need to land before this test compiles.
import type { ObjectIndexEntry } from '../../types/palette';
import { parseQuery } from './parseQuery';

const entry = (overrides: Partial<ObjectIndexEntry>): ObjectIndexEntry => ({
  objNum: 1,
  gen: 0,
  typeName: '',
  free: false,
  reachable: true,
  nodeId: 'obj:0:1',
  ...overrides,
});

const fixture: ObjectIndexEntry[] = [
  entry({ objNum: 1, gen: 0, typeName: 'Catalog', nodeId: 'obj:0:1' }),
  entry({ objNum: 2, gen: 0, typeName: 'Pages', nodeId: 'obj:0:2' }),
  entry({ objNum: 3, gen: 0, typeName: 'Page', nodeId: 'obj:0:3' }),
  entry({ objNum: 4, gen: 0, typeName: 'Page', nodeId: 'obj:0:4' }),
  entry({ objNum: 5, gen: 0, typeName: 'Font', nodeId: 'obj:0:5' }),
  entry({ objNum: 6, gen: 0, typeName: 'FontDescriptor', nodeId: 'obj:0:6' }),
  entry({ objNum: 7, gen: 0, typeName: '', nodeId: 'obj:0:7' }),
  // A free entry for orphan/free tier assertion
  entry({ objNum: 8, gen: 0, typeName: '', free: true, reachable: false, nodeId: '' }),
  // Two generations of obj 9 for gen-asc disambiguation
  entry({ objNum: 9, gen: 0, typeName: '', nodeId: 'obj:0:9' }),
  entry({ objNum: 9, gen: 2, typeName: '', nodeId: 'obj:2:9' }),
];

describe('rankResults (AC5 / AC6 / AC8)', () => {
  test('numeric query returns the matching ObjNum first, gen-asc on ties', () => {
    const ranked = rankResults(parseQuery('9'), fixture);
    expect(ranked.length).toBeGreaterThanOrEqual(2);
    expect(ranked[0].objNum).toBe(9);
    expect(ranked[0].gen).toBe(0);
    expect(ranked[1].objNum).toBe(9);
    expect(ranked[1].gen).toBe(2);
  });

  test('numeric query with explicit gen returns only that one entry', () => {
    const ranked = rankResults(parseQuery('9 2 R'), fixture);
    expect(ranked).toHaveLength(1);
    expect(ranked[0].objNum).toBe(9);
    expect(ranked[0].gen).toBe(2);
  });

  test('exact /Type filter excludes prefix-only matches', () => {
    const ranked = rankResults(parseQuery('/Font'), fixture);
    // FontDescriptor must NOT appear (exact-match only).
    expect(ranked.find((e) => e.typeName === 'FontDescriptor')).toBeUndefined();
    expect(ranked.find((e) => e.typeName === 'Font')).toBeDefined();
  });

  test('prefix /Type filter includes both exact and prefix matches', () => {
    const ranked = rankResults(parseQuery('Font'), fixture);
    // Both Font and FontDescriptor should appear; exact match first.
    const idxFont = ranked.findIndex((e) => e.typeName === 'Font');
    const idxFontDesc = ranked.findIndex((e) => e.typeName === 'FontDescriptor');
    expect(idxFont).toBeGreaterThanOrEqual(0);
    expect(idxFontDesc).toBeGreaterThanOrEqual(0);
    expect(idxFont).toBeLessThan(idxFontDesc);
  });

  test('case-insensitive type matching', () => {
    const upper = rankResults(parseQuery('FONT'), fixture);
    const lower = rankResults(parseQuery('font'), fixture);
    expect(upper.map((e) => e.nodeId)).toEqual(lower.map((e) => e.nodeId));
    expect(upper.length).toBeGreaterThan(0);
  });

  test('free/orphan entries sort last within a match tier', () => {
    // A query that matches both reachable and free entries on ObjNum range.
    // Numeric "8" should return obj 8 -- which in fixture is free.
    const ranked = rankResults(parseQuery('8'), fixture);
    expect(ranked).toHaveLength(1);
    expect(ranked[0].free).toBe(true);
    expect(ranked[0].reachable).toBe(false);
  });

  test('result list is capped at 8 entries (AC6)', () => {
    const many: ObjectIndexEntry[] = [];
    for (let i = 1; i <= 20; i++) {
      many.push(entry({ objNum: i, gen: 0, typeName: 'Page', nodeId: `obj:0:${i}` }));
    }
    const ranked = rankResults(parseQuery('Page'), many);
    expect(ranked.length).toBeLessThanOrEqual(8);
  });

  test('empty query returns no results', () => {
    const ranked = rankResults(parseQuery(''), fixture);
    expect(ranked).toEqual([]);
  });

  test('invalid query returns no results', () => {
    const ranked = rankResults(parseQuery('!@#'), fixture);
    expect(ranked).toEqual([]);
  });

  test('numeric miss returns empty array (no soft fallback to type)', () => {
    const ranked = rankResults(parseQuery('99999'), fixture);
    expect(ranked).toEqual([]);
  });
});
