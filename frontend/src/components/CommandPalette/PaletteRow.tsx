/**
 * @file Single result row in the Cmd+K palette. Renders the object ref,
 * /Type, and (when this row is highlighted) the breadcrumb path.
 * Story 9-8 Task 4.
 */
import type { ObjectIndexEntry } from '../../types/palette';

interface PaletteRowProps {
  entry: ObjectIndexEntry;
  highlighted: boolean;
  breadcrumb: string | null;
  onClick: () => void;
  /** Test marker: 'command-palette-row' for fresh results, 'command-palette-recent-row' for recents. */
  testId: string;
}

/** Result row. Free/orphan rows render grey-italic with the (free)/(orphan) suffix. */
export function PaletteRow({ entry, highlighted, breadcrumb, onClick, testId }: PaletteRowProps) {
  const ref = `${entry.objNum} ${entry.gen} R`;
  const suffix = entry.free ? '(free)' : !entry.reachable ? '(orphan)' : '';
  const baseClass = 'px-3 py-1.5 cursor-pointer text-sm font-ui flex flex-col';
  const stateClass = highlighted ? 'bg-surface-selected' : 'hover:bg-surface-hover';
  const dimmedClass = entry.free || !entry.reachable ? 'text-text-muted italic' : 'text-text';
  return (
    <div
      role="option"
      aria-selected={highlighted}
      data-testid={testId}
      className={`${baseClass} ${stateClass} ${dimmedClass}`}
      onClick={onClick}
    >
      <div className="flex items-center gap-2">
        <span className="font-mono">[{ref}]</span>
        {entry.typeName !== '' && (
          <span className="text-text-muted">/T:{entry.typeName}</span>
        )}
        {suffix !== '' && (
          <span className="text-text-muted text-xs">{suffix}</span>
        )}
      </div>
      {highlighted && breadcrumb !== null && breadcrumb !== '' && (
        <div className="text-xs text-text-muted mt-0.5">in {breadcrumb}</div>
      )}
    </div>
  );
}
