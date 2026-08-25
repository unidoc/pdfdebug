/**
 * @file Pure-function literal-substring match finder for the Plain Text
 * find bar. Operates over the Latin-1 byte-for-codepoint string produced by
 * internal/pdfcore/plaintext.go's latin1Decode -- any query codepoint > U+00FF
 * cannot match by construction and short-circuits to an empty list.
 *
 * Non-overlapping by design: searching "aa" in "aaaa" yields 2 matches at
 * offsets 0 and 2, not 3. The cursor advances by start + query.length.
 */

// Self-import the module namespace so findMatches' internal call to
// buildLineStartOffsets goes through the live ESM export binding. A bare local
// call would be inlined by the bundler and could not be intercepted by
// vi.spyOn(module, 'buildLineStartOffsets'); routing through the namespace lets
// the test assert the internal build is skipped when offsets are supplied.
import * as self from './findMatches';

/** One literal substring hit in the corpus. */
export interface Match {
  /** Code-unit offset where the match starts (inclusive). */
  start: number;
  /** Code-unit offset where the match ends (exclusive). */
  end: number;
  /** 1-based line number of the line containing the match's start offset. */
  line: number;
}

/**
 * Returns the list of code-unit offsets where each line begins. Index 0 is
 * always 0 (line 1 starts at offset 0). LF / CR / CRLF all break a line; the
 * offset stored is the position AFTER the line break. Computed once per
 * content and memoizable upstream because the table is O(content length).
 */
export function buildLineStartOffsets(content: string): number[] {
  const offsets: number[] = [0];
  const len = content.length;
  for (let i = 0; i < len; i++) {
    const c = content.charCodeAt(i);
    if (c === 0x0a) {
      // LF
      offsets.push(i + 1);
    } else if (c === 0x0d) {
      // CR or CRLF: skip the following LF (if any) so CRLF counts once.
      if (i + 1 < len && content.charCodeAt(i + 1) === 0x0a) {
        offsets.push(i + 2);
        i++;
      } else {
        offsets.push(i + 1);
      }
    }
  }
  return offsets;
}

/**
 * Binary-search the line-start offset table for the 1-based line number that
 * contains the given code-unit offset. Returns 1 when offsets is empty.
 */
function lineForOffset(offsets: number[], offset: number): number {
  // offsets[i] is the start of line (i + 1). We want the largest i where
  // offsets[i] <= offset, then return i + 1.
  let lo = 0;
  let hi = offsets.length - 1;
  while (lo < hi) {
    const mid = (lo + hi + 1) >>> 1;
    if (offsets[mid] <= offset) {
      lo = mid;
    } else {
      hi = mid - 1;
    }
  }
  return lo + 1;
}

/** True when the query contains any codepoint > U+00FF (non-Latin-1). */
function hasNonLatin1(query: string): boolean {
  // Iterate via code points so a surrogate pair is examined as one codePoint
  // (always > 0xFFFF, therefore > 0xFF).
  for (const ch of query) {
    const cp = ch.codePointAt(0);
    if (cp === undefined) continue;
    if (cp > 0xff) return true;
  }
  return false;
}

/**
 * Find every non-overlapping literal-substring occurrence of `query` in
 * `content`. Case-insensitive when `caseSensitive` is false. Returns an empty
 * array for empty query, empty corpus, query > corpus length, or any non-
 * Latin-1 codepoint in the query. Each Match carries 1-based line.
 *
 * `lineStartOffsets` and `haystack` are optional caller-supplied caches (#8,
 * #20). When omitted the function rebuilds them internally (backward-
 * compatible). When supplied, `haystack` MUST satisfy
 * `haystack.length === content.length` so match offsets index identically into
 * both -- true for the Latin-1 corpus where toLowerCase is length-preserving.
 */
export function findMatches(
  content: string,
  query: string,
  caseSensitive: boolean,
  lineStartOffsets?: number[],
  haystack?: string,
): Match[] {
  if (query === '' || content === '') return [];
  if (query.length > content.length) return [];
  if (hasNonLatin1(query)) return [];

  const searchSpace = haystack ?? (caseSensitive ? content : content.toLowerCase());
  const needle = caseSensitive ? query : query.toLowerCase();
  const offsets = lineStartOffsets ?? self.buildLineStartOffsets(content);

  const matches: Match[] = [];
  const step = needle.length;
  let from = 0;
  while (from <= searchSpace.length - step) {
    const idx = searchSpace.indexOf(needle, from);
    if (idx === -1) break;
    matches.push({
      start: idx,
      end: idx + step,
      line: lineForOffset(offsets, idx),
    });
    from = idx + step;
  }
  return matches;
}
