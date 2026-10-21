/**
 * @file Bordered notice box for a problem found while decomposing an object.
 * Detail views place it at the top of the affected section.
 */
import type { ReactNode } from 'react';

/** Severity of a {@link ProblemCallout}: 'error' is red, 'warning' is amber. */
export type ProblemTone = 'error' | 'warning';

/** Props for {@link ProblemCallout}. */
export interface ProblemCalloutProps {
  tone: ProblemTone;
  children: ReactNode;
  testid?: string;
  className?: string;
}

const TONE_CLASS: Record<ProblemTone, string> = {
  error: 'border-error/60 text-error',
  warning: 'border-warning/60 text-warning',
};

/** Renders a small bordered notice in the error or warning color. */
export function ProblemCallout({ tone, children, testid, className = '' }: ProblemCalloutProps) {
  return (
    <div
      className={`px-2 py-1 text-xs rounded border break-all ${TONE_CLASS[tone]} ${className}`}
      data-testid={testid}
    >
      {children}
    </div>
  );
}

export default ProblemCallout;
