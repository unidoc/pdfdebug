/**
 * Off-screen guard and size-clamp logic for restoring persisted window
 * geometry on startup.
 *
 * Extracted from App.jsx so the math is unit-testable in isolation against
 * synthetic Screens.GetAll() payloads (multi-monitor, monitor-disconnected,
 * empty-array fallback paths).
 *
 * Task 5.2 (off-screen guard) and Task 5.3 (size clamp).
 */
import type { WindowGeometry } from '../hooks/useWindowPersistence';

/** Subset of `@wailsio/runtime` Rect used by the guard. */
export interface WorkAreaRect {
  X: number;
  Y: number;
  Width: number;
  Height: number;
}

/** Subset of `@wailsio/runtime` Screen used by the guard. */
export interface ScreenLike {
  WorkArea?: WorkAreaRect | null;
}

/** Fallback single-screen bounds (used when Screens.GetAll() rejects/empty). */
export interface FallbackScreen {
  availWidth: number;
  availHeight: number;
}

/** Decision returned by computeRestorePlan: clamped size + position-allowed flag. */
export interface RestorePlan {
  width: number;
  height: number;
  positionAllowed: boolean;
}

/** True when a Rect has all four fields as finite numbers. */
function isFiniteRect(wa: WorkAreaRect | null | undefined): wa is WorkAreaRect {
  if (!wa) return false;
  return (
    Number.isFinite(wa.X) &&
    Number.isFinite(wa.Y) &&
    Number.isFinite(wa.Width) &&
    Number.isFinite(wa.Height)
  );
}

/** True when geometry's rectangle intersects the WorkArea's rectangle. */
function intersects(geometry: WindowGeometry, wa: WorkAreaRect): boolean {
  const x1 = geometry.x;
  const y1 = geometry.y;
  const x2 = geometry.x + geometry.width;
  const y2 = geometry.y + geometry.height;
  return x1 < wa.X + wa.Width && x2 > wa.X && y1 < wa.Y + wa.Height && y2 > wa.Y;
}

/**
 * Decide whether the persisted geometry is safe to restore as-is and clamp
 * the size to fit the largest available WorkArea.
 *
 * Primary path: when `screens` has at least one Screen with a finite WorkArea,
 * `positionAllowed` is true iff geometry intersects ANY screen's WorkArea.
 * Width/height are clamped DOWN to the largest finite WorkArea.Width /
 * WorkArea.Height (never up).
 *
 * Fallback path: when `screens` is empty/null, use `fallback` (single-screen
 * availWidth/availHeight). Position is rejected when the window's edges are
 * past the screen edges by more than 100px (keeps a sliver of titlebar
 * visible). Size is clamped to availWidth/availHeight.
 *
 * If neither path yields a reliable bound, the original geometry is returned
 * unchanged with positionAllowed = true (caller still applies size restore).
 */
export function computeRestorePlan(
  geometry: WindowGeometry,
  screens: readonly ScreenLike[] | null | undefined,
  fallback: FallbackScreen | null | undefined,
): RestorePlan {
  let width = geometry.width;
  let height = geometry.height;
  let positionAllowed = true;

  if (Array.isArray(screens) && screens.length > 0) {
    // Multi-screen primary path: WorkArea-intersection check.
    const validWorkAreas: WorkAreaRect[] = [];
    for (const s of screens) {
      const wa = s?.WorkArea;
      if (isFiniteRect(wa)) validWorkAreas.push(wa);
    }

    if (validWorkAreas.length > 0) {
      positionAllowed = validWorkAreas.some((wa) => intersects(geometry, wa));

      // Clamp DOWN to the largest available WorkArea so the restored window
      // cannot exceed any connected display.
      let maxW = 0;
      let maxH = 0;
      for (const wa of validWorkAreas) {
        if (wa.Width > maxW) maxW = wa.Width;
        if (wa.Height > maxH) maxH = wa.Height;
      }
      if (maxW > 0 && width > maxW) width = maxW;
      if (maxH > 0 && height > maxH) height = maxH;
      return { width, height, positionAllowed };
    }
    // All WorkAreas were non-finite -- fall through to the secondary path.
  }

  if (
    fallback &&
    Number.isFinite(fallback.availWidth) &&
    Number.isFinite(fallback.availHeight) &&
    fallback.availWidth > 0 &&
    fallback.availHeight > 0
  ) {
    const aw = fallback.availWidth;
    const ah = fallback.availHeight;
    if (
      geometry.x + geometry.width < 100 ||
      geometry.y + geometry.height < 100 ||
      geometry.x > aw - 100 ||
      geometry.y > ah - 100
    ) {
      positionAllowed = false;
    }
    if (width > aw) width = aw;
    if (height > ah) height = ah;
  }

  return { width, height, positionAllowed };
}
