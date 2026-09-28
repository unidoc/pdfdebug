/**
 * @file The left rail's destination registry. The rail's items, their order,
 * keyboard bounds, Cmd/Ctrl+digit shortcuts, the persisted view id and the
 * left panel's contents all derive from this array; adding a destination is
 * one entry here.
 */
import type { ComponentType } from 'react';
import { ListTree, Files, type LucideIcon } from 'lucide-react';
import { TreePanel } from './TreePanel';
import { PagesPanel } from './PagesPanel';

/** Props every destination panel receives. */
export interface LeftRailPanelProps {
  /** True while this destination is the one shown in the left panel. */
  active: boolean;
}

/** One left-rail destination: a navigator that fills the left panel. */
export interface LeftRailDestination {
  /** Stable id, persisted as the selected view. */
  id: string;
  /** Always-visible text label under the icon. */
  label: string;
  icon: LucideIcon;
  /** Component rendered in the left panel's top pane. */
  panel: ComponentType<LeftRailPanelProps>;
}

/** Ordered left-rail destinations. */
export const LEFT_RAIL_DESTINATIONS: readonly LeftRailDestination[] = [
  { id: 'structure', label: 'Structure', icon: ListTree, panel: TreePanel },
  { id: 'pages', label: 'Pages', icon: Files, panel: PagesPanel },
];
