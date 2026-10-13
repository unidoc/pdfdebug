/**
 * platform-aware Cmd+K / Ctrl+K modifier for the command palette.
 *
 * So on macOS, Ctrl+K must be a no-op (palette stays closed) and only Cmd+K
 * opens it; on Linux/Windows, Ctrl+K opens it.
 *
 * getPlatformModifier is mocked via vi.mock('../lib/platform', ...) per the AC.
 * The hook reads tabActivationVersion from useAppState, so renderHook is wrapped
 * in AppProvider.
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
// On macOS (modifier === 'Cmd'), Ctrl+K is a no-op: the handler reads
// e.metaKey only, so the control key never opens the palette there.
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
// On macOS, Cmd+K opens the palette.
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
// On Linux/Windows (modifier === 'Ctrl'), Ctrl+K opens the palette.
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
// On Linux/Windows, Cmd+K does nothing: the non-mac branch reads e.ctrlKey
// only, so a metaKey press never opens the palette.
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
