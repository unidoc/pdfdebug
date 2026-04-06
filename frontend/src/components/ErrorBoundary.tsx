/**
 * @file React error boundary. Catches render errors in child components
 * and shows a fallback UI instead of crashing the entire app.
 */
import { Component, type ErrorInfo, type ReactNode } from 'react';

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
}

/**
 * Class-based error boundary (hooks cannot catch render errors).
 * Wraps panels so a crash in one does not tear down siblings.
 */
export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(): State {
    return { hasError: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error('ErrorBoundary caught:', error, info);
  }

  render(): ReactNode {
    if (this.state.hasError) {
      return this.props.fallback ?? (
        <div className="h-full flex items-center justify-center text-text-muted text-sm">
          Something went wrong rendering this panel.
        </div>
      );
    }
    return this.props.children;
  }
}
