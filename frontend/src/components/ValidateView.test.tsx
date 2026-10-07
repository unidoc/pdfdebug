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
 *  - Every result shows the profile's scope sentence
 *    (data-testid="validate-scope") and the rules checked
 *    (data-testid="validate-rules", one data-testid="validate-rule" each)
 *    above the result.
 *  - Empty/clean result shows data-testid="validate-empty" naming the rule
 *    count: "None of the N rules checked found a problem".
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
  scope: 'A structural subset of PDF/A-1b limited to the rules listed.',
  rules: [
    {
      ruleId: 'font-embedding',
      specRef: 'ISO 19005-1:2005, 6.3.4',
      severity: 'error',
      checks: 'each non-Type3 font has /FontFile, /FontFile2 or /FontFile3 (Type3 exempt)',
      evaluated: true,
      findings: 1,
    },
    {
      ruleId: 'lang',
      specRef: 'ISO 14289-1:2014, 7.2',
      severity: 'warning',
      checks: 'catalog /Lang is a non-empty string',
      evaluated: true,
      findings: 1,
    },
  ],
};

/** A clean-for-profile result: zero problems. */
const cleanResult = {
  profile: 'pdfua-1-structural',
  summary: { errors: 0, warnings: 0 },
  problems: [],
  scope: 'A structural subset of PDF/UA-1 made of catalog-level checks.',
  rules: [
    {
      ruleId: 'lang',
      specRef: 'ISO 14289-1:2014, 7.2',
      severity: 'warning',
      checks: 'catalog /Lang is a non-empty string',
      evaluated: true,
      findings: 0,
    },
  ],
};

