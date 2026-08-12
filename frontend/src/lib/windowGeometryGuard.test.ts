/**
 * Unit tests for computeRestorePlan -- the off-screen-guard math extracted
 * from App.jsx so it can be tested in isolation against synthetic
 * Screens.GetAll() payloads.
 *
 * Story 8-4 traceability follow-up: closes the AC#2 PARTIAL coverage gap
 * (WorkArea intersection logic had no unit test).
 *
 * Run: cd frontend && npx vitest run src/lib/windowGeometryGuard.test.ts
 */
import { describe, test, expect } from 'vitest';
import { computeRestorePlan } from './windowGeometryGuard';
import type { ScreenLike } from './windowGeometryGuard';

function screen(workArea: { X: number; Y: number; Width: number; Height: number }): ScreenLike {
  return { WorkArea: workArea };
}

const PRIMARY: ScreenLike = screen({ X: 0, Y: 0, Width: 1920, Height: 1040 });
const SECONDARY_RIGHT: ScreenLike = screen({ X: 1920, Y: 0, Width: 2560, Height: 1400 });
const SECONDARY_LEFT: ScreenLike = screen({ X: -1920, Y: 0, Width: 1920, Height: 1040 });

describe('computeRestorePlan', () => {
  describe('multi-monitor primary path', () => {
    test('on-screen geometry on primary display: positionAllowed = true', () => {
      const plan = computeRestorePlan(
        { x: 100, y: 100, width: 1280, height: 800 },
        [PRIMARY],
        null,
      );
      expect(plan.positionAllowed).toBe(true);
      expect(plan.width).toBe(1280);
      expect(plan.height).toBe(800);
    });

    test('geometry intersects when only the right edge sits on a screen', () => {
      // Left edge at -100 (off-screen), right edge at 100 (on primary).
      // Half overlap is enough to keep position.
      const plan = computeRestorePlan(
        { x: -100, y: 50, width: 200, height: 400 },
        [PRIMARY],
        null,
      );
      expect(plan.positionAllowed).toBe(true);
    });

    test('geometry on a connected secondary monitor: positionAllowed = true', () => {
      const plan = computeRestorePlan(
        { x: 2200, y: 100, width: 1280, height: 800 },
        [PRIMARY, SECONDARY_RIGHT],
        null,
      );
      expect(plan.positionAllowed).toBe(true);
    });

    test('geometry on a left-of-primary secondary monitor (negative x)', () => {
      const plan = computeRestorePlan(
        { x: -1500, y: 100, width: 1024, height: 768 },
        [PRIMARY, SECONDARY_LEFT],
        null,
      );
      expect(plan.positionAllowed).toBe(true);
    });

    test('monitor-disconnected: geometry was on a now-absent screen, positionAllowed = false', () => {
      // Geometry sits at (3000, 100) which used to be on SECONDARY_RIGHT.
      // After disconnect, only PRIMARY is connected.
      const plan = computeRestorePlan(
        { x: 3000, y: 100, width: 1280, height: 800 },
        [PRIMARY],
        null,
      );
      expect(plan.positionAllowed).toBe(false);
    });

    test('far off-screen (e.g. -3000, -2000): positionAllowed = false', () => {
      const plan = computeRestorePlan(
        { x: -3000, y: -2000, width: 1280, height: 800 },
        [PRIMARY],
        null,
      );
      expect(plan.positionAllowed).toBe(false);
      // Size still applies even when position is rejected.
      expect(plan.width).toBe(1280);
      expect(plan.height).toBe(800);
    });

    test('geometry exactly aligned to screen edge (x = WorkArea.X + WorkArea.Width) is rejected', () => {
      // Right edge of primary is x=1920. A window whose left edge is at 1920
      // does not overlap (strict inequality).
      const plan = computeRestorePlan(
        { x: 1920, y: 0, width: 800, height: 600 },
        [PRIMARY],
        null,
      );
      expect(plan.positionAllowed).toBe(false);
    });
  });

  describe('size-clamp behavior', () => {
    test('width and height clamped DOWN to the largest WorkArea', () => {
      const plan = computeRestorePlan(
        { x: 0, y: 0, width: 5000, height: 4000 },
        [PRIMARY, SECONDARY_RIGHT],
        null,
      );
      // Largest width across screens is SECONDARY_RIGHT.Width = 2560.
      // Largest height is SECONDARY_RIGHT.Height = 1400.
      expect(plan.width).toBe(2560);
      expect(plan.height).toBe(1400);
    });

    test('size never grows -- a small window is left unchanged', () => {
      const plan = computeRestorePlan(
        { x: 0, y: 0, width: 800, height: 600 },
        [PRIMARY],
        null,
      );
      expect(plan.width).toBe(800);
      expect(plan.height).toBe(600);
    });
  });

  describe('fallback path: empty / null Screens', () => {
    test('empty screens array uses fallback: on-screen geometry allowed', () => {
      const plan = computeRestorePlan(
        { x: 100, y: 100, width: 1024, height: 768 },
        [],
        { availWidth: 1920, availHeight: 1040 },
      );
      expect(plan.positionAllowed).toBe(true);
    });

    test('empty screens array + far-off-screen geometry: position rejected', () => {
      const plan = computeRestorePlan(
        { x: -3000, y: -2000, width: 1280, height: 800 },
        [],
        { availWidth: 1920, availHeight: 1040 },
      );
      expect(plan.positionAllowed).toBe(false);
    });

    test('null screens uses fallback', () => {
      const plan = computeRestorePlan(
        { x: 100, y: 100, width: 1024, height: 768 },
        null,
        { availWidth: 1920, availHeight: 1040 },
      );
      expect(plan.positionAllowed).toBe(true);
    });

    test('fallback rejects when window edges past 100px margin', () => {
      // Right edge of geometry < 100: x + width < 100
      const plan1 = computeRestorePlan(
        { x: -2000, y: 100, width: 1000, height: 600 },
        [],
        { availWidth: 1920, availHeight: 1040 },
      );
      expect(plan1.positionAllowed).toBe(false);

      // Bottom edge < 100: y + height < 100
      const plan2 = computeRestorePlan(
        { x: 100, y: -700, width: 1000, height: 600 },
        [],
        { availWidth: 1920, availHeight: 1040 },
      );
      expect(plan2.positionAllowed).toBe(false);

      // x > availWidth - 100
      const plan3 = computeRestorePlan(
        { x: 1900, y: 100, width: 1000, height: 600 },
        [],
        { availWidth: 1920, availHeight: 1040 },
      );
      expect(plan3.positionAllowed).toBe(false);

      // y > availHeight - 100
      const plan4 = computeRestorePlan(
        { x: 100, y: 1000, width: 1000, height: 600 },
        [],
        { availWidth: 1920, availHeight: 1040 },
      );
      expect(plan4.positionAllowed).toBe(false);
    });

    test('fallback clamps size to availWidth/availHeight', () => {
      const plan = computeRestorePlan(
        { x: 0, y: 0, width: 5000, height: 4000 },
        [],
        { availWidth: 1920, availHeight: 1040 },
      );
      expect(plan.width).toBe(1920);
      expect(plan.height).toBe(1040);
    });
  });

  describe('degenerate inputs', () => {
    test('all WorkArea fields non-finite: falls through to fallback', () => {
      const corruptScreens: ScreenLike[] = [
        { WorkArea: { X: NaN, Y: 0, Width: 1920, Height: 1040 } },
        { WorkArea: { X: 0, Y: Infinity, Width: 1920, Height: 1040 } },
      ];
      const plan = computeRestorePlan(
        { x: 100, y: 100, width: 1024, height: 768 },
        corruptScreens,
        { availWidth: 1920, availHeight: 1040 },
      );
      // No valid WorkArea -- fallback path takes over and accepts on-screen geometry.
      expect(plan.positionAllowed).toBe(true);
    });

    test('mixed valid + invalid WorkAreas: only valid ones contribute', () => {
      const mixed: ScreenLike[] = [
        { WorkArea: { X: NaN, Y: 0, Width: 1920, Height: 1040 } }, // invalid
        PRIMARY,
      ];
      const plan = computeRestorePlan(
        { x: 100, y: 100, width: 1024, height: 768 },
        mixed,
        null,
      );
      // PRIMARY is valid and accepts the geometry.
      expect(plan.positionAllowed).toBe(true);
    });

    test('null WorkArea on a Screen entry is skipped, not treated as match', () => {
      const screens: ScreenLike[] = [{ WorkArea: null }, PRIMARY];
      const plan = computeRestorePlan(
        { x: 100, y: 100, width: 1024, height: 768 },
        screens,
        null,
      );
      expect(plan.positionAllowed).toBe(true);
    });

    test('no screens and no fallback: defaults to positionAllowed = true with original size', () => {
      const plan = computeRestorePlan(
        { x: 100, y: 100, width: 1024, height: 768 },
        null,
        null,
      );
      // No reliable bound -- caller is expected to apply size restore as-is.
      expect(plan.positionAllowed).toBe(true);
      expect(plan.width).toBe(1024);
      expect(plan.height).toBe(768);
    });

    test('fallback with non-finite availWidth: position guard skipped, size unchanged', () => {
      const plan = computeRestorePlan(
        { x: -3000, y: -2000, width: 1024, height: 768 },
        null,
        { availWidth: NaN, availHeight: 1040 },
      );
      // Without a finite bound, the guard cannot reject -- positionAllowed stays true.
      expect(plan.positionAllowed).toBe(true);
      expect(plan.width).toBe(1024);
    });

    test('fallback with zero availWidth/Height: guard skipped', () => {
      const plan = computeRestorePlan(
        { x: -3000, y: -2000, width: 1024, height: 768 },
        null,
        { availWidth: 0, availHeight: 0 },
      );
      expect(plan.positionAllowed).toBe(true);
    });
  });
});
