/**
 * ValidateView component tests.
 *
 * Component contract:
 *  - Document-level Validate panel with a "Run checks" action
 *    (data-testid="validate-run") and a profile selector
 *    (data-testid="validate-profile"): pdfa-1b (default) and
 *    pdfua-1-structural.
 *  - Running calls the Validate(tabId, profile) binding and renders the
 *    returned problems GROUPED BY SEVERITY: data-testid="validate-group-error"
 *    and data-testid="validate-group-warning".
 *  - Each problem is a row (data-testid="validate-problem"). Clicking a problem
 *    that carries an objNodeId jumps the tree via onNavigate(objNodeId)
 *    (the existing NAVIGATE_TO_REF wiring). A problem with an empty (or
 *    unresolvable) objNodeId is shown but NOT clickable -- onNavigate is never
 *    called (the no-jump fallback).
 *  - Problems without an object ref (document-level, e.g. missing /Lang) are
 *    surfaced under a "Document" group.
 *  - Empty/clean result shows data-testid="validate-empty" reading
 *    "no structural problems found (structural checks only)".
 *  - The not-authoritative disclaimer (data-testid="validate-disclaimer",
 *    "structural checks only") is ALWAYS visible, and NO authoritative
 *    conformance verdict language appears anywhere.
 *
 * Run: cd frontend && npx vitest run src/components/ValidateView.test.tsx
 */
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { ValidateView } from './ValidateView';

const mockValidate = vi.fn();
vi.mock(
  '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js',
  () => ({
    Validate: (...a: unknown[]) => mockValidate(...a),
  })
);

/** An object-scoped PDF/A-1b error carrying the tree node id for jump. */
const fontError = {
  ruleId: 'font-embedding',
  profile: 'pdfa-1b',
  severity: 'error',
  message: 'font /F1 is not embedded',
  objRef: '4 0 R',
  objNodeId: 'obj:0:4',
  specRef: 'ISO 19005-1:2005, 6.3.4',
};

/** A document-level problem: no object ref -> "Document" group, no jump. */
const langProblem = {
  ruleId: 'lang',
  profile: 'pdfa-1b',
  severity: 'warning',
  message: 'document /Lang is missing',
  objRef: '',
  objNodeId: '',
  specRef: 'ISO 14289-1:2014, 7.2',
};

/** A mixed result: one error (jump-capable) + one document-level warning. */
const mixedResult = {
  profile: 'pdfa-1b',
  summary: { errors: 1, warnings: 1 },
  problems: [fontError, langProblem],
};

/** A clean-for-profile result: zero problems. */
const cleanResult = {
  profile: 'pdfua-1-structural',
  summary: { errors: 0, warnings: 0 },
  problems: [],
};

/** Authoritative conformance verdicts the panel must never render. */
const forbiddenVerdicts = [
  'pdf/a compliant',
  'pdf/a-compliant',
  'is compliant',
  'fully compliant',
  'conformant',
  'is valid',
  'valid pdf/a',
  'passed validation',
];

function assertNoVerdict(text: string) {
  const low = text.toLowerCase();
  for (const p of forbiddenVerdicts) {
    expect(low, `authoritative verdict "${p}" present`).not.toContain(p);
  }
}

beforeEach(() => {
  vi.clearAllMocks();
  mockValidate.mockResolvedValue(mixedResult);
});

