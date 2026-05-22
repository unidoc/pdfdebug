/** Props for {@link ErrorBanner}. */
export interface ErrorBannerProps {
  message: string;
  severity: 'error' | 'warning';
  onDismiss: () => void;
}

/**
 * Dismissible banner for document-level errors and warnings.
 * Renders at the top of the app shell with severity-appropriate styling.
 */
export function ErrorBanner({ message, severity, onDismiss }: ErrorBannerProps) {
  const isError = severity === 'error';
  // Use dark tinted text on the pale-tinted background. The brand --color-error
  // / --color-warning tokens are vivid (red-500 / amber-500); on a 50-tinted
  // background the contrast is too weak to read. Standard tinted-banner
  // pattern: 50 background with 900 text in light, 900/20 background with
  // 200 text in dark.
  // No `dark:` variants here on purpose: the rest of the app shell uses
  // design-token CSS that does not flip on prefers-color-scheme, so a
  // dark-system-theme user otherwise gets a "dark banner on a light shell"
  // mismatch. Keeping the banner explicitly light-tinted matches the
  // surrounding chrome and gives high text-on-bg contrast.
  const bgColor = isError ? 'bg-red-100' : 'bg-amber-100';
  const textColor = isError ? 'text-red-900' : 'text-amber-900';
  const icon = isError ? '(x)' : '(!)';
  const testId = isError ? 'error-banner' : 'warning-banner';
  const dismissLabel = isError ? 'Dismiss error' : 'Dismiss warning';

  return (
    <div
      data-testid={testId}
      role="alert"
      className={`flex items-center gap-2 px-4 py-2 ${bgColor} ${textColor} transition-all duration-150`}
    >
      <span className="text-sm font-medium" aria-hidden="true">{icon}</span>
      <span className="text-sm flex-1" data-testid="error-banner-message">
        {message}
      </span>
      <button
        data-testid="error-banner-dismiss"
        className="p-1 hover:opacity-70 focus-visible:ring-2 focus-visible:ring-border-focus focus-visible:outline-none rounded"
        onClick={onDismiss}
        aria-label={dismissLabel}
      >
        X
      </button>
    </div>
  );
}
