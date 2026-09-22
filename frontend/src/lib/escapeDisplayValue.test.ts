/**
 * escapeDisplayValue renders a decoded scalar's control characters as escape
 * sequences. It is the frontend counterpart of the backend's
 * EscapeDisplayValue, and the two must produce the same text for the same
 * bytes, or the GUI and the CLI disagree about the same value.
 *
 * Run: cd frontend && npx vitest run src/lib/escapeDisplayValue.test.ts
 */
import { describe, test, expect } from 'vitest';
import { clampDisplayValue, escapeDisplayValue, TREE_VALUE_RENDER_CAP } from './escapeDisplayValue';

describe('escapeDisplayValue', () => {
  test('leaves ordinary text untouched', () => {
    expect(escapeDisplayValue('Rapport cell')).toBe('Rapport cell');
    expect(escapeDisplayValue('')).toBe('');
  });

  test('renders the three named control characters by name', () => {
    expect(escapeDisplayValue('a\nb')).toBe('a\\nb');
    expect(escapeDisplayValue('a\rb')).toBe('a\\rb');
    expect(escapeDisplayValue('a\tb')).toBe('a\\tb');
  });

  test('doubles a literal backslash so the escaping stays reversible', () => {
    expect(escapeDisplayValue('a\\b')).toBe('a\\\\b');
    expect(escapeDisplayValue('a\\nb')).toBe('a\\\\nb');
  });

  test('renders the remaining C0 controls and DEL as uppercase hex', () => {
    expect(escapeDisplayValue('a\x00b')).toBe('a\\x00b');
    expect(escapeDisplayValue('a\x0bb')).toBe('a\\x0Bb');
    expect(escapeDisplayValue('a\x1fb')).toBe('a\\x1Fb');
    expect(escapeDisplayValue('a\x7fb')).toBe('a\\x7Fb');
  });

  test('renders the C1 block, which the Latin-1 fallback makes reachable', () => {
    // A CP1252 curly apostrophe (0x92) decodes to U+0092, an invisible
    // character that must not reach the screen unmarked.
    expect(escapeDisplayValue('don\u0092t')).toBe('don\\x92t');
    expect(escapeDisplayValue('\u0080')).toBe('\\x80');
    expect(escapeDisplayValue('\u009f')).toBe('\\x9F');
  });

  test('leaves printable characters on both sides of the C1 block alone', () => {
    expect(escapeDisplayValue('~')).toBe('~');
    expect(escapeDisplayValue('\u00a0')).toBe('\u00a0');
    expect(escapeDisplayValue('\u00e9')).toBe('\u00e9');
  });

  test('passes multi-byte and astral characters through whole', () => {
    expect(escapeDisplayValue('\u4e2d\u6587')).toBe('\u4e2d\u6587');
    expect(escapeDisplayValue('\u{1f600}')).toBe('\u{1f600}');
  });
});

describe('clampDisplayValue', () => {
  test('escapes a value at or under the limit whole, with no marker', () => {
    expect(clampDisplayValue('a\nb', 10)).toBe('a\\nb');
    expect(clampDisplayValue('xxxxx', 5)).toBe('xxxxx');
  });

  test('marks a cut with the emitted and total counts', () => {
    expect(clampDisplayValue('xxxxxx', 5)).toBe('xxxxx [truncated: 5 of 6]');
  });

  test('escaping happens before the cut, so both counts are escaped lengths', () => {
    // Ten NULs escape to forty characters; a limit of ten fits two of them.
    expect(clampDisplayValue('\x00'.repeat(10), 10)).toBe('\\x00\\x00 [truncated: 8 of 40]');
  });

  test('an escape sequence is never halved', () => {
    // The fifth tab would need two more characters than the limit leaves.
    expect(clampDisplayValue('\t'.repeat(6), 9)).toBe('\\t\\t\\t\\t [truncated: 8 of 12]');
  });

  test('nothing after a unit that did not fit is emitted, so the text keeps its order', () => {
    // The tab does not fit, and the "b" behind it must not jump the gap.
    expect(clampDisplayValue('a\tb', 2)).toBe('a [truncated: 1 of 4]');
  });

  test('counts astral characters as one unit each, as the backend counts runes', () => {
    expect(clampDisplayValue('\u{1f600}\u{1f600}', 1)).toBe('\u{1f600} [truncated: 1 of 2]');
    expect(clampDisplayValue('\u{1f600}\u{1f600}', 2)).toBe('\u{1f600}\u{1f600}');
  });

  test('counts multi-byte text by code point, not by byte', () => {
    expect(clampDisplayValue('中'.repeat(90), 80)).toBe(
      `${'中'.repeat(80)} [truncated: 80 of 90]`,
    );
  });

  test('the render cap leaves a normal-sized value intact', () => {
    const value = 'L'.repeat(500);
    expect(clampDisplayValue(value, TREE_VALUE_RENDER_CAP)).toBe(value);
  });

  test('reports the same text the backend reports for the same value and limit', () => {
    // Mirrors the backend's ClampDisplayValue table at the plain-text tree cap.
    const cap = 80;
    expect(clampDisplayValue('x'.repeat(79), cap)).toBe('x'.repeat(79));
    expect(clampDisplayValue('x'.repeat(80), cap)).toBe('x'.repeat(80));
    expect(clampDisplayValue('x'.repeat(81), cap)).toBe(`${'x'.repeat(80)} [truncated: 80 of 81]`);
    expect(clampDisplayValue('\t'.repeat(41), cap)).toBe(
      `${'\\t'.repeat(40)} [truncated: 80 of 82]`,
    );
    expect(clampDisplayValue(`a${'\t'.repeat(41)}`, cap)).toBe(
      `a${'\\t'.repeat(39)} [truncated: 79 of 83]`,
    );
  });
});
