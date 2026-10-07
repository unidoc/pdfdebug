/**
 * @file Validate view -- document-level panel for a named subset of structural
 * checks. A "Run checks" action with a profile selector runs the bounded
 * PDF/A-1b or PDF/UA-1-structural rule set. Each result shows the profile's
 * scope sentence and the rules checked (id, spec ref, outcome, what each rule
 * checks) above either a clean sentence naming the rule count or the problems
 * grouped by severity. Clicking an object-scoped problem jumps TreePanel to the offending
 * object via the existing NAVIGATE_TO_REF wiring; a problem with no object node
 * id is shown but not clickable, while a non-empty-but-unresolvable id degrades
 * gracefully through the reducer's navigation-error path (never a broken jump).
 * The not-authoritative disclaimer is ALWAYS visible and the panel NEVER states
 * an authoritative conformance verdict.
 */
import { useEffect, useRef, useState } from 'react';
import { Validate } from '../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { extractErrorMessage } from '../lib/extractErrorMessage';
import { SMALL_BUTTON } from './buttonStyles';

/** One structural problem, mirroring `pdfcore.Problem`. */
export interface ValidateProblem {
  ruleId: string;
  profile: string;
  severity: string;
  message: string;
  objRef: string;
  objNodeId: string;
  specRef: string;
}

/** One rule that ran and its outcome, mirroring `pdfcore.RuleInfo`. */
export interface ValidateRuleInfo {
  ruleId: string;
  specRef: string;
  severity: string;
  checks: string;
  /** False when the rule degraded to an info problem. */
  evaluated: boolean;
  /** Problems the rule emitted in this run (0 when not evaluated). */
  findings: number;
}

/** The validation outcome, mirroring `pdfcore.ValidationResult`. */
export interface ValidateResult {
  profile: string;
  summary: { errors: number; warnings: number; info?: number };
  problems: ValidateProblem[];
  disclaimer?: string;
  scope: string;
  rules: ValidateRuleInfo[];
}

/** Props for {@link ValidateView}. */
export interface ValidateViewProps {
  /** Active document tab ID. */
  tabId: string;
  /** True when the Validate tab is active (unused: results persist per mount). */
  active: boolean;
  /** Dispatches NAVIGATE_TO_REF to jump TreePanel to an object-scoped problem. */
  onNavigate: (nodeId: string) => void;
}

/** The two bounded, explicit profiles. */
const PROFILES: { value: string; label: string }[] = [
  { value: 'pdfa-1b', label: 'PDF/A-1b (structural subset)' },
  { value: 'pdfua-1-structural', label: 'PDF/UA-1 (structural subset)' },
];

/** Severity groups in display order, each with its stable testid. */
const SEVERITY_GROUPS: { severity: string; testid: string; label: string }[] = [
  { severity: 'error', testid: 'validate-group-error', label: 'Errors' },
  { severity: 'warning', testid: 'validate-group-warning', label: 'Warnings' },
  { severity: 'info', testid: 'validate-group-info', label: 'Info' },
];

/**
 * The always-visible honesty disclaimer. Deliberately free of any
 * authoritative verdict language ("compliant" / "valid" / "passed").
 */
// Kept byte-identical to pdfcore.DisclaimerText so the CLI and GUI never ship
// two different honesty-guardrail wordings. (The banner is shown before a run
// too, so a client-side constant is required rather than result.disclaimer.)
const DISCLAIMER =
  'structural checks only - not full conformance; use veraPDF for authoritative PDF/A / PDF/UA validation';

/** One problem row. Object-scoped problems are clickable (jump to node);
 *  object-ref-less (document-level) problems are shown but not clickable. */
function ProblemRow({
  problem,
  onNavigate,
}: {
  problem: ValidateProblem;
  onNavigate: (nodeId: string) => void;
}) {
  const clickable = Boolean(problem.objNodeId);
  return (
    <div
      data-testid="validate-problem"
      role={clickable ? 'button' : undefined}
      tabIndex={clickable ? 0 : undefined}
      onClick={clickable ? () => onNavigate(problem.objNodeId) : undefined}
      onKeyDown={
        clickable
          ? (e) => {
              // Keyboard activation parity with mouse click (Enter/Space),
              // matching the role="button" idiom used elsewhere in the panel.
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onNavigate(problem.objNodeId);
              }
            }
          : undefined
      }
      className={
        'border border-border rounded px-2 py-1.5 mb-1.5 text-xs font-mono ' +
        (clickable
          ? 'cursor-pointer hover:bg-surface-hover'
          : 'cursor-default text-text-secondary')
      }
    >
      <div className="text-text break-all">{problem.message}</div>
      <div className="text-text-muted mt-0.5 flex gap-3 flex-wrap">
        {problem.objRef ? (
          <span>{problem.objRef}</span>
        ) : (
          // Document-level problem (no object ref): flagged as the "Document"
          // group so it is visually distinct from object-scoped rows.
          <span data-testid="validate-doc-label" className="uppercase tracking-wide">
            Document
          </span>
        )}
        <span>{problem.ruleId}</span>
        <span>{problem.specRef}</span>
      </div>
    </div>
  );
}

/** Outcome column text for one rule. */
function ruleOutcome(rule: ValidateRuleInfo): string {
  return rule.evaluated ? `${rule.findings} found` : 'not evaluated';
}

