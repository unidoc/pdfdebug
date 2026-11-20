/**
 * ValidateView with the pdfcpu profile: selecting it runs
 * Validate(tabId, 'pdfcpu'), and every problem that carries an objNodeId jumps
 * to its object through onNavigate (the NAVIGATE_TO_REF wiring).
 *
 * Run: cd frontend && npx vitest run src/components/ValidateView.pdfcpu.test.tsx
 */
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { ValidateView } from './ValidateView';
import { requestValidateRun } from '../lib/pdfcpuCheck';

const mockValidate = vi.fn();
vi.mock('../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js', () => ({
  Validate: (...a: unknown[]) => mockValidate(...a),
}));

const pdfcpuResult = {
  profile: 'pdfcpu',
  summary: { errors: 1, warnings: 1, info: 0 },
  problems: [
    {
      ruleId: 'pdfcpu-validator',
      profile: 'pdfcpu',
      severity: 'error',
      message: 'catalog: StructTreeRoot: obj#12: circular structure tree',
      objRef: '12 0 R',
      objNodeId: 'obj:0:12',
      specRef: 'pdfcpu',
    },
    {
      ruleId: 'pdfcpu-notices',
      profile: 'pdfcpu',
      severity: 'warning',
      message: 'digested during validate: Type1 font CustomFont: missing required entries Widths',
      objRef: '5 0 R',
      objNodeId: 'obj:0:5',
      specRef: 'pdfcpu',
    },
  ],
  disclaimer: 'structural checks only - not full conformance; use veraPDF for authoritative PDF/A / PDF/UA validation',
  scope: 'pdfcpu validator on a separate parse of the file; the list can be incomplete.',
  rules: [
    { ruleId: 'pdfcpu-validator', specRef: 'pdfcpu', severity: 'error', checks: 'relaxed validator accepts the document', evaluated: true, findings: 1 },
    { ruleId: 'pdfcpu-notices', specRef: 'pdfcpu', severity: 'warning', checks: 'repairs pdfcpu records', evaluated: true, findings: 1 },
  ],
};

beforeEach(() => {
  vi.clearAllMocks();
  mockValidate.mockResolvedValue(pdfcpuResult);
});

