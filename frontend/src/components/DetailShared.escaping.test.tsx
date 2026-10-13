/**
 * ValueDisplay escapes control characters in a decoded PDF value.
 *
 * The backend sends the decoded string unescaped so the JSON contract stays
 * lossless, which leaves a C1 byte from the Latin-1 decode fallback invisible
 * and an embedded newline collapsed into its neighbours unless the row escapes
 * it. The tree row does; so must the detail view.
 *
 * Run: cd frontend && npx vitest run src/components/DetailShared.escaping.test.tsx
 */
import { render, screen } from '@testing-library/react';
import { describe, test, expect } from 'vitest';
import { ValueDisplay, type ValueEntryData } from './DetailShared';

function entry(display: string, type = 'string'): ValueEntryData {
  return { type, display, raw: '', refTarget: '' };
}

describe('ValueDisplay control-character escaping', () => {
  test('an embedded newline renders as a two-character escape', () => {
    const { container } = render(<ValueDisplay value={entry('Rapport\ncell')} />);
    expect(container.textContent).toBe('Rapport\\ncell');
  });

  test('a C1 control from the Latin-1 fallback renders visibly', () => {
    const { container } = render(<ValueDisplay value={entry('don\u0092t')} />);
    expect(container.textContent).toBe('don\\x92t');
  });

  test('a literal backslash is doubled so the escaping stays reversible', () => {
    const { container } = render(<ValueDisplay value={entry('a\\b')} />);
    expect(container.textContent).toBe('a\\\\b');
  });

  test('ordinary text and non-string values are untouched', () => {
    const { container } = render(<ValueDisplay value={entry('/Catalog', 'name')} />);
    expect(container.textContent).toBe('/Catalog');
  });

  test('a reference keeps its click target while its text is escaped', () => {
    render(
      <ValueDisplay
        value={{ type: 'reference', display: '7 0 R', raw: '7 0 R', refTarget: '7 0 R' }}
      />,
    );
    const span = screen.getByRole('button');
    expect(span.getAttribute('data-ref-target')).toBe('7 0 R');
    expect(span.textContent).toBe('7 0 R');
  });
});
