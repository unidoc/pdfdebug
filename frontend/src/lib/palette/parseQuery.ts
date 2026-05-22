/**
 * @file Command-palette grammar parser. Pure function with no React or DOM.
 * Story 9-8 AC5 contract.
 */

/** Parsed palette query in one of four shapes. */
export type PaletteQuery =
  | { kind: 'empty' }
  | { kind: 'invalid'; raw: string }
  | { kind: 'numeric'; objNum: number; gen: number | null }
  | { kind: 'type'; value: string; exact: boolean };

/**
 * Parse a raw input string into a PaletteQuery.
 *
 * Routing rule: if input after trimming is all digits (with optional
 * generation and optional surrounding parens), route numeric; if it starts
 * with a forward slash, route to exact type filter; otherwise route to
 * prefix type filter for bare alphabetic, or invalid for junk.
 */
export function parseQuery(input: string): PaletteQuery {
  const trimmed = input.trim();
  if (trimmed === '') return { kind: 'empty' };

  // Parenthesized form: strip a single matching pair of parens then re-trim.
  let body = trimmed;
  if (body.startsWith('(') && body.endsWith(')')) {
    body = body.slice(1, -1).trim();
  }

  // Numeric forms. The R suffix is case-insensitive; gen and the R token
  // are both optional.
  const numericMatch = /^(\d+)(?:\s+(\d+)(?:\s+[Rr])?)?$/.exec(body);
  if (numericMatch) {
    const objNum = Number.parseInt(numericMatch[1], 10);
    const gen = numericMatch[2] !== undefined ? Number.parseInt(numericMatch[2], 10) : null;
    return { kind: 'numeric', objNum, gen };
  }

  // Slash-prefixed type filter (exact match). Reject bare slash and any
  // non-identifier body.
  if (body.startsWith('/')) {
    const name = body.slice(1).trim();
    if (name === '' || !/^[A-Za-z][A-Za-z0-9]*$/.test(name)) {
      return { kind: 'invalid', raw: trimmed };
    }
    return { kind: 'type', value: name, exact: true };
  }

  // Bare alphabetic prefix type filter.
  if (/^[A-Za-z][A-Za-z0-9]*$/.test(body)) {
    return { kind: 'type', value: body, exact: false };
  }

  return { kind: 'invalid', raw: trimmed };
}
