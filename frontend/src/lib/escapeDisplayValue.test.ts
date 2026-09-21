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
  test('escapes a value at or under the limit whole', () => {
    expect(clampDisplayValue('a\nb', 10)).toBe('a\\nb');
    expect(clampDisplayValue('xxxxx', 5)).toBe('xxxxx');
  });

  test('cuts a value over the limit and marks the cut', () => {
    expect(clampDisplayValue('xxxxxx', 5)).toBe('xxxxx...');
  });

  test('escaping happens after the cut, so the result stays bounded', () => {
    // Ten NULs escape to forty characters; only the first three are kept.
    expect(clampDisplayValue('\x00'.repeat(10), 3)).toBe('\\x00\\x00\\x00...');
  });

  test('never cuts an astral character in half', () => {
    // Each emoji is two UTF-16 code units, so a limit of 3 lands mid-pair.
    expect(clampDisplayValue('\u{1f600}\u{1f600}', 3)).toBe('\u{1f600}...');
  });

  test('the render cap leaves a normal-sized value intact', () => {
    const value = 'L'.repeat(500);
    expect(clampDisplayValue(value, TREE_VALUE_RENDER_CAP)).toBe(value);
  });
});