/** The scope sentence and the rules that ran, shown above every result. */
function RulesChecked({ result }: { result: ValidateResult }) {
  return (
    <div className="mb-3">
      <div className="text-xs text-text-muted mb-2" data-testid="validate-scope">
        {result.scope}
      </div>
      <div className="border-b border-border pb-2" data-testid="validate-rules">
        <h3 className="text-xs font-medium text-text-secondary mb-1">
          Rules checked ({result.rules.length})
        </h3>
        {/* One grid for all rows so the columns line up; a long description
            wraps inside its own column, next to the rule it belongs to. */}
        <div className="grid grid-cols-[max-content_max-content_max-content_minmax(0,1fr)] gap-x-3 gap-y-1 text-xs font-mono">
          {result.rules.map((rule) => (
            <div key={rule.ruleId} className="contents" data-testid="validate-rule">
              <span className="text-text">{rule.ruleId}</span>
              <span className="text-text-muted">{rule.specRef}</span>
              <span className="text-text">{ruleOutcome(rule)}</span>
              <span className="text-text-muted break-words">{rule.checks}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

/**
 * Document-level structural-validation panel. Self-contained: it runs the
 * chosen profile against `tabId` on demand and renders the grouped result.
 */
export function ValidateView({ tabId, active: _active, onNavigate }: ValidateViewProps) {
  const [profile, setProfile] = useState<string>('pdfa-1b');
  const [result, setResult] = useState<ValidateResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [running, setRunning] = useState(false);

  // The panel is a single instance reused across document tabs (not remounted
  // per tab), so switching documents must clear the previous document's result
  // and error - otherwise stale findings from tab A are shown under tab B.
  useEffect(() => {
    setResult(null);
    setError(null);
    setRunning(false);
  }, [tabId]);

  // Tracks the currently-active tab and selected profile so an in-flight run
  // is discarded if either changes before it resolves. Profile is guarded in
  // addition to tab because a tab switch resets `running`, which re-enables the
  // profile selector and would otherwise let a stale run overwrite the result
  // under a different profile.
  const tabIdRef = useRef(tabId);
  tabIdRef.current = tabId;
  const profileRef = useRef(profile);
  profileRef.current = profile;

  const runChecks = () => {
    if (!tabId) return;
    const runTabId = tabId;
    const runProfile = profile;
    const stale = () => tabIdRef.current !== runTabId || profileRef.current !== runProfile;
    setRunning(true);
    setError(null);
    Validate(tabId, profile)
      .then((res: unknown) => {
        if (stale()) return; // tab or profile changed mid-run
        setResult(res as ValidateResult);
      })
      .catch((err: unknown) => {
        if (stale()) return;
        setError(extractErrorMessage(err));
      })
      .finally(() => {
        if (stale()) return;
        setRunning(false);
      });
  };

  return (
    <div className="h-full flex flex-col" data-testid="validate-view">
      <div className="flex items-center gap-2 px-3 py-2 border-b border-border bg-surface flex-shrink-0">
        <label className="text-xs text-text-muted" htmlFor="validate-profile-select">
          Profile
        </label>
        <select
          id="validate-profile-select"
          data-testid="validate-profile"
          className="text-xs bg-surface border border-border rounded px-1.5 py-0.5 text-text"
          value={profile}
          // Locked while a run is in flight so the profile cannot change out
          // from under an unresolved Validate call (which would otherwise render
          // the old profile's findings under the newly-selected profile).
          disabled={running}
          onChange={(e) => {
            // Changing the profile invalidates the displayed result (it was run
            // for the previous profile); clear it so the panel never shows
            // findings that do not match the selected profile.
            setProfile(e.target.value);
            setResult(null);
            setError(null);
          }}
        >
          {PROFILES.map((p) => (
            <option key={p.value} value={p.value}>
              {p.label}
            </option>
          ))}
        </select>
        <button
          type="button"
          data-testid="validate-run"
          className={`px-2 py-1 ${SMALL_BUTTON} disabled:opacity-50 disabled:cursor-not-allowed`}
          onClick={runChecks}
          disabled={running || !tabId}
        >
          {running ? 'Running...' : 'Run checks'}
        </button>
      </div>

      <div
        className="px-3 py-1.5 text-xs text-text-muted italic border-b border-border flex-shrink-0"
        data-testid="validate-disclaimer"
      >
        {DISCLAIMER}
      </div>

      <div className="flex-1 min-h-0 overflow-auto p-3">
        {error && (
          <div className="text-error text-sm" data-testid="validate-error">
            {error}
          </div>
        )}

        {!error && result && <RulesChecked result={result} />}

        {!error && result && result.problems.length === 0 && (
          <div className="text-text-muted text-sm" data-testid="validate-empty">
            None of the {result.rules.length} rule{result.rules.length === 1 ? '' : 's'} checked
            found a problem (structural subset only)
          </div>
        )}

        {!error && result && result.problems.length > 0 && (
          <>
            <div className="text-xs text-text-muted mb-2" data-testid="validate-summary">
              {result.summary.errors} error{result.summary.errors === 1 ? '' : 's'},{' '}
              {result.summary.warnings} warning{result.summary.warnings === 1 ? '' : 's'}
              {result.summary.info ? (
                <>
                  , {result.summary.info} info problem{result.summary.info === 1 ? '' : 's'}
                </>
              ) : null}{' '}
              (structural checks only)
            </div>
            {SEVERITY_GROUPS.map((g) => {
              const group = result.problems.filter((p) => p.severity === g.severity);
              if (group.length === 0) return null;
              return (
                <div key={g.severity} className="mb-3" data-testid={g.testid}>
                  <h3 className="text-xs font-medium text-text-secondary mb-1">
                    {g.label} ({group.length})
                  </h3>
                  {group.map((p, i) => (
                    <ProblemRow key={`${p.ruleId}-${i}`} problem={p} onNavigate={onNavigate} />
                  ))}
                </div>
              );
            })}
          </>
        )}
      </div>
    </div>
  );
}

export default ValidateView;
