/**
 * @file React error boundary. Catches render errors in child components
 * and shows a fallback UI instead of crashing the entire app.
 */
import { Component, type ErrorInfo, type ReactNode } from 'react';

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
  /** A change of this value clears a caught error and renders the children again. */
  resetKey?: string | null;
}

interface State {
  hasError: boolean;
}

/**
 * Class-based error boundary (hooks cannot catch render errors).
 * Wraps panels so a crash in one does not tear down siblings. A panel that
 * stays mounted across documents passes the active tab id as resetKey, so an
 * error caused by one document does not stick to the next.
 */
export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(): State {
    return { hasError: true };
  }

  componentDidUpdate(prevProps: Props, prevState: State): void {
    // An error caught in the same update that changed resetKey belongs to the new key.
    if (prevState.hasError && this.state.hasError && prevProps.resetKey !== this.props.resetKey) {
      this.setState({ hasError: false });
    }
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // eslint-disable-next-line no-console -- legitimate diagnostics in an error boundary
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
