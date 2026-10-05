/**
 * @file The left rail's destination registry. The rail's items, their order,
 * keyboard bounds, Cmd/Ctrl+digit shortcuts, the selected view id and the
 * left panel's contents all derive from this array; adding a destination is
 * one entry here.
 */
import type { ComponentType } from 'react';
import { ListTree, Files, Images, type LucideIcon } from 'lucide-react';
import { TreePanel } from './TreePanel';
import { PagesPanel } from './PagesPanel';
import { ImagesPanel } from './ImagesPanel';

/** Props every destination panel receives. */
export interface LeftRailPanelProps {
  /**
   * True while this destination is the one shown in the left panel for the
   * active tab and the panel is expanded. Flips on tab switches too.
   */
  active: boolean;
}

/** One left-rail destination: a navigator that fills the left panel. */
export interface LeftRailDestination {
  /** Stable id, held in each tab's state as its selected view. */
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
  { id: 'images', label: 'Images', icon: Images, panel: ImagesPanel },
];
