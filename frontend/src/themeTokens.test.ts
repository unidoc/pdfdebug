/**
 * Colour-token guard for row states and diff text.
 *
 * Reads style.css, parses the `:root` hex values and checks the contrast
 * floors between the selected-row fill, the focus bar, the hover fills, the
 * tab hover divider and label, the diff text colours and the `-on-selected`
 * text tokens that `.row-selected-text` maps onto inside a selected row, using
 * WCAG 2.x relative luminance. It also checks every new token is registered in
 * `@theme inline`: an unregistered token generates no utility class, so the
 * component className assertions would still pass.
 *
 * Run: cd frontend && npx vitest run src/themeTokens.test.ts
 */
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { describe, test, expect } from 'vitest';
import { ROW_SELECTED_FILL } from './components/rowState';

const here = dirname(fileURLToPath(import.meta.url));
// Comments are stripped so a commented-out declaration never counts, and a `}`
// inside a comment cannot end a block early.
const css = readFileSync(join(here, 'style.css'), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');

function block(selector: RegExp): string {
  const m = css.match(selector);
  return m ? m[1] : '';
}

const rootBlock = block(/:root\s*\{([^}]*)\}/);
const themeInlineBlock = block(/@theme\s+inline\s*\{([^}]*)\}/);
const selectedTextBlock = block(/\.row-selected-text\s*\{([^}]*)\}/);

// Every declaration, last one wins as in the cascade, whatever its value.
function declarations(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const m of text.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) {
    out[m[1]] = m[2].trim().toLowerCase();
  }
  return out;
}

const rootValue = declarations(rootBlock);
const selectedTextValue = declarations(selectedTextBlock);

const HEX6 = /^#[0-9a-f]{6}$/;

function token(name: string): string {
  const v = rootValue[name];
  if (v === undefined) throw new Error(`style.css :root does not declare ${name}`);
  if (!HEX6.test(v)) throw new Error(`style.css :root ${name} is "${v}", expected #rrggbb`);
  return v;
}

