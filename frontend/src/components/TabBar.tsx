/**
 * @file Tab bar for multi-document management.
 * Renders one tab per open PDF using Radix UI Tabs for ARIA semantics.
 * Reads state from context -- no props needed.
 */
import { useEffect, useCallback } from 'react';
import * as Tabs from '@radix-ui/react-tabs';
import { Events } from '@wailsio/runtime';
import { useAppState, useAppDispatch, type TabState } from '../hooks/useDocumentState';
import { closeDocumentTab } from '../lib/openedDocuments';
import { useDiskStatus, type DiskStatusEntry } from '../lib/diskStatusStore';
import { formatTimestamp } from '../lib/formatTimestamp';
import { FILE_MISSING_MESSAGE } from '../lib/fileChange';
import { getShortcutHint } from '../lib/platform';

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
      closeDocumentTab(dispatch, tabId);
    },
    [dispatch],
  );

  // Cycle to adjacent tab (wrapping). direction: 1 = next, -1 = prev.
  const cycleTab = useCallback(
    (direction: 1 | -1) => {
      if (tabs.length < 2 || !activeTabId) return;
      const idx = tabs.findIndex((t) => t.tabId === activeTabId);
      if (idx === -1) return;
      const nextIdx = (idx + direction + tabs.length) % tabs.length;
      dispatch({ type: 'ACTIVATE_TAB', payload: { tabId: tabs[nextIdx].tabId } });
    },
    [tabs, activeTabId, dispatch],
  );

  // Close active document (shared by keyboard shortcut and native menu)
  const closeActiveTab = useCallback(() => {
    if (!activeTabId) return;
    const tabExists = tabs.some((t) => t.tabId === activeTabId);
    if (!tabExists) return;
    closeDocumentTab(dispatch, activeTabId);
  }, [tabs, activeTabId, dispatch]);

  // Keyboard shortcuts: Cmd/Ctrl+Arrow (tab switch), Cmd/Ctrl+W (close)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      if (!mod) return;

      // Cmd/Ctrl+Right -- next tab
      if (e.key === 'ArrowRight') {
        e.preventDefault();
        cycleTab(1);
        return;
      }

      // Cmd/Ctrl+Left -- previous tab
      if (e.key === 'ArrowLeft') {
        e.preventDefault();
        cycleTab(-1);
        return;
      }

      // Cmd+W (macOS) or Ctrl+W (Windows/Linux) -- close document
      if (e.key === 'w' || e.key === 'W') {
        e.preventDefault();
        closeActiveTab();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [cycleTab, closeActiveTab]);

  // Native menu: Navigate > Next Tab / Previous Tab, File > Close Document
  useEffect(() => {
    const offNext = Events.On('tab:next', () => cycleTab(1));
    const offPrev = Events.On('tab:prev', () => cycleTab(-1));
    const offClose = Events.On('document:close-active', () => closeActiveTab());
    return () => { offNext(); offPrev(); offClose(); };
  }, [cycleTab, closeActiveTab]);

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
          <TabItem key={tab.tabId} tab={tab} onClose={handleClose} />
        ))}
      </Tabs.List>
    </Tabs.Root>
  );
}

/** Second tooltip line for a tab whose file changed or went missing on disk. */
function diskStatusLine(status: DiskStatusEntry): string {
  if (status.state === 'missing') return FILE_MISSING_MESSAGE;
  return `Document updated on disk ${formatTimestamp(status.diskModTime)} - press ${getShortcutHint('R')} to reload`;
}

interface TabItemProps {
  tab: TabState;
  onClose: (tabId: string, e: React.MouseEvent) => void;
}

/**
 * One tab trigger. Subscribes to its own tab's disk status only, so a status
 * update re-renders this tab and nothing else. The dot is an indicator: it is
 * aria-hidden, has no handler of its own and sits in a fixed slot that is
 * reserved whether or not it shows, so tab widths never change.
 */
function TabItem({ tab, onClose }: TabItemProps) {
  const status = useDiskStatus(tab.tabId);
  const path = tab.filePath || tab.fileName;
  const title = status ? `${path}\n${diskStatusLine(status)}` : path;
  const srText = status ? (status.state === 'missing' ? ', file not found' : ', changed on disk') : '';
  return (
    <Tabs.Trigger value={tab.tabId} asChild>
      <div
        className="group flex items-center px-3 py-1.5 text-sm border-r border-border truncate max-w-[200px] bg-surface-hover text-text-secondary hover:bg-tab-hover hover:text-text data-[state=inactive]:hover:border-r-tab-hover-border cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-border-focus data-[state=active]:bg-bg data-[state=active]:text-text data-[state=active]:border-b-2 data-[state=active]:border-b-border-focus"
        data-testid={`tab-${tab.tabId}`}
        title={title}
      >
        <span className="truncate">{tab.fileName}</span>
        {srText && <span className="sr-only">{srText}</span>}
        <span
          className="ml-1.5 flex h-2 w-2 flex-shrink-0 items-center justify-center"
          data-testid={`tab-disk-slot-${tab.tabId}`}
          aria-hidden="true"
        >
          {status && (
            <span
              className="disk-dot-in block h-1.5 w-1.5 rounded-full bg-warning ring-1 ring-text-secondary"
              data-testid={`tab-disk-dot-${tab.tabId}`}
              aria-hidden="true"
            />
          )}
        </span>
        <button
          type="button"
          className="ml-2 opacity-0 group-hover:opacity-100 rounded-sm hover:bg-surface-hover text-text-muted hover:text-text w-4 h-4 flex items-center justify-center flex-shrink-0 cursor-pointer"
          data-testid={`tab-close-${tab.tabId}`}
          aria-label={`Close ${tab.fileName}`}
          onClick={(e) => onClose(tab.tabId, e)}
        >
          <svg width="12" height="12" viewBox="0 0 12 12" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M2.5 2.5L9.5 9.5M9.5 2.5L2.5 9.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
        </button>
      </div>
    </Tabs.Trigger>
  );
}
