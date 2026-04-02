export interface ErrorBannerProps {
  message: string;
  severity: 'error' | 'warning';
  onDismiss: () => void;
}

export function ErrorBanner({ message, severity, onDismiss }: ErrorBannerProps) {
  const bgColor = severity === 'error' ? 'bg-red-50' : 'bg-amber-50';
  const textColor = severity === 'error' ? 'text-error' : 'text-warning';

  return (
    <div
      data-testid="error-banner"
      role="alert"
      className={`flex items-center gap-2 px-4 py-2 ${bgColor} ${textColor} transition-all duration-150`}
    >
      <span className="text-sm flex-1" data-testid="error-banner-message">
        {message}
      </span>
      <button
        data-testid="error-banner-dismiss"
        className="p-1 hover:opacity-70 focus-visible:ring-2 focus-visible:ring-border-focus focus-visible:outline-none rounded"
        onClick={onDismiss}
        aria-label="Dismiss error"
      >
        X
      </button>
    </div>
  );
}
