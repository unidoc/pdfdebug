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
