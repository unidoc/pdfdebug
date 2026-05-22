/**
 * @file Command-palette result ranker. Pure function operating on a parsed
 * query and the in-memory object index. Story 9-8 AC5/AC6/AC8.
 */
import type { ObjectIndexEntry } from '../../types/palette';
import type { PaletteQuery } from './parseQuery';

const MAX_RESULTS = 8;

/**
 * Rank the index against a parsed query and return at most MAX_RESULTS rows.
 *
 * Numeric: match by ObjNum (and Gen when supplied), order Gen asc.
 * Type: exact match when query.exact, otherwise case-insensitive prefix
 *       match; exact-tier rows sort before prefix-tier rows.
 * Within any match tier: reachable rows sort before free/orphan rows.
 *
 * Returns [] for empty/invalid queries and for numeric misses (no soft
 * fallback to type filter).
 */
export function rankResults(query: PaletteQuery, index: ObjectIndexEntry[]): ObjectIndexEntry[] {
  if (query.kind === 'empty' || query.kind === 'invalid') return [];

  if (query.kind === 'numeric') {
    const matched = index.filter((e) => {
      if (e.objNum !== query.objNum) return false;
      if (query.gen !== null && e.gen !== query.gen) return false;
      return true;
    });
    matched.sort((a, b) => {
      // Reachable rows precede free/orphan rows.
      if (a.reachable !== b.reachable) return a.reachable ? -1 : 1;
      return a.gen - b.gen;
    });
    return matched.slice(0, MAX_RESULTS);
  }

  // type filter
  const needle = query.value.toLowerCase();
  const matched: Array<{ entry: ObjectIndexEntry; tier: number }> = [];
  for (const e of index) {
    if (e.typeName === '') continue;
    const tn = e.typeName.toLowerCase();
    if (query.exact) {
      if (tn === needle) matched.push({ entry: e, tier: 0 });
    } else {
      if (tn === needle) {
        matched.push({ entry: e, tier: 0 });
      } else if (tn.startsWith(needle)) {
        matched.push({ entry: e, tier: 1 });
      }
    }
  }
  matched.sort((a, b) => {
    if (a.tier !== b.tier) return a.tier - b.tier;
    // Reachable rows precede free/orphan rows within a tier.
    if (a.entry.reachable !== b.entry.reachable) return a.entry.reachable ? -1 : 1;
    if (a.entry.objNum !== b.entry.objNum) return a.entry.objNum - b.entry.objNum;
    return a.entry.gen - b.entry.gen;
  });
  return matched.slice(0, MAX_RESULTS).map((m) => m.entry);
}
