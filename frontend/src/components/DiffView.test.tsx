/**
 * Story 13.6: DiffView side-by-side comparison component tests (AC 5, 6).
 *
 * RED PHASE: the `./DiffView` import below is the failing seam -- the component
 * does not exist yet, so this file fails to collect until Task 4.1 lands it
 * (mirrors the 13.4 SignaturesView / 13.5 ValidateView red-phase precedent).
 * Test files are excluded from the app build/typecheck via the tsconfig
 * `exclude` test glob, so this red file never breaks `npm run build` or
 * `npm run typecheck`; only `vitest run` shows it red.
 *
 * Component contract (from the story AC5):
 *  - On mount (active), DiffView fetches DiffDocuments(leftTabId, rightTabId)
 *    and renders a summary header (data-testid="diff-summary") with the
 *    added/removed/changed counts and the high-signal facts (page-count change).
 *  - Two synchronized tree panes: data-testid="diff-tree-left" and
 *    data-testid="diff-tree-right".
 *  - Each rendered node is a row (data-testid="diff-node") carrying its status
 *    via data-status ("added" | "removed" | "changed" | "unchanged") so it can
 *    be color-coded.
 *  - Unchanged subtrees are COLLAPSED by default; the path leading to a change
 *    is auto-expanded so the change is visible without interaction.
 *  - A "next change" / "prev change" navigation
 *    (data-testid="diff-next-change" / "diff-prev-change") moves the selection
 *    through the changed/added/removed nodes; the selected node carries
 *    data-selected="true".
 *  - Selecting a changed node shows the per-key/value detail
 *    (data-testid="diff-detail") listing the changed keys and left-vs-right
 *    values.
 *
 * Naming: 13.6-UNIT-2NN [Px].
 * Run: cd frontend && npx vitest run src/components/DiffView.test.tsx
 */
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { DiffView } from './DiffView';

const mockDiffDocuments = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    DiffDocuments: (...a: unknown[]) => mockDiffDocuments(...a),
  })
);

/** A changed scalar leaf: MediaBox height 792 -> 842. */
const changedLeaf = {
  path: '/Root/Pages/Kids[0]/MediaBox/[3]',
  status: 'changed',
  kind: 'scalar',
  changedKeys: [] as string[],
  leftSummary: '792',
  rightSummary: '842',
  children: [],
};

/** The page dict whose MediaBox key changed. */
const pageNode = {
  path: '/Root/Pages/Kids[0]',
  status: 'changed',
  kind: 'dict',
  changedKeys: ['MediaBox'],
  leftSummary: '',
  rightSummary: '',
  children: [
    {
      path: '/Root/Pages/Kids[0]/MediaBox',
      status: 'changed',
      kind: 'array',
      changedKeys: [] as string[],
      leftSummary: '[0 0 612 792]',
      rightSummary: '[0 0 612 842]',
      children: [changedLeaf],
    },
  ],
};

const pagesNode = {
  path: '/Root/Pages',
  status: 'changed',
  kind: 'dict',
  changedKeys: [] as string[],
  leftSummary: '',
  rightSummary: '',
  children: [pageNode],
};

/** An object present only on the right (added). */
const addedNode = {
  path: '/Root/Metadata',
  status: 'added',
  kind: 'dict',
  changedKeys: [] as string[],
  leftSummary: '',
  rightSummary: '<< /Type /Metadata /Subtype /XML >>',
  children: [],
};

/** A FULLY unchanged sibling subtree with a unique leaf ("/GoToUnchanged"). */
const unchangedBranch = {
  path: '/Root/OpenAction',
  status: 'unchanged',
  kind: 'dict',
  changedKeys: [] as string[],
  leftSummary: '',
  rightSummary: '',
  children: [
    {
      path: '/Root/OpenAction/S',
      status: 'unchanged',
      kind: 'scalar',
      changedKeys: [] as string[],
      leftSummary: '/GoToUnchanged',
      rightSummary: '/GoToUnchanged',
      children: [],
    },
  ],
};

const root = {
  path: '/Root',
  status: 'changed',
  kind: 'dict',
  changedKeys: ['Metadata'],
  leftSummary: '',
  rightSummary: '',
  children: [pagesNode, addedNode, unchangedBranch],
};

/** A diff with 1 added, 0 removed, 2 changed objects and a page-count delta. */
const diffResult = {
  summary: {
    added: 1,
    removed: 0,
    changed: 2,
    pageCountLeft: 1,
    pageCountRight: 2,
    versionChanged: false,
    encryptionChanged: false,
    infoChanged: false,
    xmpChanged: false,
  },
  root,
};

/** A zero-delta (identical) diff. */
const identicalResult = {
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
  },
  root: { ...unchangedBranch, path: '/Root', status: 'unchanged' },
};

beforeEach(() => {
  vi.clearAllMocks();
  mockDiffDocuments.mockResolvedValue(diffResult);
});

