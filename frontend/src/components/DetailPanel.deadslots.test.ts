/**
 * Dead write-only state slots are removed from DetailPanel.tsx.
 *
 * The `setLoading` and `setFontLoading` slots are write-only (their reads were
 * dropped); they mislead future readers. They are deleted along with all call
 * sites. `showFontLoading` is RETAINED -- it is read in the JSX gating
 * (`!fontState && showFontLoading`).
 *
 * There is no observable behavior to assert (the slots were write-only), so
 * per the AC this is verified structurally: grep `setLoading|setFontLoading`
 * must return no matches. This test reads the component source directly rather
 * than the bundled module so the source identifiers are checked, not runtime
 * behavior.
 *
 * Run: cd frontend && npx vitest run src/components/DetailPanel.deadslots.test.ts
 */
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { describe, test, expect } from 'vitest';

const here = dirname(fileURLToPath(import.meta.url));
const source = readFileSync(join(here, 'DetailPanel.tsx'), 'utf8');

describe('DetailPanel dead-slot removal', () => {
  test('setLoading is fully removed (declaration + call sites)', () => {
    expect(source).not.toMatch(/setLoading/);
  });

  test('setFontLoading is fully removed (declaration + call sites)', () => {
    expect(source).not.toMatch(/setFontLoading/);
  });

  test('showFontLoading is RETAINED (read in JSX gating)', () => {
    expect(source).toMatch(/showFontLoading/);
  });
});
