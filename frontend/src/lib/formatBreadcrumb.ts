/**
 * Joins path elements into the breadcrumb format used across the app: the
 * structure tree's Copy path, the command palette's location line and the
 * Images panel's resource-name paths.
 */
export function formatBreadcrumb(parts: string[]): string {
  return parts.join(' > ');
}
