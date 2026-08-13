/**
 * Story 14.3: DiffView depth-cap truncation display branch.
 *
 * DiffView's `identical` const (DiffView.tsx) mirrors Go's diffIsIdentical, and
 * that includes `summary.truncatedSubtrees === 0`. Given a result whose walk was
 * bounded by the depth cap (truncatedSubtrees > 0) but whose visible node counts
 * are all zero, the component must NOT compute identical === true and must NOT
 * render the "No structural differences" banner without a truncation marker --
 * the quiet lie this story closes, mirrored on the GUI surface.
 *
 * This is the thin display branch of a backend-verified field, kept at the
 * component level (NOT E2E).
 *
 * Run: cd frontend && npx vitest run src/components/DiffView.truncation.test.tsx
 */
import { render, screen, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { DiffView } from './DiffView';

const mockDiffDocuments = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    DiffDocuments: (...a: unknown[]) => mockDiffDocuments(...a),
  })
);

/**
 * A diff whose visible node counts are all zero but whose walk was bounded by
 * the depth cap (truncatedSubtrees > 0). Under the bug this reports identical;
 * post-fix it must NOT. The single cut node carries `truncated: true`.
 */
const depthCappedResult = {
  summary: {
    added: 0,
    removed: 0,
    changed: 0,
    pageCountLeft: 1,
    pageCountRight: 1,
    versionChanged: false,
    encryptionChanged: false,
    infoChanged: false,
    xmpChanged: false,
    // Additive field surfaced by the Go DiffSummary; declared on
    // DiffSummaryData in DiffView.tsx.
    truncatedSubtrees: 1,
  },
  root: {
    path: '/Root',
    status: 'unchanged',
    kind: 'dict',
    changedKeys: [] as string[],
    leftSummary: '',
    rightSummary: '',
    children: [
      {
        path: '/Root/Deep',
        status: 'unchanged',
        kind: 'ref',
        changedKeys: [] as string[],
        leftSummary: '<< /L <ref> >>',
        rightSummary: '<< /L <ref> >>',
        truncated: true,
        children: [],
      },
    ],
  },
};

beforeEach(() => {
  vi.clearAllMocks();
  mockDiffDocuments.mockResolvedValue(depthCappedResult);
});

describe('DiffView depth-cap truncation', () => {
  // A result with truncatedSubtrees > 0 must NOT render the "No structural
  // differences / identical" banner -- the walk was bounded, so identity cannot
  // be claimed.
  test('suppresses the identical banner when a subtree was depth-capped', async () => {
    render(<DiffView leftTabId="left" rightTabId="right" active />);

    const summary = await screen.findByTestId('diff-summary');
    const text = (summary.textContent ?? '').toLowerCase();
    expect(text).not.toMatch(/no structural differences|no differ|identical/);
  });

  // The per-node [truncated: depth cap] ROW renders, not just the summary note.
  // The depth-capped node reports status "unchanged", so
  // hasDelta must treat `truncated` as a delta for its ancestors to auto-expand;
  // otherwise the marker sits under an unexpanded ancestor and is unreachable.
  // Asserts the bracketed row text (distinct from the summary note's "truncated
  // at the depth cap") AND the cut node's path, so it genuinely covers the
  // DiffView.tsx per-node marker branch rather than passing on the summary note.
  test('auto-expands to the depth-cap node and renders its row marker', async () => {
    render(<DiffView leftTabId="left" rightTabId="right" active />);

    await waitFor(() => expect(mockDiffDocuments).toHaveBeenCalled());
    await screen.findByTestId('diff-summary');

    // The cut node itself is rendered (its ancestors auto-expanded to reach it).
    expect(screen.getAllByText('/Root/Deep').length).toBeGreaterThan(0);
    // ...carrying the per-node marker: bracketed row text, which the summary
    // note ("... truncated at the depth cap ...") does not contain.
    expect(screen.getAllByText(/\[truncated: depth cap\]/).length).toBeGreaterThan(0);
  });
});