/** Authoritative conformance verdicts the panel must never render. */
const forbiddenVerdicts = [
  'pdf/a compliant',
  'pdf/a-compliant',
  'pdf/ua compliant',
  'is compliant',
  'fully compliant',
  'conformant',
  'is valid',
  'valid pdf/a',
  'pdf/a valid',
  'passed validation',
  'validation passed',
  'compliance: pass',
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

  // A clean (zero-problem) result names how many rules found nothing -- not a
  // "compliant/valid" verdict.
  test('clean result shows the no-problems state', async () => {
    mockValidate.mockResolvedValue(cleanResult);
    const { container } = render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => expect(screen.getByTestId('validate-empty')).toBeInTheDocument());
    expect(screen.getByTestId('validate-empty').textContent).toContain(
      'None of the 1 rule checked found a problem'
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

/** Rule entries mirroring `pdfcore.RuleInfo` for the pdfua-1-structural profile. */
const uaRules = [
  {
    ruleId: 'marked',
    specRef: 'ISO 14289-1:2014, 7.1',
    severity: 'warning',
    checks: 'catalog /MarkInfo /Marked is true',
    evaluated: true,
    findings: 0,
  },
  {
    ruleId: 'struct-tree-root',
    specRef: 'ISO 14289-1:2014, 7.1',
    severity: 'warning',
    checks: 'catalog has a /StructTreeRoot key (presence only)',
    evaluated: true,
    findings: 0,
  },
  {
    ruleId: 'lang',
    specRef: 'ISO 14289-1:2014, 7.2',
    severity: 'warning',
    checks: 'catalog /Lang is a non-empty string',
    evaluated: true,
    findings: 0,
  },
];

const uaScope =
  'a structural subset of PDF/UA-1 made of catalog-level checks; marked content, structure tree contents, alternate text and fonts are not examined; not a PDF/UA-1 conformance check';

/** The disclaimer text, identical to pdfcore.DisclaimerText. */
const DISCLAIMER_TEXT =
  'structural checks only - not full conformance; use veraPDF for authoritative PDF/A / PDF/UA validation';

/** A clean pdfua-1-structural result carrying its scope and rule list. */
const scopedCleanResult = {
  profile: 'pdfua-1-structural',
  summary: { errors: 0, warnings: 0, info: 0 },
  problems: [],
  disclaimer: DISCLAIMER_TEXT,
  scope: uaScope,
  rules: uaRules,
};

/** A pdfa-1b result with one object-scoped finding and one degraded rule. */
const scopedProblemResult = {
  profile: 'pdfa-1b',
  summary: { errors: 1, warnings: 0, info: 1 },
  problems: [
    fontError,
    {
      ruleId: 'xmp-metadata',
      profile: 'pdfa-1b',
      severity: 'info',
      message: 'rule "xmp-metadata" could not be evaluated: boom',
      objRef: '',
      objNodeId: '',
      specRef: 'ISO 19005-1:2005, 6.7.2/6.7.3',
    },
  ],
  disclaimer: DISCLAIMER_TEXT,
  scope: 'a structural subset of PDF/A-1b limited to the rules listed',
  rules: [
    {
      ruleId: 'font-embedding',
      specRef: 'ISO 19005-1:2005, 6.3.4',
      severity: 'error',
      checks: 'every non-Type3 font has /FontFile, /FontFile2 or /FontFile3',
      evaluated: true,
      findings: 1,
    },
    {
      ruleId: 'no-encryption',
      specRef: 'ISO 19005-1:2005, 6.1.3',
      severity: 'error',
      checks: 'trailer has no /Encrypt',
      evaluated: true,
      findings: 0,
    },
    {
      ruleId: 'xmp-metadata',
      specRef: 'ISO 19005-1:2005, 6.7.2/6.7.3',
      severity: 'error',
      checks: 'XMP packet present; /Info keys present in both compared',
      evaluated: false,
      findings: 0,
    },
  ],
};

/** True when node a precedes node b in document order. */
function precedes(a: Element, b: Element) {
  return Boolean(a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING);
}

describe('ValidateView scope and rules checked', () => {
  test('clean result shows the scope sentence and every rule checked', async () => {
    mockValidate.mockResolvedValue(scopedCleanResult);
    const { container } = render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => expect(screen.getByTestId('validate-scope')).toBeInTheDocument());

    expect(screen.getByTestId('validate-scope').textContent).toContain(uaScope);
    const list = screen.getByTestId('validate-rules');
    expect(list.textContent).toMatch(/rules checked \(3\)/i);
    const rows = screen.getAllByTestId('validate-rule');
    expect(rows).toHaveLength(uaRules.length);
    uaRules.forEach((rule, i) => {
      const text = rows[i].textContent ?? '';
      expect(text).toContain(rule.ruleId);
      expect(text).toContain(rule.specRef);
      expect(text).toContain(rule.checks);
      expect(text).toContain('0 found');
    });
    assertNoVerdict(container.textContent ?? '');
  });

  test('clean state names the rule count instead of an unscoped claim', async () => {
    mockValidate.mockResolvedValue(scopedCleanResult);
    const { container } = render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => expect(screen.getByTestId('validate-empty')).toBeInTheDocument());

    const clean = screen.getByTestId('validate-empty').textContent ?? '';
    expect(clean.toLowerCase()).toContain('none');
    expect(clean).toMatch(/\b3 rules\b/);
    expect((container.textContent ?? '').toLowerCase()).not.toContain('no structural problems found');
    expect(precedes(screen.getByTestId('validate-rules'), screen.getByTestId('validate-empty'))).toBe(true);
  });

  test('clean sentence follows the rule count of the result', async () => {
    mockValidate.mockResolvedValue({ ...scopedCleanResult, rules: uaRules.slice(0, 2) });
    render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => expect(screen.getByTestId('validate-empty')).toBeInTheDocument());

    expect(screen.getByTestId('validate-empty').textContent).toMatch(/\b2 rules\b/);
    expect(screen.getAllByTestId('validate-rule')).toHaveLength(2);
  });

  test('result with problems shows scope and rules above the summary', async () => {
    mockValidate.mockResolvedValue(scopedProblemResult);
    render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => expect(screen.getByTestId('validate-rules')).toBeInTheDocument());

    expect(screen.getByTestId('validate-scope').textContent).toContain(scopedProblemResult.scope);
    const rows = screen.getAllByTestId('validate-rule');
    expect(rows).toHaveLength(3);
    expect(rows[0].textContent).toContain('font-embedding');
    expect(rows[0].textContent).toContain('1 found');
    expect(rows[1].textContent).toContain('0 found');
    expect(rows[2].textContent).toContain('xmp-metadata');
    expect(rows[2].textContent).toContain('not evaluated');
    expect(rows[2].textContent).not.toContain('0 found');

    expect(precedes(screen.getByTestId('validate-rules'), screen.getByTestId('validate-summary'))).toBe(true);
    expect(precedes(screen.getByTestId('validate-scope'), screen.getByTestId('validate-rules'))).toBe(true);
    expect(screen.queryByTestId('validate-empty')).toBeNull();
  });

  test('jump to object still works with the rules list shown', async () => {
    mockValidate.mockResolvedValue(scopedProblemResult);
    const onNavigate = vi.fn();
    render(<ValidateView tabId="tab-1" active onNavigate={onNavigate} />);

    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => screen.getByText(/not embedded/i));

    const row = screen.getByText(/not embedded/i).closest('[data-testid="validate-problem"]');
    expect(row).not.toBeNull();
    fireEvent.click(row as Element);
    expect(onNavigate).toHaveBeenCalledWith('obj:0:4');
  });

  test('no scope or rules list before a run', () => {
    render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    expect(screen.queryByTestId('validate-scope')).toBeNull();
    expect(screen.queryByTestId('validate-rules')).toBeNull();
  });

  test('no scope or rules list when the run fails', async () => {
    mockValidate.mockRejectedValue(new Error('validate failed'));
    render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => expect(screen.getByTestId('validate-error')).toBeInTheDocument());

    expect(screen.queryByTestId('validate-scope')).toBeNull();
    expect(screen.queryByTestId('validate-rules')).toBeNull();
  });

  test('a failed rerun hides the previous scope and rules list', async () => {
    mockValidate.mockResolvedValueOnce(scopedCleanResult);
    render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => expect(screen.getByTestId('validate-rules')).toBeInTheDocument());

    mockValidate.mockRejectedValueOnce(new Error('validate failed'));
    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => expect(screen.getByTestId('validate-error')).toBeInTheDocument());

    expect(screen.queryByTestId('validate-scope')).toBeNull();
    expect(screen.queryByTestId('validate-rules')).toBeNull();
  });

  test('profile labels say structural subset and keep their values', () => {
    render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    const select = screen.getByTestId('validate-profile') as HTMLSelectElement;
    const options = Array.from(select.options).map((o) => ({ value: o.value, label: o.textContent }));
    expect(options).toEqual([
      { value: 'pdfa-1b', label: 'PDF/A-1b (structural subset)' },
      { value: 'pdfua-1-structural', label: 'PDF/UA-1 (structural subset)' },
    ]);
  });

  test('pre-run disclaimer text is unchanged', () => {
    render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);

    expect(screen.getByTestId('validate-disclaimer').textContent).toBe(DISCLAIMER_TEXT);
  });
});
