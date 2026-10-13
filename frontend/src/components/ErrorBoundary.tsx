/**
 * @file React error boundary. Catches render errors in child components
 * and shows a fallback UI instead of crashing the entire app.
 */
import { Component, type ErrorInfo, type ReactNode } from 'react';
import { SMALL_BUTTON } from './buttonStyles';

interface Props {
  children: ReactNode;
  /** Replaces the default fallback, including its Try again button. */
  fallback?: ReactNode;
  /**
   * A change in any of these values, compared element by element, clears a
   * caught error and renders the children again.
   */
  resetKeys?: readonly unknown[];
}

interface State {
  hasError: boolean;
}

function keysChanged(a: readonly unknown[] = [], b: readonly unknown[] = []): boolean {
  return a.length !== b.length || a.some((v, i) => !Object.is(v, b[i]));
}

/**
 * Class-based error boundary (hooks cannot catch render errors).
 * Wraps panels so a crash in one does not tear down siblings. A panel that
 * stays mounted while its input changes passes that input as resetKeys (the
 * active tab, and for a panel that renders the selection, the selected node),
 * so an error caused by one input does not stick to the next. The default
 * fallback has a Try again button that renders the children again in place.
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
    // An error caught in the same update that changed resetKeys belongs to the new keys.
    if (prevState.hasError && this.state.hasError && keysChanged(prevProps.resetKeys, this.props.resetKeys)) {
      this.setState({ hasError: false });
    }
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // eslint-disable-next-line no-console -- legitimate diagnostics in an error boundary
    console.error('ErrorBoundary caught:', error, info);
  }

  private retry = (): void => {
    this.setState({ hasError: false });
  };

  render(): ReactNode {
    if (this.state.hasError) {
      return this.props.fallback ?? (
        <div className="h-full flex flex-col items-center justify-center gap-2 text-text-muted text-sm">
          <p>Something went wrong rendering this panel.</p>
          <button type="button" className={`px-2 py-1 ${SMALL_BUTTON}`} onClick={this.retry}>
            Try again
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