describe('DiffView', () => {
  // 13.6-UNIT-201 [P0] AC5: DiffView fetches DiffDocuments(left, right) and
  // renders a summary header with the added/removed/changed counts.
  test('fetches the diff and renders the summary counts', async () => {
    render(<DiffView leftTabId="left" rightTabId="right" active />);

    await waitFor(() => expect(mockDiffDocuments).toHaveBeenCalledWith('left', 'right'));
    const summary = await screen.findByTestId('diff-summary');
    const text = (summary.textContent ?? '').toLowerCase();
    expect(text).toContain('added');
    expect(text).toContain('removed');
    expect(text).toContain('changed');
    // The actual counts must appear (1 added, 2 changed).
    expect(summary.textContent).toMatch(/\b1\b/);
    expect(summary.textContent).toMatch(/\b2\b/);
  });

  // 13.6-UNIT-202 [P0] AC5: two synchronized tree panes are rendered.
  test('renders synchronized left and right tree panes', async () => {
    render(<DiffView leftTabId="left" rightTabId="right" active />);

    await screen.findByTestId('diff-tree-left');
    expect(screen.getByTestId('diff-tree-left')).toBeInTheDocument();
    expect(screen.getByTestId('diff-tree-right')).toBeInTheDocument();
  });

  // 13.6-UNIT-203 [P0] AC5: nodes carry their status via data-status for
  // color-coding; added and changed statuses are both present.
  test('nodes expose data-status for color-coding', async () => {
    const { container } = render(<DiffView leftTabId="left" rightTabId="right" active />);

    await screen.findByTestId('diff-summary');
    const statuses = Array.from(container.querySelectorAll('[data-testid="diff-node"]')).map(
      (n) => n.getAttribute('data-status')
    );
    expect(statuses).toContain('added');
    expect(statuses).toContain('changed');
  });

  // 13.6-UNIT-204 [P0] AC5: unchanged subtrees collapse by default, but the path
  // to a change is auto-expanded (the changed leaf value is visible; the
  // unchanged-only branch's unique leaf is not).
  test('unchanged subtrees collapse; change paths auto-expand', async () => {
    render(<DiffView leftTabId="left" rightTabId="right" active />);

    await screen.findByTestId('diff-summary');
    // The changed MediaBox value is visible without interaction.
    expect(screen.getAllByText(/842/).length).toBeGreaterThan(0);
    // The fully-unchanged branch's unique leaf is NOT rendered by default.
    expect(screen.queryByText(/GoToUnchanged/)).toBeNull();
  });

  // 13.6-UNIT-205 [P1] AC5: "next change" navigation selects a non-unchanged
  // node; the selected node carries data-selected="true".
  test('next-change selects the next changed node', async () => {
    const { container } = render(<DiffView leftTabId="left" rightTabId="right" active />);

    await screen.findByTestId('diff-next-change');
    fireEvent.click(screen.getByTestId('diff-next-change'));

    await waitFor(() => {
      const selected = container.querySelectorAll('[data-testid="diff-node"][data-selected="true"]');
      expect(selected.length).toBe(1);
      expect(selected[0].getAttribute('data-status')).not.toBe('unchanged');
    });
  });

  // 13.6-UNIT-206 [P1] AC5: selecting a changed node shows the per-key/value
  // detail (changed key name + left-vs-right values).
  test('selecting a changed node shows key/value detail', async () => {
    render(<DiffView leftTabId="left" rightTabId="right" active />);

    await screen.findByTestId('diff-next-change');
    fireEvent.click(screen.getByTestId('diff-next-change'));

    const detail = await screen.findByTestId('diff-detail');
    const text = within(detail).getByText;
    // At least one navigated change surfaces its left vs right values.
    await waitFor(() => {
      expect(detail.textContent).toMatch(/792/);
      expect(detail.textContent).toMatch(/842/);
    });
    expect(text).toBeTypeOf('function');
  });

  // 13.6-UNIT-207 [P1] AC3/AC6: the summary header surfaces the page-count
  // change (1 -> 2).
  test('summary surfaces the page-count change', async () => {
    render(<DiffView leftTabId="left" rightTabId="right" active />);

    const summary = await screen.findByTestId('diff-summary');
    expect(summary.textContent?.toLowerCase()).toContain('page');
    // Both the old and new page counts appear.
    expect(summary.textContent).toMatch(/1/);
    expect(summary.textContent).toMatch(/2/);
  });

  // 13.6-UNIT-208 [P2] AC6: an identical (zero-delta) diff renders an explicit
  // "no differences" state rather than an empty view.
  test('identical documents show a zero-delta state', async () => {
    mockDiffDocuments.mockResolvedValue(identicalResult);
    render(<DiffView leftTabId="left" rightTabId="right" active />);

    const summary = await screen.findByTestId('diff-summary');
    expect(summary.textContent?.toLowerCase()).toMatch(/no differ|identical|0 added/);
  });

  // 13.6-UNIT-209 [P1] AC6: a failed DiffDocuments (parse failure on the
  // comparison file) surfaces a clear error state, not a stuck spinner or a
  // crash - the isolation contract that a broken second file does not take down
  // the view.
  test('a failed diff surfaces the error state', async () => {
    mockDiffDocuments.mockRejectedValue(new Error('could not parse comparison file'));
    render(<DiffView leftTabId="left" rightTabId="right" active />);

    const err = await screen.findByTestId('diff-error');
    expect(err.textContent).toContain('could not parse comparison file');
    // The loading placeholder must not linger once the error is shown.
    expect(screen.queryByTestId('diff-loading')).toBeNull();
  });

  // 13.6-UNIT-210 [P1] AC5: "prev change" navigation (the mirror of next-change)
  // selects a non-unchanged node. With nothing selected yet, prev wraps to the
  // LAST change - exercising navChange(-1)'s distinct index/wrap path.
  test('prev-change selects a changed node', async () => {
    const { container } = render(<DiffView leftTabId="left" rightTabId="right" active />);

    await screen.findByTestId('diff-prev-change');
    fireEvent.click(screen.getByTestId('diff-prev-change'));

    await waitFor(() => {
      const selected = container.querySelectorAll('[data-testid="diff-node"][data-selected="true"]');
      expect(selected.length).toBe(1);
      expect(selected[0].getAttribute('data-status')).not.toBe('unchanged');
    });
  });
});