describe('ValidateView pdfcpu profile', () => {
  test('selecting pdfcpu and running calls Validate with the pdfcpu profile', async () => {
    render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);
    fireEvent.change(screen.getByTestId('validate-profile'), { target: { value: 'pdfcpu' } });
    expect((screen.getByTestId('validate-profile') as HTMLSelectElement).value).toBe('pdfcpu');
    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => expect(mockValidate).toHaveBeenCalledWith('tab-1', 'pdfcpu'));
  });

  test('every pdfcpu problem with an objNodeId navigates to its object', async () => {
    const onNavigate = vi.fn();
    render(<ValidateView tabId="tab-1" active onNavigate={onNavigate} />);
    fireEvent.change(screen.getByTestId('validate-profile'), { target: { value: 'pdfcpu' } });
    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => expect(screen.getAllByTestId('validate-problem')).toHaveLength(2));
    expect(mockValidate).toHaveBeenCalledWith('tab-1', 'pdfcpu');

    for (const row of screen.getAllByTestId('validate-problem')) fireEvent.click(row);
    expect(onNavigate.mock.calls.map((c) => c[0])).toEqual(['obj:0:12', 'obj:0:5']);
  });

  test('a clean pdfcpu run names the rule count without the structural-subset note', async () => {
    mockValidate.mockResolvedValue({ ...pdfcpuResult, summary: { errors: 0, warnings: 0, info: 0 }, problems: [] });
    render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);
    fireEvent.change(screen.getByTestId('validate-profile'), { target: { value: 'pdfcpu' } });
    fireEvent.click(screen.getByTestId('validate-run'));
    const empty = await screen.findByTestId('validate-empty');
    expect(empty.textContent).toBe('None of the 2 rules checked found a problem');
  });

  test('a pdfa-1b run in flight when a pdfcpu run request arrives never renders', async () => {
    let resolvePdfa: (v: unknown) => void = () => {};
    let resolvePdfcpu: (v: unknown) => void = () => {};
    mockValidate.mockImplementation((_tab: string, profile: string) =>
      new Promise((resolve) => {
        if (profile === 'pdfcpu') resolvePdfcpu = resolve;
        else resolvePdfa = resolve;
      }),
    );
    const pdfaResult = {
      ...pdfcpuResult,
      profile: 'pdfa-1b',
      summary: { errors: 1, warnings: 0, info: 0 },
      problems: [{ ruleId: 'no-encryption', profile: 'pdfa-1b', severity: 'error', message: 'stale pdfa-1b finding', specRef: '6.1.3' }],
    };
    render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);
    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => expect(mockValidate).toHaveBeenCalledWith('tab-1', 'pdfa-1b'));

    act(() => requestValidateRun('tab-1', 'pdfcpu'));
    await waitFor(() => expect(mockValidate).toHaveBeenCalledWith('tab-1', 'pdfcpu'));
    expect((screen.getByTestId('validate-profile') as HTMLSelectElement).value).toBe('pdfcpu');

    await act(async () => resolvePdfa(pdfaResult));
    expect(screen.queryByText('stale pdfa-1b finding')).toBeNull();
    expect(screen.getByTestId('validate-run').textContent).toBe('Running...');

    await act(async () => resolvePdfcpu(pdfcpuResult));
    await waitFor(() => expect(screen.getAllByTestId('validate-problem')).toHaveLength(2));
    expect(screen.queryByText('stale pdfa-1b finding')).toBeNull();
    expect(screen.getByTestId('validate-run').textContent).toBe('Run checks');
  });

  test('a superseded pdfa-1b run that resolves during a newer pdfa-1b run neither renders nor unlocks the run', async () => {
    const pending: Array<{ profile: string; resolve: (v: unknown) => void }> = [];
    mockValidate.mockImplementation((_tab: string, profile: string) =>
      new Promise((resolve) => {
        pending.push({ profile, resolve });
      }),
    );
    const pdfaResult = (message: string) => ({
      ...pdfcpuResult,
      profile: 'pdfa-1b',
      summary: { errors: 1, warnings: 0, info: 0 },
      problems: [{ ruleId: 'no-encryption', profile: 'pdfa-1b', severity: 'error', message, specRef: '6.1.3' }],
    });
    render(<ValidateView tabId="tab-1" active onNavigate={vi.fn()} />);
    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => expect(pending).toHaveLength(1));

    act(() => requestValidateRun('tab-1', 'pdfcpu'));
    await waitFor(() => expect(pending).toHaveLength(2));
    await act(async () => pending[1].resolve(pdfcpuResult));
    await waitFor(() => expect(screen.getByTestId('validate-run').textContent).toBe('Run checks'));

    fireEvent.change(screen.getByTestId('validate-profile'), { target: { value: 'pdfa-1b' } });
    fireEvent.click(screen.getByTestId('validate-run'));
    await waitFor(() => expect(pending).toHaveLength(3));

    await act(async () => pending[0].resolve(pdfaResult('superseded pdfa-1b finding')));
    expect(screen.queryByText('superseded pdfa-1b finding')).toBeNull();
    expect(screen.getByTestId('validate-run').textContent).toBe('Running...');
    expect((screen.getByTestId('validate-profile') as HTMLSelectElement).disabled).toBe(true);

    await act(async () => pending[2].resolve(pdfaResult('latest pdfa-1b finding')));
    await waitFor(() => expect(screen.getByText('latest pdfa-1b finding')).toBeTruthy());
    expect(screen.queryByText('superseded pdfa-1b finding')).toBeNull();
    expect(screen.getByTestId('validate-run').textContent).toBe('Run checks');
  });
});
