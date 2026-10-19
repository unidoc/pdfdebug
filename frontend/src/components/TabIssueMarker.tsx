/**
 * @file Small dot shown next to a tab label when the tab's content has a
 * problem. Same size and ring as the document-tab disk-change dot.
 */
import type { ProblemTone } from './ProblemCallout';

/** Props for {@link TabIssueMarker}. */
export interface TabIssueMarkerProps {
  tone: ProblemTone;
  /** Hover text and accessible name, e.g. "1 signature could not be decomposed". */
  title: string;
  testid?: string;
}

/** Renders a red (error) or amber (warning) dot with an accessible label. */
export function TabIssueMarker({ tone, title, testid }: TabIssueMarkerProps) {
  return (
    <span
      className={`disk-dot-in ml-1.5 inline-block h-1.5 w-1.5 rounded-full align-middle ring-1 ring-text-secondary ${
        tone === 'error' ? 'bg-error' : 'bg-warning'
      }`}
      role="img"
      aria-label={title}
      title={title}
      data-tone={tone}
      data-testid={testid}
    />
  );
}

export default TabIssueMarker;
