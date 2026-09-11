/**
 * @file Presentational find-bar component. Static structure
 * (input + count + case toggle + prev + next + close) plus conditional
 * non-Latin-1 hint and wrap-status. All state lives in useFindBar; this
 * component is pure props -> DOM.
 */
import { useEffect, useRef, type KeyboardEvent } from 'react';
import { X } from 'lucide-react';
import * as Tooltip from '@radix-ui/react-tooltip';

/** Props for {@link FindBar}. */
export interface FindBarProps {
  /**
   * Per-instance testid + element-id prefix. Defaults to the Plain Text values
   * so the Story 10-2 `plain-text-find-*` tests keep asserting on the same
   * testids. Object/XREF instances pass their own prefix so all three bars can
   * coexist in the force-mounted DOM without duplicate ids.
   */
  idPrefix?: string;
  /** role="search" aria-label. Defaults to the Plain Text wording. */
  label?: string;
  /** Total number of matches in the active tab's source. */
  matchCount: number;
  activeIndex: number;
  query: string;
  caseSensitive: boolean;
  wrapped: 'top' | 'bottom' | null;
  nonLatin1: boolean;
  /** Monotonic counter; on bump, the bar re-focuses + selects-all the input. */
  focusVersion?: number;
  onQueryChange: (q: string) => void;
  onNext: () => void;
  onPrev: () => void;
  onCaseToggle: () => void;
  onClose: () => void;
}

/**
 * Inline find bar mounted above a find-enabled tab's scroll container. All
 * state lives in useFindBar / useSpanFind; this component is pure props -> DOM.
 */
export function FindBar(props: FindBarProps): JSX.Element {
  const {
    idPrefix = 'plain-text-find',
    label = 'Find in plain text',
    matchCount,
    activeIndex,
    query,
    caseSensitive,
    wrapped,
    nonLatin1,
    focusVersion,
    onQueryChange,
    onNext,
    onPrev,
    onCaseToggle,
    onClose,
  } = props;

  const hintId = `${idPrefix}-non-latin1-hint`;

  const inputRef = useRef<HTMLInputElement | null>(null);

  // Autofocus on mount + on every focusVersion bump.
  useEffect(() => {
    const el = inputRef.current;
    if (!el) return;
    el.focus();
    el.select();
  }, [focusVersion]);

  const handleInputKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      if (e.shiftKey) {
        onPrev();
      } else {
        onNext();
      }
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      onNext();
      return;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      onPrev();
      return;
    }
  };

  // Esc is scoped to the FindBar root subtree so it fires no matter which
  // descendant (input, buttons) holds focus, and stopPropagation keeps it from
  // waking a window-level sibling listener (e.g. Cmd+K palette).
  const handleRootKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      onClose();
    }
  };

  const hasMatches = matchCount > 0;
  const countText = hasMatches ? `${activeIndex + 1} of ${matchCount}` : '0 of 0';

  return (
    <Tooltip.Provider delayDuration={300}>
    <div
      role="search"
      aria-label={label}
      data-testid={`${idPrefix}-bar`}
      onKeyDown={handleRootKeyDown}
      className="flex-shrink-0 flex items-center gap-2 px-2 py-1 border-b border-border bg-surface text-sm"
    >
      <div className="flex-1 min-w-0 relative">
        <input
          ref={inputRef}
          type="text"
          data-testid={`${idPrefix}-input`}
          aria-label="Find query"
          aria-describedby={nonLatin1 ? hintId : undefined}
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          onKeyDown={handleInputKeyDown}
          className="w-full px-2 py-0.5 pr-6 bg-bg border border-border rounded text-text font-mono"
          placeholder="Find"
        />
        {query !== '' && (
          <button
            type="button"
            data-testid={`${idPrefix}-clear`}
            aria-label="Clear find query"
            onClick={() => {
              onQueryChange('');
              inputRef.current?.focus();
            }}
            className="absolute right-1 top-1/2 -translate-y-1/2 flex items-center justify-center w-4 h-4 rounded text-text-muted hover:text-text hover:bg-surface-hover cursor-pointer"
          >
            <X className="w-3 h-3" />
          </button>
        )}
      </div>
      <span
        data-testid={`${idPrefix}-count`}
        aria-live="polite"
        className="text-text-muted min-w-[6ch] text-right tabular-nums"
      >
        {countText}
      </span>
      {wrapped !== null && (
        <span
          data-testid={`${idPrefix}-wrap-status`}
          aria-live="polite"
          className="text-text-muted text-xs"
        >
          {wrapped === 'top' ? 'Wrapped to top' : 'Wrapped to bottom'}
        </span>
      )}
      <Tooltip.Root>
        <Tooltip.Trigger asChild>
          <button
            type="button"
            data-testid={`${idPrefix}-case-toggle`}
            aria-label="Match case"
            aria-pressed={caseSensitive ? 'true' : 'false'}
            onClick={onCaseToggle}
            className={
              'px-2 py-0.5 rounded text-xs cursor-pointer ' +
              (caseSensitive
                ? 'bg-surface-selected border border-border-focus text-text'
                : 'bg-bg border border-border text-text-muted hover:bg-surface-hover')
            }
          >
            Aa
          </button>
        </Tooltip.Trigger>
        <Tooltip.Portal>
          <Tooltip.Content
            data-testid={`${idPrefix}-case-toggle-tooltip`}
            className="bg-surface border border-border rounded px-2 py-1 text-xs text-text shadow-md z-50"
            sideOffset={5}
          >
            {caseSensitive ? 'Match case: on' : 'Match case: off'}
          </Tooltip.Content>
        </Tooltip.Portal>
      </Tooltip.Root>
      <button
        type="button"
        data-testid={`${idPrefix}-prev`}
        aria-label="Previous match"
        aria-disabled={hasMatches ? 'false' : 'true'}
        disabled={!hasMatches}
        onClick={onPrev}
        className="px-2 py-0.5 rounded text-xs bg-bg border border-border text-text-muted hover:bg-surface-hover cursor-pointer disabled:cursor-not-allowed disabled:opacity-60"
      >
        Prev
      </button>
      <button
        type="button"
        data-testid={`${idPrefix}-next`}
        aria-label="Next match"
        aria-disabled={hasMatches ? 'false' : 'true'}
        disabled={!hasMatches}
        onClick={onNext}
        className="px-2 py-0.5 rounded text-xs bg-bg border border-border text-text-muted hover:bg-surface-hover cursor-pointer disabled:cursor-not-allowed disabled:opacity-60"
      >
        Next
      </button>
      <button
        type="button"
        data-testid={`${idPrefix}-close`}
        aria-label="Close find"
        onClick={onClose}
        className="px-2 py-0.5 rounded text-xs bg-bg border border-border text-text-muted hover:bg-surface-hover cursor-pointer"
      >
        Close
      </button>
      {nonLatin1 && (
        <span
          id={hintId}
          data-testid={`${idPrefix}-non-latin1-hint`}
          className="text-warning text-xs"
        >
          {"Non-Latin-1 characters won't match"}
        </span>
      )}
    </div>
    </Tooltip.Provider>
  );
}

export default FindBar;
