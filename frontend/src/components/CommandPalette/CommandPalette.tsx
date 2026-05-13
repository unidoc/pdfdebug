/**
 * @file Cmd+K command palette overlay. Story 9-8 AC4-AC10.
 *
 * Behaviour:
 *   - Renders only when the module-level useCommandPalette state is open.
 *   - Trapped focus on the input; Esc / click-outside close.
 *   - Empty input shows recent jumps for the active tab (per-tab LRU);
 *     or a one-line grammar hint when there are no recents.
 *   - Numeric / type queries are dispatched through the existing
 *     NAVIGATE_TO_REF flow once the user commits (Enter or click).
 *   - Single-match Enter is gated by a 50ms input-idle window: an Enter
 *     pressed within 50ms of the last input change is ignored. This stops
 *     "Cmd+K, 8, Enter" (typed fast) from firing nav for object 8 when
 *     the user meant to type more.
 *   - Free/orphan rows are visible but non-navigable; Enter/click on them
 *     surfaces an inline "not reachable" notice and keeps the palette open.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { GetAncestorPath } from '../../../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js';
import { useAppDispatch, useAppState } from '../../hooks/useDocumentState';
import { closePalette, useCommandPalette } from '../../hooks/useCommandPalette';
import { useObjectIndex } from '../../hooks/useObjectIndex';
import { parseQuery } from '../../lib/palette/parseQuery';
import { rankResults } from '../../lib/palette/rankResults';
import type { ObjectIndexEntry } from '../../types/palette';
import { PaletteRow } from './PaletteRow';

const IDLE_MS = 50;

export function CommandPalette() {
  const { isOpen } = useCommandPalette();
  const { tabs, activeTabId } = useAppState();
  const dispatch = useAppDispatch();
  const { index, ensureIndex } = useObjectIndex();

  const activeTab = tabs.find((t) => t.tabId === activeTabId) ?? null;
  const recentJumps = activeTab?.recentJumps ?? [];

  const [input, setInput] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [unreachable, setUnreachable] = useState<number | null>(null);
  const [breadcrumb, setBreadcrumb] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const previouslyFocusedRef = useRef<HTMLElement | null>(null);
  const lastInputChangeRef = useRef<number>(0);

  // Lazy-fetch the index on first open per tab.
  useEffect(() => {
    if (!isOpen || !activeTabId) return;
    void ensureIndex(activeTabId);
  }, [isOpen, activeTabId, ensureIndex]);

  // Reset transient state on open and capture the previously-focused element
  // so we can restore focus on close (AC4).
  useEffect(() => {
    if (isOpen) {
      previouslyFocusedRef.current = (document.activeElement as HTMLElement | null) ?? null;
      setInput('');
      setSelectedIndex(0);
      setUnreachable(null);
      setBreadcrumb(null);
      lastInputChangeRef.current = 0;
      // Focus on next microtask so React has mounted the input.
      queueMicrotask(() => {
        inputRef.current?.focus();
      });
    } else {
      // Restore focus to the previously-focused element.
      previouslyFocusedRef.current?.focus();
    }
  }, [isOpen]);

  const parsed = useMemo(() => parseQuery(input), [input]);
  const results: ObjectIndexEntry[] = useMemo(() => {
    if (parsed.kind === 'empty' || parsed.kind === 'invalid') return [];
    if (!index) return [];
    return rankResults(parsed, index);
  }, [parsed, index]);

  // Clamp selectedIndex when result count shrinks.
  useEffect(() => {
    if (selectedIndex >= results.length && results.length > 0) {
      setSelectedIndex(0);
    }
  }, [results.length, selectedIndex]);

  // Fetch breadcrumb for the highlighted row (single-flight; supersedes on
  // selection change). Only for reachable rows.
  useEffect(() => {
    if (!isOpen || !activeTabId) {
      setBreadcrumb(null);
      return;
    }
    if (results.length === 0 || selectedIndex >= results.length) {
      setBreadcrumb(null);
      return;
    }
    const entry = results[selectedIndex];
    if (!entry.reachable || entry.nodeId === '') {
      setBreadcrumb(null);
      return;
    }
    let cancelled = false;
    setBreadcrumb(null);
    (async () => {
      try {
        const path = await GetAncestorPath(activeTabId, entry.nodeId);
        if (cancelled) return;
        if (path && path.length > 1) {
          // Drop the final element (the target itself) and join with arrows.
          setBreadcrumb(path.slice(0, -1).join(' > '));
        }
      } catch {
        // Silent: missing/loading breadcrumb renders as nothing per AC6.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [isOpen, activeTabId, results, selectedIndex]);

  // Highest object number in the index, for the "no object N -- highest is M" message.
  const highestObjNum = useMemo(() => {
    if (!index || index.length === 0) return null;
    let max = 0;
    for (const e of index) if (e.objNum > max) max = e.objNum;
    return max;
  }, [index]);

  /** Commit a single index entry: dispatch nav, record recent, close palette. */
  const commitEntry = useCallback(
    (entry: ObjectIndexEntry) => {
      if (!activeTabId) return;
      if (!entry.reachable || entry.nodeId === '') {
        setUnreachable(entry.objNum);
        return;
      }
      dispatch({
        type: 'PUSH_RECENT_JUMP',
        payload: {
          tabId: activeTabId,
          entry: {
            objNum: entry.objNum,
            gen: entry.gen,
            typeName: entry.typeName,
            nodeId: entry.nodeId,
          },
        },
      });
      dispatch({ type: 'NAVIGATE_TO_REF', payload: { targetNodeId: entry.nodeId } });
      closePalette();
    },
    [activeTabId, dispatch],
  );

  /** Backdrop click handler -- close if click target is the backdrop itself. */
  const onBackdropMouseDown = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    if (e.target === e.currentTarget) {
      closePalette();
    }
  }, []);

  /** Input keydown: Esc, ArrowUp/Down, Enter. */
  const onInputKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        closePalette();
        return;
      }
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        if (results.length > 0) {
          setSelectedIndex((i) => (i + 1) % results.length);
        }
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        if (results.length > 0) {
          setSelectedIndex((i) => (i - 1 + results.length) % results.length);
        }
        return;
      }
      if (e.key === 'Enter') {
        e.preventDefault();
        // 50ms idle gate: if the input changed within the last IDLE_MS,
        // drop the Enter to avoid firing during fast typing.
        const elapsed = Date.now() - lastInputChangeRef.current;
        if (lastInputChangeRef.current > 0 && elapsed < IDLE_MS) {
          return;
        }
        if (results.length === 0) return;
        commitEntry(results[selectedIndex]);
        return;
      }
      // Focus trap (AC4): the input is the only focusable element in the
      // palette, so Tab and Shift+Tab pin focus on it instead of escaping
      // to elements behind the backdrop.
      if (e.key === 'Tab') {
        e.preventDefault();
        return;
      }
    },
    [results, selectedIndex, commitEntry],
  );

  // Auto-dismiss the unreachable notice after 2s (AC8).
  useEffect(() => {
    if (unreachable === null) return;
    const t = setTimeout(() => setUnreachable(null), 2000);
    return () => clearTimeout(t);
  }, [unreachable]);

  if (!isOpen) return null;

  const showRecents = parsed.kind === 'empty';
  const showGrammarHint = showRecents && recentJumps.length === 0;
  // Suppress empty-state messages until the index has loaded; otherwise users
  // who type before the first GetObjectIndex response see a transient
  // "No matches" before real results render.
  const indexLoaded = index !== null;
  const showNumericEmpty = indexLoaded && parsed.kind === 'numeric' && results.length === 0;
  const showTypeEmpty = indexLoaded && parsed.kind === 'type' && results.length === 0;

  return (
    <div
      className="fixed inset-0 bg-black/40 flex items-start justify-center pt-[15vh] z-50"
      onMouseDown={onBackdropMouseDown}
      data-testid="command-palette-backdrop"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Command palette"
        data-testid="command-palette"
        className="bg-surface border border-border rounded-md shadow-xl w-[480px] max-w-[90vw] flex flex-col overflow-hidden"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <input
          ref={inputRef}
          type="text"
          autoComplete="off"
          spellCheck={false}
          placeholder="Object number or /Type filter (e.g. 847, 847 0 R, /Font)"
          data-testid="command-palette-input"
          className="px-3 py-2 text-sm font-ui bg-surface text-text border-b border-border outline-none"
          value={input}
          onChange={(e) => {
            setInput(e.target.value);
            setSelectedIndex(0);
            setUnreachable(null);
            lastInputChangeRef.current = Date.now();
          }}
          onKeyDown={onInputKeyDown}
        />

        {showGrammarHint && (
          <div
            data-testid="command-palette-grammar-hint"
            className="px-3 py-2 text-xs text-text-muted"
          >
            Type an object number (e.g. 847), an N G R, or /Type to filter.
          </div>
        )}

        {showRecents && recentJumps.length > 0 && (
          <div role="listbox" className="max-h-[60vh] overflow-auto">
            {recentJumps.map((r, idx) => (
              <PaletteRow
                key={`recent-${r.nodeId}-${idx}`}
                entry={{
                  objNum: r.objNum,
                  gen: r.gen,
                  typeName: r.typeName,
                  free: false,
                  reachable: true,
                  nodeId: r.nodeId,
                }}
                highlighted={false}
                breadcrumb={null}
                onClick={() =>
                  commitEntry({
                    objNum: r.objNum,
                    gen: r.gen,
                    typeName: r.typeName,
                    free: false,
                    reachable: true,
                    nodeId: r.nodeId,
                  })
                }
                testId="command-palette-recent-row"
              />
            ))}
          </div>
        )}

        {results.length > 0 && (
          <div role="listbox" className="max-h-[60vh] overflow-auto">
            {results.map((entry, idx) => (
              <PaletteRow
                key={`${entry.objNum}-${entry.gen}-${idx}`}
                entry={entry}
                highlighted={idx === selectedIndex}
                breadcrumb={idx === selectedIndex ? breadcrumb : null}
                onClick={() => {
                  setSelectedIndex(idx);
                  commitEntry(entry);
                }}
                testId="command-palette-row"
              />
            ))}
          </div>
        )}

        {showNumericEmpty && parsed.kind === 'numeric' && (
          <div
            data-testid="command-palette-empty"
            className="px-3 py-2 text-xs text-text-muted"
          >
            No object {parsed.objNum}
            {highestObjNum !== null && ` -- highest object in this document is ${highestObjNum}.`}
          </div>
        )}

        {showTypeEmpty && (
          <div
            data-testid="command-palette-empty"
            className="px-3 py-2 text-xs text-text-muted"
          >
            No matches
          </div>
        )}

        {unreachable !== null && (
          <div
            data-testid="command-palette-unreachable-notice"
            className="px-3 py-2 text-xs text-text-muted border-t border-border"
          >
            Object {unreachable} is not reachable from the catalog -- tree navigation is unavailable.
          </div>
        )}
      </div>
    </div>
  );
}
