/**
 * Render a scalar value's control characters as escape sequences so one value
 * always occupies exactly one line: `\n`, `\r` and `\t` by name, every other C0
 * control plus DEL and the whole C1 block as `\xHH` in uppercase hex, and a
 * literal backslash doubled so the escaping stays reversible.
 *
 * C1 matters because the backend's Latin-1 decode fallback turns a CP1252 curly
 * apostrophe (0x92) into U+0092, which would otherwise render invisible. The
 * backend sends the decoded string unescaped, so the escaping happens here for
 * the same reason the CLI does it on its own rows.
 */
export function escapeDisplayValue(value: string): string {
  let out = '';
  for (const ch of value) {
    const code = ch.codePointAt(0) ?? 0;
    if (ch === '\n') out += '\\n';
    else if (ch === '\r') out += '\\r';
    else if (ch === '\t') out += '\\t';
    else if (ch === '\\') out += '\\\\';
    else if (code < 0x20 || code === 0x7f || (code >= 0x80 && code <= 0x9f)) {
      out += `\\x${code.toString(16).toUpperCase().padStart(2, '0')}`;
    } else out += ch;
  }
  return out;
}

/**
 * The ceiling a tree row escapes and renders. TreeNode.value is uncapped by
 * design - a dictionary entry can hold a multi-megabyte string literal - and
 * neither CSS truncation nor a `title` attribute bounds the work of escaping
 * it, so the row cuts first and escapes the cut.
 */
export const TREE_VALUE_RENDER_CAP = 2000;

/**
 * Cut `value` to at most `limit` UTF-16 code units, then escape it with
 * {@link escapeDisplayValue}. A cut appends a trailing `...`; a value at or
 * under the limit is escaped whole, so a normal-sized row still carries its
 * full value on screen and in its title.
 *
 * The cut lands on a code-point boundary: a trailing lone high surrogate is
 * dropped rather than rendered as a replacement character.
 */
export function clampDisplayValue(value: string, limit: number): string {
  if (value.length <= limit) return escapeDisplayValue(value);
  let cut = value.slice(0, limit);
  const last = cut.charCodeAt(cut.length - 1);
  if (last >= 0xd800 && last <= 0xdbff) cut = cut.slice(0, -1);
  return `${escapeDisplayValue(cut)}...`;
}