function luminance(hex: string): number {
  const channels = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
  const [r, g, b] = channels.map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(a: string, b: string): number {
  const la = luminance(a);
  const lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

function expectFloor(fg: string, bg: string, floor: number) {
  const ratio = contrast(token(fg), token(bg));
  expect(
    ratio,
    `${fg} (${token(fg)}) vs ${bg} (${token(bg)}) is ${ratio.toFixed(2)}:1, needs >= ${floor}:1`
  ).toBeGreaterThanOrEqual(floor);
}

const NEW_TOKENS = [
  '--color-row-selected',
  '--color-surface-armed',
  '--color-tab-hover',
  '--color-tab-hover-border',
  '--color-diff-added',
  '--color-diff-removed',
  '--color-diff-changed',
  '--color-diff-context',
];

const DIFF_TEXT = ['--color-diff-added', '--color-diff-removed', '--color-diff-changed', '--color-diff-context'];
const DIFF_BACKGROUNDS = ['--color-bg', '--color-surface-hover', '--color-row-selected', '--color-surface'];

describe('contrast helper', () => {
  test('black on white is 21:1', () => {
    expect(contrast('#000000', '#ffffff')).toBeCloseTo(21, 5);
  });
});

describe('token registration', () => {
  test.each(NEW_TOKENS)('%s is declared on :root with a hex value', (name) => {
    expect(HEX6.test(rootValue[name] ?? ''), `style.css :root has no #rrggbb value for ${name}`).toBe(true);
  });

  test.each(NEW_TOKENS)('%s is registered in @theme inline', (name) => {
    const re = new RegExp(`${name}\\s*:\\s*var\\(\\s*${name}\\s*\\)\\s*;`);
    expect(re.test(themeInlineBlock), `@theme inline does not register ${name}: var(${name});`).toBe(true);
  });

  test.each(['--color-tree-selected', '--color-tree-hover', '--color-surface-selected'])(
    'unused token %s is gone from :root and @theme inline',
    (name) => {
      const re = new RegExp(`${name}\\s*:`);
      expect(re.test(rootBlock), `${name} is still declared on :root`).toBe(false);
      expect(re.test(themeInlineBlock), `${name} is still registered in @theme inline`).toBe(false);
    }
  );
});

describe('selected row fill and focus bar', () => {
  test('focus bar stands out from the selected fill', () => {
    expectFloor('--color-border-focus', '--color-row-selected', 3.0);
  });

  test('focus bar stands out from the panel background', () => {
    expectFloor('--color-border-focus', '--color-bg', 3.0);
  });

  test('focus bar stands out from the palette surface', () => {
    expectFloor('--color-border-focus', '--color-surface', 3.0);
  });

  test('selected fill is distinguishable from the panel background', () => {
    expectFloor('--color-row-selected', '--color-bg', 1.15);
  });

  test('selected fill is distinguishable from the palette surface', () => {
    expectFloor('--color-row-selected', '--color-surface', 1.15);
  });

  test('selected fill is distinguishable from the hover fill', () => {
    expectFloor('--color-row-selected', '--color-surface-hover', 1.1);
  });

  test('selected fill and hover fill are different values', () => {
    expect(token('--color-row-selected'), '--color-row-selected equals --color-surface-hover').not.toBe(
      token('--color-surface-hover')
    );
  });
});

describe('file-tab hover fill', () => {
  test('tab hover is distinguishable from the inactive tab rest fill', () => {
    expectFloor('--color-tab-hover', '--color-surface-hover', 1.1);
  });

  test('tab hover is distinguishable from the active tab fill', () => {
    expectFloor('--color-tab-hover', '--color-bg', 1.1);
  });

  test('tab hover and active tab fill are different values', () => {
    expect(token('--color-tab-hover'), '--color-tab-hover equals --color-bg').not.toBe(token('--color-bg'));
  });

  test('hovered tab divider stands out from the tab hover fill', () => {
    expectFloor('--color-tab-hover-border', '--color-tab-hover', 1.15);
  });

  test('hovered tab divider and tab hover fill are different values', () => {
    expect(token('--color-tab-hover-border'), '--color-tab-hover-border equals --color-tab-hover').not.toBe(
      token('--color-tab-hover')
    );
  });

  test('hovered tab label is readable on the tab hover fill', () => {
    expectFloor('--color-text', '--color-tab-hover', 4.5);
  });
});

// Resolves a `.row-selected-text` override to the `:root` token it points at.
function overrideTarget(name: string): string | undefined {
  return selectedTextValue[name]?.match(/^var\(\s*(--[\w-]+)\s*\)$/)?.[1];
}

describe('text inside a selected row', () => {
  const typeTokens = Object.keys(rootValue).filter(
    (n) => n.startsWith('--color-type-') && !n.endsWith('-on-selected')
  );

  test('the selected-row fill classes include the scoped text overrides', () => {
    expect(ROW_SELECTED_FILL.split(/\s+/)).toContain('row-selected-text');
    expect(selectedTextBlock.trim(), 'style.css has no .row-selected-text block').not.toBe('');
  });

  test.each(['--color-text-muted', '--color-text-secondary', '--color-error'])(
    '%s is overridden inside a selected row',
    (name) => {
      expect(selectedTextValue[name], `.row-selected-text does not redefine ${name}`).toBeDefined();
    }
  );

  test('every override maps a :root token onto its -on-selected token', () => {
    for (const [name, value] of Object.entries(selectedTextValue)) {
      expect(rootValue[name], `.row-selected-text redefines ${name}, which :root does not declare`).toBeDefined();
      expect(value, `.row-selected-text ${name} is "${value}", expected var(${name}-on-selected)`).toBe(
        `var(${name}-on-selected)`
      );
    }
  });

  test.each(Object.keys(selectedTextValue))('override %s is readable on the selected fill', (name) => {
    const target = overrideTarget(name) ?? `${name}-on-selected`;
    expectFloor(target, '--color-row-selected', 4.5);
  });

  test('secondary text stays darker than muted text inside a selected row', () => {
    const secondary = token('--color-text-secondary-on-selected');
    const muted = token('--color-text-muted-on-selected');
    expect(secondary, 'secondary and muted collapse to one colour on the selected fill').not.toBe(muted);
    expect(luminance(secondary), `secondary ${secondary} is not darker than muted ${muted}`).toBeLessThan(luminance(muted));
  });

  test('the error glyph colour is readable on the selected fill', () => {
    expect(overrideTarget('--color-error'), '.row-selected-text does not override --color-error').toBe(
      '--color-error-on-selected'
    );
    expectFloor('--color-error-on-selected', '--color-row-selected', 4.5);
  });

  test.each(typeTokens)('%s is readable on the selected fill, with or without an override', (name) => {
    const target = overrideTarget(name);
    const value = target ? token(target) : token(name);
    const ratio = contrast(value, token('--color-row-selected'));
    expect(ratio, `${name} (${value}) is ${ratio.toFixed(2)}:1 on --color-row-selected`).toBeGreaterThanOrEqual(4.5);
  });
});

describe('diff text contrast', () => {
  const pairs = DIFF_TEXT.flatMap((fg) => DIFF_BACKGROUNDS.map((bg) => [fg, bg] as const));
  test.each(pairs)('%s is readable on %s', (fg, bg) => {
    expectFloor(fg, bg, 4.5);
  });
});

describe('unchanged tokens keep their values', () => {
  const baseline: Record<string, string> = {
    '--color-success': '#22c55e',
    '--color-error': '#ef4444',
    '--color-warning': '#f59e0b',
    '--color-text-muted': '#94a3b8',
    '--color-text-secondary': '#64748b',
    '--color-surface-hover': '#f1f5f9',
    '--color-surface-armed': '#eff6ff',
    '--color-border': '#e2e8f0',
    '--color-border-focus': '#3b82f6',
    '--color-find-match': '#fef08a',
    '--color-find-match-fg': '#1e293b',
    '--color-find-active': '#f59e0b',
    '--color-find-active-fg': '#0f172a',
    '--color-find-gutter': '#fbbf24',
  };
  test.each(Object.entries(baseline))('%s stays %s', (name, value) => {
    expect(token(name), `${name} changed`).toBe(value);
  });
});
