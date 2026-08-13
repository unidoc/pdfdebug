/**
 * Story 10.2: Find Bar in Plain Text View -- FindBar component red-phase suite.
 *
 * TDD RED PHASE: every test below fails until frontend/src/components/FindBar.tsx
 * is implemented per Task 4.
 *
 * Scope:
 * - Static structure (role, aria-labels, data-testids)
 * - Keyboard wiring (Enter, Shift+Enter, Up/Down, arrow fall-through)
 * - onQueryChange / onNext / onPrev / onCaseToggle / onClose callbacks
 * - Match count text "n of m" + "0 of 0"
 * - aria-pressed reflects caseSensitive prop
 * - non-Latin-1 hint visibility + aria-describedby
 * - prev/next disabled when matches.length === 0
 * - Wrap-status testid mounts only when wrapped !== null
 * - aria-live="polite" on count + wrap-status
 *
 * Test IDs follow the convention.
 *
 * Run: cd frontend && npx vitest run src/components/FindBar.test.tsx
 */
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, test, expect, vi } from 'vitest';
// RED PHASE: this import fails until Task 4.1 lands.
import { FindBar } from './FindBar';
import type { Match } from '../lib/findMatches';

// Minimal Match factory for component tests. The FindBar does not depend on
// the actual offsets -- it consumes matches.length, activeIndex, and the
// surrounding state. The shape must match the exported Match interface.
function fakeMatches(count: number): Match[] {
  return Array.from({ length: count }, (_, i) => ({
    start: i * 10,
    end: i * 10 + 3,
    line: i + 1,
  }));
}

interface RenderOpts {
  matches?: Match[];
  activeIndex?: number;
  query?: string;
  caseSensitive?: boolean;
  wrapped?: 'top' | 'bottom' | null;
  nonLatin1?: boolean;
}

function renderBar(opts: RenderOpts = {}) {
  const onQueryChange = vi.fn();
  const onNext = vi.fn();
  const onPrev = vi.fn();
  const onCaseToggle = vi.fn();
  const onClose = vi.fn();
  render(
    <FindBar
      matches={opts.matches ?? []}
      activeIndex={opts.activeIndex ?? 0}
      query={opts.query ?? ''}
      caseSensitive={opts.caseSensitive ?? false}
      wrapped={opts.wrapped ?? null}
      nonLatin1={opts.nonLatin1 ?? false}
      onQueryChange={onQueryChange}
      onNext={onNext}
      onPrev={onPrev}
      onCaseToggle={onCaseToggle}
      onClose={onClose}
    />,
  );
  return { onQueryChange, onNext, onPrev, onCaseToggle, onClose };
}

// ---------------------------------------------------------------------------
// Static structure -- role + aria-label root.
// ---------------------------------------------------------------------------

describe('root role + aria-label', () => {
  test('root element carries role="search" and aria-label="Find in plain text"', () => {
    renderBar();
    const bar = screen.getByTestId('plain-text-find-bar');
    expect(bar.getAttribute('role')).toBe('search');
    expect(bar.getAttribute('aria-label')).toBe('Find in plain text');
  });
});

// ---------------------------------------------------------------------------
// Input renders with aria-label="Find query".
// ---------------------------------------------------------------------------

describe('input', () => {
  test('input testid is plain-text-find-input and aria-label="Find query"', () => {
    renderBar();
    const input = screen.getByTestId('plain-text-find-input');
    expect(input.tagName).toBe('INPUT');
    expect(input.getAttribute('aria-label')).toBe('Find query');
  });

  test('typing fires onQueryChange with the new value', () => {
    const { onQueryChange } = renderBar();
    const input = screen.getByTestId('plain-text-find-input') as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'helvetica' } });
    expect(onQueryChange).toHaveBeenCalledWith('helvetica');
  });
});

// ---------------------------------------------------------------------------
// Match count text.
// ---------------------------------------------------------------------------

describe('match count', () => {
  test('empty query -> "0 of 0"', () => {
    renderBar({ query: '', matches: [] });
    expect(screen.getByTestId('plain-text-find-count').textContent).toBe('0 of 0');
  });

  test('non-empty query, no matches -> "0 of 0"', () => {
    renderBar({ query: 'xyz', matches: [] });
    expect(screen.getByTestId('plain-text-find-count').textContent).toBe('0 of 0');
  });

  test('5 matches, activeIndex=2 -> "3 of 5"', () => {
    renderBar({ query: 'foo', matches: fakeMatches(5), activeIndex: 2 });
    expect(screen.getByTestId('plain-text-find-count').textContent).toBe('3 of 5');
  });

  test('count element has aria-live="polite"', () => {
    renderBar();
    expect(screen.getByTestId('plain-text-find-count').getAttribute('aria-live')).toBe('polite');
  });
});

