/**
 * Render one code point as the display unit standing in for it: the character
 * itself, or its escape sequence. Cutting on unit boundaries is what keeps a
 * two-character escape sequence from being halved by a cap.
 */
function escapeUnit(ch: string): string {
  const code = ch.codePointAt(0) ?? 0;
  if (ch === '\n') return '\\n';
  if (ch === '\r') return '\\r';
  if (ch === '\t') return '\\t';
  if (ch === '\\') return '\\\\';
  if (code < 0x20 || code === 0x7f || (code >= 0x80 && code <= 0x9f)) {
    return `\\x${code.toString(16).toUpperCase().padStart(2, '0')}`;
  }
  return ch;
}

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
  for (const ch of value) out += escapeUnit(ch);
  return out;
}

/**
 * The ceiling a tree row escapes and renders. TreeNode.value is uncapped by
 * design - a dictionary entry can hold a multi-megabyte string literal - and
 * neither CSS truncation nor a `title` attribute bounds the work of rendering
 * it, so the row clamps first and renders the clamp.
 */
export const TREE_VALUE_RENDER_CAP = 2000;

/**
 * Escape `value` with {@link escapeDisplayValue} and cut the result to at most
 * `limit` code points, appending ` [truncated: N of M]` when it elides: N the
 * code points emitted, M the code points the whole escaped value has. A value
 * that fits is escaped whole and carries no marker, so a normal-sized row still
 * shows its full value on screen and in its title.
 *
 * The counting matches the backend's ClampDisplayValue unit for unit - escape
 * first, then cut on code points - so the GUI and the CLI report the same two
 * numbers for the same value. The cut lands on an escape-sequence boundary, so
 * it never separates a backslash from its letter, and once one unit does not
 * fit no later unit is emitted either: skipping a wide unit to fit a narrow one
 * behind it would reorder the text.
 */
export function clampDisplayValue(value: string, limit: number): string {
  let out = '';
  let emitted = 0;
  let total = 0;
  let cut = false;
  for (const ch of value) {
    const unit = escapeUnit(ch);
    const n = [...unit].length;
    total += n;
    if (cut || emitted + n > limit) {
      cut = true;
      continue;
    }
    out += unit;
    emitted += n;
  }
  if (!cut) return out;
  return `${out} [truncated: ${emitted} of ${total}]`;
}