describe('ValidateView', () => {
  // "Run checks" invokes Validate(tabId, profile) with the default profile and
  // renders problems grouped by severity.
  test('run checks fetches with default profile and groups by severity', async () => {
    render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    fireEvent.click(screen.getByTestId('validate-run'));

    await waitFor(() => expect(mockValidate).toHaveBeenCalledWith('tab-1', 'pdfa-1b'));
    await waitFor(() => expect(screen.getByTestId('validate-group-error')).toBeInTheDocument());
    expect(screen.getByTestId('validate-group-error').textContent).toContain('not embedded');
    expect(screen.getByTestId('validate-group-warning')).toBeInTheDocument();
    expect(screen.getByTestId('validate-group-warning').textContent).toContain('/Lang is missing');
  });

  // Clicking an object-scoped problem jumps the tree via
  // onNavigate(objNodeId).
  test('clicking an object-scoped problem navigates to its node', async () => {
    const onNavigate = vi.fn();
    render(<ValidateView tabId="tab-1" active onNavigate={onNavigate} />);

    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => screen.getByText(/not embedded/i));

    const row = screen.getByText(/not embedded/i).closest('[data-testid="validate-problem"]');
    expect(row).not.toBeNull();
    fireEvent.click(row as Element);
    expect(onNavigate).toHaveBeenCalledWith('obj:0:4');
  });

  // A problem with an empty objNodeId is shown but NOT clickable -- the
  // no-jump fallback (never a broken navigation).
  test('document-level problem does not navigate on click', async () => {
    const onNavigate = vi.fn();
    render(<ValidateView tabId="tab-1" active onNavigate={onNavigate} />);

    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => screen.getByText(/\/Lang is missing/i));

    const row = screen.getByText(/\/Lang is missing/i).closest('[data-testid="validate-problem"]');
    expect(row).not.toBeNull();
    fireEvent.click(row as Element);
    expect(onNavigate).not.toHaveBeenCalled();
  });

  // object-ref-less problems are surfaced as the "Document" group via a
  // dedicated label (not merely because the message text happens to contain
  // "document").
  test('document-level problems carry a Document label', async () => {
    render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => screen.getByText(/\/Lang is missing/i));
    // The doc-level row for the missing /Lang warning must render the Document
    // marker; an object-scoped row must NOT.
    const docLabels = screen.getAllByTestId('validate-doc-label');
    expect(docLabels.length).toBe(1);
    expect(docLabels[0].textContent).toMatch(/document/i);
    const langRow = screen.getByText(/\/Lang is missing/i).closest('[data-testid="validate-problem"]');
    expect(langRow?.querySelector('[data-testid="validate-doc-label"]')).not.toBeNull();
  });

  // The not-authoritative disclaimer is always visible (before and after a
  // run) and no conformance verdict language appears.
  test('disclaimer always visible, never a conformance verdict', async () => {
    const { container } = render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    // Visible before running.
    expect(screen.getByTestId('validate-disclaimer').textContent?.toLowerCase()).toContain(
      'structural checks only'
    );
    assertNoVerdict(container.textContent ?? '');

    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => screen.getByTestId('validate-group-error'));

    // Still visible after results render.
    expect(screen.getByTestId('validate-disclaimer')).toBeInTheDocument();
    assertNoVerdict(container.textContent ?? '');
  });

  // A clean (zero-problem) result shows the explicit no-problems state --
  // not a "compliant/valid" verdict.
  test('clean result shows the no-problems state', async () => {
    mockValidate.mockResolvedValue(cleanResult);
    const { container } = render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => expect(screen.getByTestId('validate-empty')).toBeInTheDocument());
    expect(screen.getByTestId('validate-empty').textContent?.toLowerCase()).toContain(
      'no structural problems found'
    );
    assertNoVerdict(container.textContent ?? '');
  });

  // The profile selector offers both profiles and a run uses the chosen one.
  test('profile selector drives the validated profile', async () => {
    render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    const select = screen.getByTestId('validate-profile') as HTMLSelectElement;
    const optionValues = Array.from(select.options).map((o) => o.value);
    expect(optionValues).toContain('pdfa-1b');
    expect(optionValues).toContain('pdfua-1-structural');

    fireEvent.change(select, { target: { value: 'pdfua-1-structural' } });
    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() =>
      expect(mockValidate).toHaveBeenCalledWith('tab-1', 'pdfua-1-structural')
    );
  });
});
