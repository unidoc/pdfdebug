/**
 * DiffView escapes control characters in the summaries it renders.
 *
 * The summaries carry decoded text, so the same rule the CLI applies to its
 * diff lines applies here: a C1 control from the Latin-1 decode fallback must
 * not reach the screen invisible, and a newline must not collapse two values
 * into one run of text.
 *
 * Run: cd frontend && npx vitest run src/components/DiffView.escaping.test.tsx
 */
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { DiffView } from './DiffView';

const mockDiffDocuments = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    DiffDocuments: (...a: unknown[]) => mockDiffDocuments(...a),
  }),
);

const changedAlt = {
  path: '/Root/StructTreeRoot/Alt',
  status: 'changed',
  kind: 'scalar',
  changedKeys: [] as string[],
  leftSummary: 'don\u0092t\nstop',
  rightSummary: 'do\tstop',
  children: [],
};

const root = {
  path: '/Root',
  status: 'changed',
  kind: 'dict',
  changedKeys: ['StructTreeRoot'],
  leftSummary: '',
  rightSummary: '',
  children: [changedAlt],
};

beforeEach(() => {
  vi.clearAllMocks();
  mockDiffDocuments.mockResolvedValue({
    root,
    summary: { added: 0, removed: 0, changed: 1, unchanged: 0, truncatedSubtrees: 0 },
  });
});

describe('DiffView summary escaping', () => {
  test('the tree rows escape control characters on both sides', async () => {
    render(<DiffView leftTabId="left" rightTabId="right" active />);

    const left = await screen.findByTestId('diff-tree-left');
    const right = screen.getByTestId('diff-tree-right');

    expect(left.textContent).toContain('don\\x92t\\nstop');
    expect(right.textContent).toContain('do\\tstop');
  });

  test('the selected-node detail escapes them too', async () => {
    render(<DiffView leftTabId="left" rightTabId="right" active />);

    const rows = await screen.findAllByTestId('diff-node');
    const row = rows.find((r) => (r.textContent ?? '').includes('/Root/StructTreeRoot/Alt'));
    expect(row).toBeTruthy();
    fireEvent.click(row!);

    const detail = await screen.findByTestId('diff-detail');
    expect(detail.textContent).toContain('don\\x92t\\nstop');
    expect(detail.textContent).toContain('do\\tstop');
  });
});
