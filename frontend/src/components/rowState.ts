/**
 * @file Row-state class strings shared by every list and tree row: the
 * selected fill (with the readable-text overrides from style.css), the 2px
 * focus bar, and the idle state that keeps a transparent bar so rows align.
 */

/** Selected-row fill plus the scoped text-colour overrides that keep descendant text readable on it. */
export const ROW_SELECTED_FILL = 'bg-row-selected row-selected-text';

/** Selected row: fill plus the 2px focus bar on the left. */
export const ROW_SELECTED = `${ROW_SELECTED_FILL} border-l-2 border-l-border-focus`;

/** Transparent 2px left bar, so unselected rows line up with the selected one. */
export const ROW_IDLE_BAR = 'border-l-2 border-l-transparent';

/** Unselected clickable row: transparent bar and the grey hover fill. */
export const ROW_IDLE = `${ROW_IDLE_BAR} hover:bg-surface-hover`;

/**
 * Selected table row. A `<tr>` border does not render under border-collapse,
 * so the fill goes on the row and the focus bar is an inset shadow on its
 * first cell (TABLE_CELL_SELECTED_BAR).
 */
export const TABLE_ROW_SELECTED = ROW_SELECTED_FILL;

/** Focus bar for the first cell of a selected table row. */
export const TABLE_CELL_SELECTED_BAR = 'shadow-[inset_2px_0_0_var(--color-border-focus)]';

/** Unselected clickable table row: grey hover fill. */
export const TABLE_ROW_IDLE = 'hover:bg-surface-hover';
