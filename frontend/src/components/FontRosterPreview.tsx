/**
 * @file FontRosterPreview -- presentational component for the
 * /Resources /Font resource map. Renders the FontResourceMap payload
 * (one row per font referenced from the page's resource dictionary).
 *
 * Pure presentational: receives all data plus an onReferenceClick handler.
 * Mirrors FontPreview's render shape (sections + Tailwind tokens).
 */

/** One row in a font-resource roster table. */
export interface FontRosterEntryData {
  name: string;
  nodeId: string;
  objectRef: string;
  baseFont: string;
  subtype: string;
  encodingSummary: string;
  embedded: boolean;
  unresolved: boolean;
}

/** Full payload returned by GetFontResourceMap. */
export interface FontResourceMapData {
  nodeId: string;
  entries: FontRosterEntryData[];
}

/** Props for the FontRosterPreview component. */
export interface FontRosterPreviewProps {
  roster: FontResourceMapData;
  onReferenceClick: (refTarget: string) => void;
}

/** Embedded chip used in the per-row Embedded column. Compact (single-row)
 *  variant of FontPreview's EmbeddedBadge -- check for embedded, dash otherwise. */
function EmbeddedCell({ embedded }: { embedded: boolean }) {
  if (embedded) {
    return (
      <span
        className="inline-flex items-center px-1.5 py-0.5 text-[10px] rounded bg-success/10 text-success font-medium"
        data-testid="font-roster-embedded-yes"
      >
        Embedded
      </span>
    );
  }
  return (
    <span
      className="inline-flex items-center px-1.5 py-0.5 text-[10px] rounded bg-error/10 text-error font-medium"
      data-testid="font-roster-embedded-no"
    >
      No
    </span>
  );
}

/** Renders the page's font roster (Fonts used on this page table). */
export function FontRosterPreview({ roster, onReferenceClick }: FontRosterPreviewProps) {
  const count = roster.entries.length;
  return (
    <div
      className="flex-1 min-h-0 flex flex-col overflow-hidden"
      data-testid="font-roster-preview"
    >
      <section className="p-3 text-xs shrink-0">
        <div className="flex items-center gap-2 mb-1">
          <span className="text-text-secondary font-medium">Fonts used on this page</span>
          <span className="text-text-muted">({count})</span>
        </div>
      </section>
      <div className="flex-1 min-h-0 overflow-auto border-t border-border">
        {count === 0 ? (
          <div className="p-3 text-text-muted text-xs" data-testid="font-roster-empty">
            No fonts referenced.
          </div>
        ) : (
          <table className="w-full text-xs">
            <thead className="sticky top-0 z-10">
              <tr>
                <th className="text-left px-2 py-1 text-text-secondary font-medium bg-hover">Name</th>
                <th className="text-left px-2 py-1 text-text-secondary font-medium bg-hover">BaseFont</th>
                <th className="text-left px-2 py-1 text-text-secondary font-medium bg-hover">Subtype</th>
                <th className="text-left px-2 py-1 text-text-secondary font-medium bg-hover">Embedded</th>
                <th className="text-left px-2 py-1 text-text-secondary font-medium bg-hover">Encoding</th>
              </tr>
            </thead>
            <tbody>
              {roster.entries.map((entry) => {
                if (entry.unresolved) {
                  return (
                    <tr
                      key={entry.name}
                      className="border-t border-border"
                      data-testid="font-roster-row"
                      data-unresolved="true"
                    >
                      <td className="px-2 py-1 font-mono">/{entry.name}</td>
                      <td className="px-2 py-1">
                        <span
                          className="inline-flex items-center px-1.5 py-0.5 text-[10px] rounded bg-error/10 text-error font-medium"
                          data-testid="font-roster-unresolved-pill"
                        >
                          unresolved
                        </span>
                      </td>
                      <td className="px-2 py-1" />
                      <td className="px-2 py-1" />
                      <td className="px-2 py-1" />
                    </tr>
                  );
                }
                return (
                  <tr
                    key={entry.name}
                    role="button"
                    tabIndex={0}
                    className="border-t border-border cursor-pointer hover:bg-hover"
                    data-testid="font-roster-row"
                    data-ref-target={entry.nodeId}
                    onClick={() => entry.nodeId && onReferenceClick(entry.nodeId)}
                    onKeyDown={(e) => {
                      if ((e.key === 'Enter' || e.key === ' ') && entry.nodeId) {
                        e.preventDefault();
                        onReferenceClick(entry.nodeId);
                      }
                    }}
                  >
                    <td className="px-2 py-1 font-mono">/{entry.name}</td>
                    <td className="px-2 py-1 font-mono">{entry.baseFont}</td>
                    <td className="px-2 py-1 font-mono">{entry.subtype}</td>
                    <td className="px-2 py-1">
                      <EmbeddedCell embedded={entry.embedded} />
                    </td>
                    <td className="px-2 py-1 font-mono text-text-muted">
                      {entry.encodingSummary}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
