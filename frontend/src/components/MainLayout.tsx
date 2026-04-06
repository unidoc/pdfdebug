/**
 * @file Three-panel resizable layout for document inspection.
 * Left: tree + object-info (vertical split). Right: detail panel.
 */
import { Allotment } from 'allotment';
import 'allotment/dist/style.css';
import { ErrorBoundary } from './ErrorBoundary';
import { TreePanel } from './TreePanel';
import { ObjectInfoPanel } from './ObjectInfoPanel';
import { DetailPanel } from './DetailPanel';

/**
 * Resizable three-panel layout using Allotment.
 * Each panel is wrapped in an ErrorBoundary so a crash in one panel
 * does not tear down the others.
 */
export function MainLayout() {
  return (
    <div className="h-full" data-testid="main-layout">
      <ErrorBoundary>
        <Allotment>
          <Allotment.Pane preferredSize={300} minSize={200}>
            <aside className="h-full" data-testid="left-panel">
              <Allotment vertical>
                <Allotment.Pane>
                  <TreePanel />
                </Allotment.Pane>
                <Allotment.Pane preferredSize="30%" minSize={100}>
                  <ErrorBoundary>
                    <ObjectInfoPanel />
                  </ErrorBoundary>
                </Allotment.Pane>
              </Allotment>
            </aside>
          </Allotment.Pane>
          <Allotment.Pane>
            <main className="h-full" data-testid="right-panel">
              <ErrorBoundary>
                <DetailPanel />
              </ErrorBoundary>
            </main>
          </Allotment.Pane>
        </Allotment>
      </ErrorBoundary>
    </div>
  );
}
