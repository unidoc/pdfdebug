/**
 * @file Tab bar for multi-document management.
 * Renders one tab per open PDF using Radix UI Tabs for ARIA semantics.
 * Reads state from context -- no props needed.
 */
import { useEffect, useCallback } from 'react';
import * as Tabs from '@radix-ui/react-tabs';
import { useAppState, useAppDispatch } from '../hooks/useDocumentState';
import { CloseDocument } from '../../bindings/unipdf-debugger/internal/pdfservice/pdfservice.js';

/** Tab bar displaying all open documents with switch and close controls. */
export function TabBar() {
  const { tabs, activeTabId } = useAppState();
  const dispatch = useAppDispatch();

  const handleTabChange = useCallback(
    (value: string) => {
      dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: value } });
    },
    [dispatch],
  );

  const handleClose = useCallback(
    (tabId: string, e: React.MouseEvent) => {
      e.stopPropagation();
      dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId } });
      Promise.resolve(CloseDocument(tabId)).catch(() => {});
    },
    [dispatch],
  );

  // Keyboard shortcuts: Cmd/Ctrl+Tab, Cmd/Ctrl+Shift+Tab, Cmd/Ctrl+W
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      if (!mod) return;

      if (e.key === 'Tab') {
        e.preventDefault();
        if (tabs.length < 2 || !activeTabId) return;
        const idx = tabs.findIndex((t) => t.tabId === activeTabId);
        if (idx === -1) return;
        const nextIdx = e.shiftKey
          ? (idx - 1 + tabs.length) % tabs.length
          : (idx + 1) % tabs.length;
        dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: tabs[nextIdx].tabId } });
      }

      if (e.key === 'w' || e.key === 'W') {
        e.preventDefault();
        if (!activeTabId) return;
        dispatch({ type: 'CLOSE_DOCUMENT', payload: { tabId: activeTabId } });
        Promise.resolve(CloseDocument(activeTabId)).catch(() => {});
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [tabs, activeTabId, dispatch]);

  return (
    <Tabs.Root
      value={activeTabId ?? ''}
      onValueChange={handleTabChange}
      data-testid="tab-bar"
    >
      <Tabs.List
        className="flex border-b border-border bg-surface overflow-x-auto h-9"
        data-testid="tab-list"
      >
        {tabs.map((tab) => (
          <Tabs.Trigger
            key={tab.tabId}
            value={tab.tabId}
            className="group flex items-center px-3 py-1.5 text-sm border-r border-border truncate max-w-[200px] bg-surface-hover text-text-secondary hover:bg-surface-selected data-[state=active]:bg-bg data-[state=active]:text-text data-[state=active]:border-b-2 data-[state=active]:border-b-border-focus"
            data-testid={`tab-${tab.tabId}`}
            title={tab.filePath || tab.fileName}
          >
            <span className="truncate">{tab.fileName}</span>
            <button
              type="button"
              className="ml-2 opacity-0 group-hover:opacity-100 rounded-sm hover:bg-surface-hover text-text-muted hover:text-text w-4 h-4 flex items-center justify-center flex-shrink-0"
              data-testid={`tab-close-${tab.tabId}`}
              aria-label={`Close ${tab.fileName}`}
              onClick={(e) => handleClose(tab.tabId, e)}
            >
              <svg width="12" height="12" viewBox="0 0 12 12" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M2.5 2.5L9.5 9.5M9.5 2.5L2.5 9.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
              </svg>
            </button>
          </Tabs.Trigger>
        ))}
      </Tabs.List>
    </Tabs.Root>
  );
}
