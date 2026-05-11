/**
 * @file Three-panel resizable layout for document inspection.
 * Left: tree + object-info (vertical split). Right: detail panel.
 * Panel sizes are persisted to localStorage via useWindowPersistence.
 */
import { useCallback, useRef } from 'react';
import { Allotment } from 'allotment';
import 'allotment/dist/style.css';
import { ErrorBoundary } from './ErrorBoundary';
import { TreePanel } from './TreePanel';
import { ObjectSourcePanel } from './ObjectInfoPanel';
import { DetailPanel } from './DetailPanel';
import { useWindowPersistence, type PanelSizes } from '../hooks/useWindowPersistence';

/**
 * Resizable three-panel layout using Allotment.
 * Each panel is wrapped in an ErrorBoundary so a crash in one panel
 * does not tear down the others. Panel sizes are persisted via localStorage.
 */
export function MainLayout() {
  const { panelSizes, savePanelSizes } = useWindowPersistence();

  // Track the latest sizes from each split so we can save both dimensions together.
  const latestRef = useRef<PanelSizes>({
    treeWidth: panelSizes?.treeWidth ?? 300,
    subPanelHeight: panelSizes?.subPanelHeight ?? 200,
    treePaneHeight: panelSizes?.treePaneHeight,
  });

  const handleHorizontalChange = useCallback(
    (sizes: number[]) => {
      if (sizes.length < 1 || !Number.isFinite(sizes[0])) return;
      latestRef.current = { ...latestRef.current, treeWidth: sizes[0] };
      savePanelSizes(latestRef.current);
    },
    [savePanelSizes],
  );

  const handleVerticalChange = useCallback(
    (sizes: number[]) => {
      if (sizes.length < 2 || !Number.isFinite(sizes[0]) || !Number.isFinite(sizes[1])) return;
      latestRef.current = {
        ...latestRef.current,
        treePaneHeight: sizes[0],
        subPanelHeight: sizes[1],
      };
      savePanelSizes(latestRef.current);
    },
    [savePanelSizes],
  );

  return (
    <div className="h-full" data-testid="main-layout">
      <ErrorBoundary>
        <Allotment onChange={handleHorizontalChange}>
          <Allotment.Pane
            preferredSize={panelSizes?.treeWidth ?? 300}
            minSize={200}
          >
            <aside className="h-full" data-testid="left-panel">
              <Allotment vertical onChange={handleVerticalChange}>
                <Allotment.Pane>
                  <TreePanel />
                </Allotment.Pane>
                <Allotment.Pane
                  preferredSize={panelSizes?.subPanelHeight ?? '30%'}
                  minSize={100}
                >
                  <ErrorBoundary>
                    <ObjectSourcePanel />
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
