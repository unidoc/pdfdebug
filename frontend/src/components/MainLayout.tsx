import { Allotment } from 'allotment';
import 'allotment/dist/style.css';
import { ErrorBoundary } from './ErrorBoundary';
import { TreePanel } from './TreePanel';

export function MainLayout() {
  return (
    <div className="h-full" data-testid="main-layout">
      <ErrorBoundary>
        <Allotment>
          <Allotment.Pane preferredSize={300} minSize={200}>
            <aside className="h-full" data-testid="left-panel">
              <TreePanel />
            </aside>
          </Allotment.Pane>
          <Allotment.Pane>
            <main className="h-full p-3 overflow-auto" data-testid="right-panel">
              <span className="text-text-muted text-sm">Detail Panel</span>
            </main>
          </Allotment.Pane>
        </Allotment>
      </ErrorBoundary>
    </div>
  );
}