// ---------------------------------------------------------------------------
// case-toggle button.
// ---------------------------------------------------------------------------

describe('case toggle', () => {
  test('case-toggle button renders with aria-label="Match case"', () => {
    renderBar();
    const toggle = screen.getByTestId('plain-text-find-case-toggle');
    expect(toggle.getAttribute('aria-label')).toBe('Match case');
  });

  test('aria-pressed reflects caseSensitive prop', () => {
    const { rerender } = render(
      <FindBar
        matches={[]}
        activeIndex={0}
        query=""
        caseSensitive={false}
        wrapped={null}
        nonLatin1={false}
        onQueryChange={vi.fn()}
        onNext={vi.fn()}
        onPrev={vi.fn()}
        onCaseToggle={vi.fn()}
        onClose={vi.fn()}
      />,
    );
    expect(screen.getByTestId('plain-text-find-case-toggle').getAttribute('aria-pressed')).toBe('false');
    rerender(
      <FindBar
        matches={[]}
        activeIndex={0}
        query=""
        caseSensitive={true}
        wrapped={null}
        nonLatin1={false}
        onQueryChange={vi.fn()}
        onNext={vi.fn()}
        onPrev={vi.fn()}
        onCaseToggle={vi.fn()}
        onClose={vi.fn()}
      />,
    );
    expect(screen.getByTestId('plain-text-find-case-toggle').getAttribute('aria-pressed')).toBe('true');
  });

  test('clicking the case toggle fires onCaseToggle', () => {
    const { onCaseToggle } = renderBar();
    fireEvent.click(screen.getByTestId('plain-text-find-case-toggle'));
    expect(onCaseToggle).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// Prev / next / close buttons.
// ---------------------------------------------------------------------------

describe('navigation + close buttons', () => {
  test('prev button: aria-label="Previous match", click fires onPrev', () => {
    const { onPrev } = renderBar({ matches: fakeMatches(2) });
    const prev = screen.getByTestId('plain-text-find-prev');
    expect(prev.getAttribute('aria-label')).toBe('Previous match');
    fireEvent.click(prev);
    expect(onPrev).toHaveBeenCalledTimes(1);
  });

  test('next button: aria-label="Next match", click fires onNext', () => {
    const { onNext } = renderBar({ matches: fakeMatches(2) });
    const next = screen.getByTestId('plain-text-find-next');
    expect(next.getAttribute('aria-label')).toBe('Next match');
    fireEvent.click(next);
    expect(onNext).toHaveBeenCalledTimes(1);
  });

  test('close button: aria-label="Close find", click fires onClose', () => {
    const { onClose } = renderBar();
    const close = screen.getByTestId('plain-text-find-close');
    expect(close.getAttribute('aria-label')).toBe('Close find');
    fireEvent.click(close);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// prev/next disabled when no matches.
// ---------------------------------------------------------------------------

describe('prev/next disabled when matches empty', () => {
  test('matches.length === 0 -> prev disabled + aria-disabled="true"', () => {
    renderBar({ matches: [] });
    const prev = screen.getByTestId('plain-text-find-prev') as HTMLButtonElement;
    expect(prev.disabled).toBe(true);
    expect(prev.getAttribute('aria-disabled')).toBe('true');
  });

  test('matches.length === 0 -> next disabled + aria-disabled="true"', () => {
    renderBar({ matches: [] });
    const next = screen.getByTestId('plain-text-find-next') as HTMLButtonElement;
    expect(next.disabled).toBe(true);
    expect(next.getAttribute('aria-disabled')).toBe('true');
  });

  test('matches.length > 0 -> prev/next enabled', () => {
    renderBar({ matches: fakeMatches(3) });
    const prev = screen.getByTestId('plain-text-find-prev') as HTMLButtonElement;
    const next = screen.getByTestId('plain-text-find-next') as HTMLButtonElement;
    expect(prev.disabled).toBe(false);
    expect(next.disabled).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// wrap-status mounts only when wrapped !== null.
// ---------------------------------------------------------------------------

describe('wrap-status rendering', () => {
  test('wrapped=null -> wrap-status testid absent', () => {
    renderBar({ wrapped: null });
    expect(screen.queryByTestId('plain-text-find-wrap-status')).toBeNull();
  });

  test('wrapped="top" -> "Wrapped to top" text appears', () => {
    renderBar({ wrapped: 'top' });
    const status = screen.getByTestId('plain-text-find-wrap-status');
    expect(status.textContent).toBe('Wrapped to top');
  });

  test('wrapped="bottom" -> "Wrapped to bottom" text appears', () => {
    renderBar({ wrapped: 'bottom' });
    const status = screen.getByTestId('plain-text-find-wrap-status');
    expect(status.textContent).toBe('Wrapped to bottom');
  });

  test('wrap-status element has aria-live="polite"', () => {
    renderBar({ wrapped: 'top' });
    const status = screen.getByTestId('plain-text-find-wrap-status');
    expect(status.getAttribute('aria-live')).toBe('polite');
  });
});

// ---------------------------------------------------------------------------
// non-Latin-1 hint mount + aria-describedby linkage.
// ---------------------------------------------------------------------------

describe('non-Latin-1 hint', () => {
  test('nonLatin1=false -> hint absent + input has no aria-describedby', () => {
    renderBar({ nonLatin1: false });
    expect(screen.queryByTestId('plain-text-find-non-latin1-hint')).toBeNull();
    const input = screen.getByTestId('plain-text-find-input');
    expect(input.getAttribute('aria-describedby')).toBeNull();
  });

  test('nonLatin1=true -> hint visible with exact copy', () => {
    renderBar({ nonLatin1: true, query: 'a→b' });
    const hint = screen.getByTestId('plain-text-find-non-latin1-hint');
    expect(hint.textContent).toBe("Non-Latin-1 characters won't match");
    expect(hint.id).toBe('plain-text-find-non-latin1-hint');
  });

  test('nonLatin1=true -> input aria-describedby links to the hint id', () => {
    renderBar({ nonLatin1: true, query: 'a→b' });
    const input = screen.getByTestId('plain-text-find-input');
    expect(input.getAttribute('aria-describedby')).toBe('plain-text-find-non-latin1-hint');
  });
});

// ---------------------------------------------------------------------------
// Keyboard wiring on the input.
// ---------------------------------------------------------------------------

describe('keyboard wiring on the input', () => {
  test('Enter on input fires onNext', () => {
    const { onNext } = renderBar({ matches: fakeMatches(3) });
    const input = screen.getByTestId('plain-text-find-input');
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onNext).toHaveBeenCalledTimes(1);
  });

  test('Shift+Enter on input fires onPrev', () => {
    const { onPrev } = renderBar({ matches: fakeMatches(3) });
    const input = screen.getByTestId('plain-text-find-input');
    fireEvent.keyDown(input, { key: 'Enter', shiftKey: true });
    expect(onPrev).toHaveBeenCalledTimes(1);
  });

  test('ArrowDown on input fires onNext', () => {
    const { onNext } = renderBar({ matches: fakeMatches(3) });
    const input = screen.getByTestId('plain-text-find-input');
    fireEvent.keyDown(input, { key: 'ArrowDown' });
    expect(onNext).toHaveBeenCalledTimes(1);
  });

  test('ArrowUp on input fires onPrev', () => {
    const { onPrev } = renderBar({ matches: fakeMatches(3) });
    const input = screen.getByTestId('plain-text-find-input');
    fireEvent.keyDown(input, { key: 'ArrowUp' });
    expect(onPrev).toHaveBeenCalledTimes(1);
  });

  test('ArrowLeft on input does NOT fire onPrev (falls through to native caret movement)', () => {
    const { onPrev } = renderBar({ matches: fakeMatches(3) });
    const input = screen.getByTestId('plain-text-find-input');
    fireEvent.keyDown(input, { key: 'ArrowLeft' });
    expect(onPrev).not.toHaveBeenCalled();
  });

  test('ArrowRight on input does NOT fire onNext (falls through to native caret movement)', () => {
    const { onNext } = renderBar({ matches: fakeMatches(3) });
    const input = screen.getByTestId('plain-text-find-input');
    fireEvent.keyDown(input, { key: 'ArrowRight' });
    expect(onNext).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Tab order is input -> case toggle -> prev -> next -> close. We assert by
// reading the DOM order of focusable elements.
// ---------------------------------------------------------------------------

describe('tab order', () => {
  test('focusable testids appear in input, case-toggle, prev, next, close order', () => {
    renderBar({ matches: fakeMatches(2) });
    const order = [
      'plain-text-find-input',
      'plain-text-find-case-toggle',
      'plain-text-find-prev',
      'plain-text-find-next',
      'plain-text-find-close',
    ];
    const positions = order.map((tid) => {
      const el = screen.getByTestId(tid);
      // documentPosition mask 0x04 = following. The previous element should
      // appear before each subsequent element in document order.
      return el;
    });
    for (let i = 0; i < positions.length - 1; i++) {
      const cur = positions[i];
      const nxt = positions[i + 1];
      // eslint-disable-next-line no-bitwise
      const followingMask = cur.compareDocumentPosition(nxt) & Node.DOCUMENT_POSITION_FOLLOWING;
      expect(followingMask).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    }
  });
});

// ---------------------------------------------------------------------------
// Input has focus on mount (autofocus contract).
// ---------------------------------------------------------------------------

describe('input autofocus on mount', () => {
  test('input is the active element immediately after render', () => {
    renderBar();
    const input = screen.getByTestId('plain-text-find-input');
    expect(document.activeElement).toBe(input);
  });
});
