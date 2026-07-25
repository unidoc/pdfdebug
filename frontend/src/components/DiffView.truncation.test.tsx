/**
 * Story 14.3: DiffView depth-cap truncation display branch (AC5, 14.3-COMP-001).
 *
 * RED PHASE: DiffView's `identical` const (DiffView.tsx) mirrors Go's
 * diffIsIdentical over the node counts + document flags only; it does not yet
 * account for `summary.truncatedSubtrees`. Given a result whose walk was bounded
 * by the depth cap (truncatedSubtrees > 0) but whose visible node counts are all
 * zero, the component today computes identical === true and renders the
 * "No structural differences" banner with NO truncation marker -- the exact
 * quiet lie this story closes, mirrored on the GUI surface.
 *
 * GREEN target: `identical` gains `&& s.truncatedSubtrees === 0`, so the banner
 * is suppressed, and a depth-cap marker is rendered so the bounded walk is
 * visible. This is the thin display branch of a backend-verified field, kept at
 * the component level (NOT E2E).
 *
 * Test files are excluded from the app tsc build, so the `truncatedSubtrees`
 * field (not yet on DiffSummaryData) does not break `npm run typecheck`; only
 * vitest exercises this file.
 *
 * Naming: 14.3-COMP-001 [P1].
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
    // Additive field surfaced by the Go DiffSummary (AC2). Cast through unknown
    // because DiffSummaryData does not declare it yet (red-phase seam).
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

describe('DiffView depth-cap truncation (Story 14.3)', () => {
  // 14.3-COMP-001 [P1] AC5: a result with truncatedSubtrees > 0 must NOT render
  // the "No structural differences / identical" banner -- the walk was bounded,
  // so identity cannot be claimed.
  test('14.3-COMP-001 suppresses the identical banner when a subtree was depth-capped', async () => {
    render(<DiffView leftTabId="left" rightTabId="right" active />);

    const summary = await screen.findByTestId('diff-summary');
    const text = (summary.textContent ?? '').toLowerCase();
    expect(text).not.toMatch(/no structural differences|no differ|identical/);
  });

  // 14.3-COMP-001 [P1] AC5: the depth-cap marker is rendered somewhere in the
  // view so the bounded walk is visible to the user (mirrors the CLI marker).
  test('14.3-COMP-001 renders a depth-cap truncation marker', async () => {
    const { container } = render(<DiffView leftTabId="left" rightTabId="right" active />);

    await waitFor(() => expect(mockDiffDocuments).toHaveBeenCalled());
    await screen.findByTestId('diff-summary');
    const body = (container.textContent ?? '').toLowerCase();
    expect(body).toMatch(/truncat|depth cap/);
  });
});
