export interface ErrorBannerProps {
  message: string;
  severity: 'error' | 'warning';
  onDismiss: () => void;
}

export function ErrorBanner({ message, severity, onDismiss }: ErrorBannerProps) {
  const isError = severity === 'error';
  const bgColor = isError ? 'bg-red-50 dark:bg-red-900/20' : 'bg-amber-50 dark:bg-amber-900/20';
  const textColor = isError ? 'text-error' : 'text-warning';
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
