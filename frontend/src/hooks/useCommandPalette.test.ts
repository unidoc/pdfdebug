/**
 * Story 10.8 AC2 / AC3: platform-aware Cmd+K / Ctrl+K modifier for the command
 * palette.
 *
 * TDD RED PHASE: these tests fail against the current useCommandPalette.ts,
 * which uses `const mod = e.metaKey || e.ctrlKey` (accepts EITHER modifier on
 * EVERY platform). After the fix the handler must select the modifier via
 * getPlatformModifier():
 *   const wantsMeta = getPlatformModifier() === 'Cmd';
 *   const mod = wantsMeta ? e.metaKey : e.ctrlKey;
 *
 * So on macOS, Ctrl+K must be a no-op (palette stays closed) and only Cmd+K
 * opens it; on Linux/Windows, Ctrl+K opens it.
 *
 * getPlatformModifier is mocked via vi.mock('../lib/platform', ...) per the AC.
 * The hook reads tabActivationVersion from useAppState, so renderHook is wrapped
 * in AppProvider.
 *
 * Test IDs follow the 10-8-HOOK-NNN convention.
 *
 * Run: cd frontend && npx vitest run src/hooks/useCommandPalette.test.ts
 */
import { renderHook, act } from '@testing-library/react';
import { createElement } from 'react';
import type { ReactNode } from 'react';
import { describe, test, expect, vi, beforeEach } from 'vitest';
import { AppProvider } from './useDocumentState';
import { useCommandPalette, closePalette } from './useCommandPalette';

// Mock the platform module so each test controls what getPlatformModifier
// returns. The fix under test must call this to decide which modifier opens
// the palette.
const mockGetPlatformModifier = vi.fn<() => string>();
vi.mock('../lib/platform', () => ({
  getPlatformModifier: () => mockGetPlatformModifier(),
  getShortcutHint: (key: string) => `${mockGetPlatformModifier()}+${key}`,
}));

// Wrap renderHook in AppProvider since useCommandPalette consumes useAppState.
function wrapper({ children }: { children: ReactNode }) {
  return createElement(AppProvider, null, children);
}

// Dispatch a real keydown on window so the hook's window-level listener fires.
// fireEvent.keyDown does not reliably reach window listeners under jsdom.
function dispatchK(opts: { metaKey?: boolean; ctrlKey?: boolean }) {
  const ev = new KeyboardEvent('keydown', {
    key: 'k',
    metaKey: opts.metaKey ?? false,
    ctrlKey: opts.ctrlKey ?? false,
    bubbles: true,
    cancelable: true,
  });
  window.dispatchEvent(ev);
  return ev;
}

beforeEach(() => {
  // The hook holds open state in a module-level variable; reset between tests.
  closePalette();
  vi.clearAllMocks();
});

// ---------------------------------------------------------------------------
// 10-8-HOOK-001 [P0] AC2: on macOS (modifier === 'Cmd'), Ctrl+K is a no-op.
// The current `metaKey || ctrlKey` implementation OPENS on Ctrl+K, so this
// FAILS until the platform-aware check lands.
// ---------------------------------------------------------------------------
describe('macOS - Ctrl+K does not open the palette', () => {
  test('Ctrl+K is a no-op on macOS', () => {
    mockGetPlatformModifier.mockReturnValue('Cmd');
    const { result } = renderHook(() => useCommandPalette(), { wrapper });

    expect(result.current.isOpen).toBe(false);
    act(() => {
      dispatchK({ ctrlKey: true });
    });
    expect(result.current.isOpen).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 10-8-HOOK-002 [P0] AC2: on macOS, Cmd+K opens the palette.
// ---------------------------------------------------------------------------
describe('macOS - Cmd+K opens the palette', () => {
  test('Cmd+K opens on macOS', () => {
    mockGetPlatformModifier.mockReturnValue('Cmd');
    const { result } = renderHook(() => useCommandPalette(), { wrapper });

    expect(result.current.isOpen).toBe(false);
    act(() => {
      dispatchK({ metaKey: true });
    });
    expect(result.current.isOpen).toBe(true);
  });

  test('Cmd+K calls preventDefault on macOS', () => {
    mockGetPlatformModifier.mockReturnValue('Cmd');
    renderHook(() => useCommandPalette(), { wrapper });

    const ev = new KeyboardEvent('keydown', {
      key: 'k',
      metaKey: true,
      bubbles: true,
      cancelable: true,
    });
    const spy = vi.spyOn(ev, 'preventDefault');
    act(() => {
      window.dispatchEvent(ev);
    });
    expect(spy).toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// 10-8-HOOK-003 [P0] AC3: on Linux/Windows (modifier === 'Ctrl'), Ctrl+K
// opens the palette.
// ---------------------------------------------------------------------------
describe('Linux/Windows - Ctrl+K opens the palette', () => {
  test('Ctrl+K opens on non-mac platforms', () => {
    mockGetPlatformModifier.mockReturnValue('Ctrl');
    const { result } = renderHook(() => useCommandPalette(), { wrapper });

    expect(result.current.isOpen).toBe(false);
    act(() => {
      dispatchK({ ctrlKey: true });
    });
    expect(result.current.isOpen).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// 10-8-HOOK-004 [P1] AC3: on Linux/Windows, Cmd+K does nothing (no metaKey to
// register in normal use). The current OR-based code would open on metaKey;
// after the fix the non-mac branch reads e.ctrlKey only.
// ---------------------------------------------------------------------------
describe('Linux/Windows - Cmd+K is a no-op', () => {
  test('Cmd+K does not open on non-mac platforms', () => {
    mockGetPlatformModifier.mockReturnValue('Ctrl');
    const { result } = renderHook(() => useCommandPalette(), { wrapper });

    expect(result.current.isOpen).toBe(false);
    act(() => {
      dispatchK({ metaKey: true });
    });
    expect(result.current.isOpen).toBe(false);
  });
});
