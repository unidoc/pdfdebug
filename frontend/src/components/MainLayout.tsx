import { Allotment } from 'allotment';
import 'allotment/dist/style.css';
import { ErrorBoundary } from './ErrorBoundary';
import { useAppState, type TreeNode } from '../hooks/useDocumentState';

function TreeNodeItem({ node, indent }: { node: TreeNode; indent: number }) {
  const isError = node.error !== '';
  return (
    <div
      className={`flex items-center py-0.5 text-sm ${isError ? 'text-error' : 'text-text'}`}
      style={{ paddingLeft: `${indent * 16}px` }}
    >
      {node.hasChildren && <span className="mr-1 text-text-muted">v</span>}
      <span>{node.label}</span>
    </div>
  );
}

export function MainLayout() {
  const { tabs, activeTabId } = useAppState();
  const activeTab = tabs.find((t) => t.tabId === activeTabId);
  const rootNode = activeTab?.rootNode ?? null;
  const rootChildren = activeTab?.rootChildren ?? null;

  return (
    <div className="h-full" data-testid="main-layout">
      <ErrorBoundary>
        <Allotment>
          <Allotment.Pane preferredSize={300} minSize={200}>
            <aside className="h-full p-3 overflow-auto" data-testid="left-panel">
              {rootNode ? (
                <div>
                  <TreeNodeItem node={rootNode} indent={0} />
                  {rootChildren && rootChildren.map((child) => (
                    <TreeNodeItem key={child.id} node={child} indent={1} />
                  ))}
                </div>
              ) : (
                <span className="text-text-muted text-sm">Tree Panel</span>
              )}
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
