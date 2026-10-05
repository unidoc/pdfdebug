/**
 * @file Main inspection layout: the left navigator rail, then a resizable
 * split of the left panel (the active navigator over the object source) and
 * the detail panel. Panel sizes are persisted to localStorage via
 * useWindowPersistence.
 */
import { useCallback, useRef } from 'react';
import { Allotment } from 'allotment';
import 'allotment/dist/style.css';
import { ErrorBoundary } from './ErrorBoundary';
import { ObjectSourcePanel } from './ObjectInfoPanel';
import { DetailPanel } from './DetailPanel';
import { LeftRail, leftRailPanelId, leftRailTabId, resolveLeftViewIndex } from './LeftRail';
import { LEFT_RAIL_DESTINATIONS } from './leftRailDestinations';
import { useWindowPersistence, type PanelSizes } from '../hooks/useWindowPersistence';
import { selectActiveLeftView, useAppState } from '../hooks/useDocumentState';
import { useLatest } from '../hooks/useLatest';

/**
 * Rail plus resizable panel layout. The rail sits outside the Allotment so the
 * horizontal split keeps two panes and sizes[0] stays the left panel width.
 * The panel shown is the active tab's rail view, so a tab switch can change it.
 * Every navigator panel stays mounted; the inactive ones are invisible, not
 * unmounted, so a tree keeps its expansion and can still perform reveals.
 * Each panel is wrapped in an ErrorBoundary so a crash in one panel does not
 * tear down the others.
 */
export function MainLayout() {
  const { panelSizes, savePanelSizes } = useWindowPersistence();
  const state = useAppState();
  const { leftPanelCollapsed } = state;
  const activeIndex = resolveLeftViewIndex(selectActiveLeftView(state));
  const collapsedRef = useLatest(leftPanelCollapsed);

  // Track the latest sizes from each split so we can save both dimensions together.
  const latestRef = useRef<PanelSizes>({
    treeWidth: panelSizes?.treeWidth ?? 300,
    subPanelHeight: panelSizes?.subPanelHeight ?? 200,
    treePaneHeight: panelSizes?.treePaneHeight,
  });

  const handleHorizontalChange = useCallback(
    (sizes: number[]) => {
      if (sizes.length < 1 || !Number.isFinite(sizes[0])) return;
      // A hidden pane reports its width as 0, and a stored width of 0 makes
      // the next launch discard every stored size, so neither is saved.
      if (collapsedRef.current || sizes[0] <= 0) return;
      latestRef.current = { ...latestRef.current, treeWidth: sizes[0] };
      savePanelSizes(latestRef.current);
    },
    [savePanelSizes, collapsedRef],
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
    <div className="flex h-full" data-testid="main-layout">
      <LeftRail />
      <div className="flex-1 min-w-0 h-full">
        <ErrorBoundary>
          <Allotment onChange={handleHorizontalChange}>
            <Allotment.Pane
              preferredSize={panelSizes?.treeWidth ?? 300}
              minSize={200}
              visible={!leftPanelCollapsed}
            >
              <aside className="h-full" data-testid="left-panel">
                <Allotment vertical onChange={handleVerticalChange}>
                  <Allotment.Pane>
                    <div className="relative h-full">
                      {LEFT_RAIL_DESTINATIONS.map((dest, index) => {
                        const Panel = dest.panel;
                        const shown = index === activeIndex;
                        return (
                          <div
                            key={dest.id}
                            role="tabpanel"
                            id={leftRailPanelId(dest.id)}
                            aria-labelledby={leftRailTabId(dest.id)}
                            className={`absolute inset-0 ${shown ? '' : 'invisible'}`}
                          >
                            <Panel active={shown && !leftPanelCollapsed} />
                          </div>
                        );
                      })}
                    </div>
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
    </div>
  );
}
