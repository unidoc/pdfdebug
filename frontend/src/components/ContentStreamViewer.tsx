/**
 * @file Presentational component that renders decoded PDF content stream
 * text with line numbers in a scrollable, monospace view.
 */

interface ContentStreamViewerProps {
  raw: string;
  error?: string;
}

/** Renders decoded content stream as plain text with a line-number gutter. */
export function ContentStreamViewer({ raw, error }: ContentStreamViewerProps) {
  if (error) {
    return (
      <div
        className="text-error text-sm p-3"
        data-testid="content-stream-error"
      >
        {error}
      </div>
    );
  }

  const lines = raw ? raw.split('\n') : [];

  return (
    <div
      className="flex-1 min-h-0 overflow-auto"
      data-testid="content-stream-viewer"
    >
      <div className="flex">
        <div
          className="flex-shrink-0 text-right pr-3 select-none text-text-muted text-xs font-mono border-r border-border"
          style={{ minWidth: `${Math.max(String(lines.length).length, 2)}ch` }}
          data-testid="content-stream-gutter"
        >
          {lines.map((_, i) => (
            <div key={i} className="px-1">{i + 1}</div>
          ))}
        </div>
        <div
          className="pl-3 font-mono text-xs text-text whitespace-pre"
          data-testid="content-stream-content"
        >
          {lines.map((line, i) => (
            <div key={i}>{line}</div>
          ))}
        </div>
      </div>
    </div>
  );
}
