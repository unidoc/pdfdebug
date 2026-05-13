/**
 * @file Frontend types for the Cmd+K command palette (Story 9-8).
 * Mirrors the backend ObjectIndexEntry struct -- kept as a hand-rolled TS
 * type rather than imported from the Wails binding so unit tests can build
 * fixtures without pulling in the Wails runtime mock.
 */

/** One row in the per-document object index. */
export interface ObjectIndexEntry {
  objNum: number;
  gen: number;
  /** Literal /Type value of the dict; "" when no /Type key. */
  typeName: string;
  /** True when the xref slot is marked free. */
  free: boolean;
  /** True when the object is reachable from the catalog via the dict graph. */
  reachable: boolean;
  /**
   * "obj:<gen>:<num>" for reachable objects; "" for free/orphan entries
   * (cannot be navigated to via the existing NAVIGATE_TO_REF path).
   */
  nodeId: string;
}
