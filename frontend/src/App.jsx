/**
 * @file Root application component. Wires Wails runtime events to app state
 * and renders either the empty state or the main three-panel layout.
 */
import { useEffect, useRef } from 'react'
import { Events, Screens, Window } from '@wailsio/runtime'
import { CloseDocument, ConsumePendingOpenFiles } from '../bindings/unidoc-pdf-debugger/internal/pdfservice/pdfservice.js'
import { AppProvider, useAppState, useAppDispatch } from './hooks/useDocumentState'
import { mapErrorMessage, openPDFFile } from './hooks/usePDFService'
import { useWindowPersistence } from './hooks/useWindowPersistence'
import { computeRestorePlan } from './lib/windowGeometryGuard'
import { getPlatformModifier } from './lib/platform'
import { EmptyState } from './components/EmptyState'
import { MainLayout } from './components/MainLayout'
import { ErrorBanner } from './components/ErrorBanner'
import { TabBar } from './components/TabBar'
import { GoToPageDialog } from './components/GoToPageDialog'
import { BatchOpenDialog } from './components/BatchOpenDialog'
import { CommandPalette } from './components/CommandPalette/CommandPalette'
import { openPalette, useCommandPalette } from './hooks/useCommandPalette'

/**
 * Module-level set of file paths the frontend has opened in this JS session.
 * The cold-start drain consults this so a drained path that is
 * already open frees its newly-created backend tab instead of leaking it.
 * tabsRef alone cannot cover this because a dev-mode reload mounts a fresh
 * reducer (empty tabs) while the previous session's documents are still open;
 * the per-instance ref does not see them. This survives a re-mount within the
 * same JS context (the reload case the test pins) but is cleared by a true page
 * reload (new context) -- where drain-on-read already returns an empty drain,
 * so the set is never consulted with content in that path.
 */
const sessionOpenPaths = new Set()

/**
 * Inner shell that subscribes to Wails backend events and delegates
 * document open/error handling to the app reducer.
 */
function AppContent() {
  const { tabs, activeTabId, documentError, documentWarning } = useAppState()
  const dispatch = useAppDispatch()
  // Mount the Cmd+K global keyboard listener exactly once at App level.
  useCommandPalette()
  const hasDocument = activeTabId !== null
  const activeTab = tabs.find((t) => t.tabId === activeTabId)
  const navHistory = activeTab?.navHistory ?? []
  const navHistoryIndex = activeTab?.navHistoryIndex ?? -1
  const canGoBack = navHistoryIndex > 0
  const canGoForward = navHistoryIndex < navHistory.length - 1

  // Window geometry persistence: load on mount, save on move/resize.
  const { windowGeometry, saveWindowGeometry } = useWindowPersistence()
  // Suppresses the restore-feedback loop: SetSize/SetPosition trigger the same
  // events we listen for, which would re-save the just-restored values.
  const restoreInProgressRef = useRef(false)
  const windowGeometryOnMountRef = useRef(windowGeometry)

  // Ref mirrors tabs for use inside event handler closures without stale captures
  const tabsRef = useRef(tabs)
  useEffect(() => { tabsRef.current = tabs }, [tabs])

  // Tracks the tabId from the most recent OPEN_DOCUMENT dispatch so the
  // post-dispatch fallback can detect when dedup fired in the reducer.
  const lastOpenedTabIdRef = useRef(null)

  // Sync navigation menu enabled state with backend
  const prevNavState = useRef({ canGoBack: false, canGoForward: false })
  useEffect(() => {
    if (prevNavState.current.canGoBack !== canGoBack || prevNavState.current.canGoForward !== canGoForward) {
      prevNavState.current = { canGoBack, canGoForward }
      Events.Emit('navigate:state-changed', { canGoBack, canGoForward })
    }
  }, [canGoBack, canGoForward])

  // Post-dispatch fallback: if reducer dedup fired, activeTabId won't match
  // lastOpenedTabIdRef. Free the orphaned backend state only if the tabId
  // was actually rejected (not present in any tab). This prevents a race
  // where ACTIVATE_TAB changes activeTabId between dispatch and effect.
  useEffect(() => {
    const pendingTabId = lastOpenedTabIdRef.current
    if (!pendingTabId) return
    lastOpenedTabIdRef.current = null
    const wasAdded = tabsRef.current.some((t) => t.tabId === pendingTabId)
    if (!wasAdded) {
      Promise.resolve(CloseDocument(pendingTabId)).catch(() => {})
    }
  }, [activeTabId])

  // Subscribe to Wails runtime events for backend-initiated document opens
  // and errors. Returns cleanup functions to unsubscribe on unmount.
  useEffect(() => {
    const offOpened = Events.On('document:opened', (event) => {
      const data = event?.data
      if (!data || !data.tabId || !data.fileName) return

      // Pre-dispatch dedup check: if a tab with the same filePath exists,
      // free the backend state for the new tabId before dispatching.
      // When dedup fires here, skip setting lastOpenedTabIdRef so the
      // post-dispatch fallback doesn't double-close the same tabId.
      const filePath = data.filePath ?? ''
      let dedupHandled = false
      if (filePath) {
        const existing = tabsRef.current.find((t) => t.filePath === filePath)
        if (existing) {
          Promise.resolve(CloseDocument(data.tabId)).catch(() => {})
          dedupHandled = true
        }
      }

      if (!dedupHandled) {
        lastOpenedTabIdRef.current = data.tabId
      }
      // Record the open so a later cold-start drain of the same path can
      // detect it (Story 12.1 cross-session dedup).
      if (filePath) sessionOpenPaths.add(filePath)
      dispatch({
        type: 'OPEN_DOCUMENT',
        payload: {
          tabId: data.tabId,
          fileName: data.fileName,
          filePath: filePath,
          pageCount: data.pageCount ?? 0,
          rootNode: data.rootNode ?? null,
          rootChildren: data.rootChildren ?? null,
        },
      })
      if (data.warning) {
        dispatch({ type: 'SET_DOCUMENT_WARNING', payload: { message: data.warning } })
      }
    })

    const offError = Events.On('document:error', (event) => {
      const data = event?.data
      const msg = (data && data.message) || 'An unknown error occurred.'
      dispatch({ type: 'SET_DOCUMENT_ERROR', payload: { message: mapErrorMessage(msg) } })
    })

    const offWarning = Events.On('document:warning', (event) => {
      const data = event?.data
      const msg = (data && data.message) || ''
      if (msg) dispatch({ type: 'SET_DOCUMENT_WARNING', payload: { message: msg } })
    })

    const offNavBack = Events.On('navigate:back', () => {
      dispatch({ type: 'NAVIGATE_BACK' })
    })

    const offNavForward = Events.On('navigate:forward', () => {
      dispatch({ type: 'NAVIGATE_FORWARD' })
    })

    const offGoToPage = Events.On('navigate:goToPage', () => {
      dispatch({ type: 'OPEN_GO_TO_PAGE' })
    })

    const offPaletteOpen = Events.On('palette:open', () => {
      openPalette()
    })

    const offLoadStart = Events.On('document:load-start', (event) => {
      const data = event?.data
      const fileName = (data && data.fileName) || ''
      if (fileName) {
        dispatch({ type: 'OPENING_START', payload: { fileName } })
      }
    })

    const offBatchStart = Events.On('document:batch-start', (event) => {
      const total = Number(event?.data?.total) || 0
      if (total > 0) dispatch({ type: 'BATCH_OPEN_START', payload: { total } })
    })

    const offBatchComplete = Events.On('document:batch-complete', () => {
      dispatch({ type: 'BATCH_OPEN_COMPLETE' })
    })

    // cold-start drain. STRICTLY AFTER the document:opened listener
    // above is registered, drain any file-association paths the backend
    // buffered before the frontend was ready (cold start). Ordering is
    // load-bearing: a path delivered between drain and subscribe would be lost,
    // so this MUST run after Events.On('document:opened', ...). Drain-on-read
    // (the backend queue stays empty after the first drain) makes StrictMode
    // double-effects and dev reloads safe -- a second drain returns [].
    async function drainPendingOpens() {
      // The binding can resolve to null (Go nil slice -> JSON null when the
      // queue is unwired or empty); treat that as an empty list. Filter to
      // non-empty strings as well: the same marshalling-boundary defense as the
      // null guard, so a null/empty/non-string element cannot make the
      // paths[0].replace basename derivation below throw outside the per-path
      // try block (which would abandon the whole drain as an unhandled
      // rejection). openPDFFile already rejects empty paths inside the loop.
      const raw = (await ConsumePendingOpenFiles()) ?? []
      const paths = raw.filter((p) => typeof p === 'string' && p.length > 0)
      if (paths.length === 0) return
      // Mirror the single-file open UX: surface the loading state for the
      // first path's basename before the openPDFFile awaits block.
      dispatch({ type: 'OPENING_START', payload: { fileName: paths[0].replace(/^.*[/\\]/, '') } })
      // Defer the error to after the loop: OPEN_DOCUMENT clears documentError,
      // so dispatching a per-path error inline would be wiped by a later
      // successful open in the same drain. Surfacing the last error after all
      // opens keeps a failed path visible without blocking the others.
      let lastError = null
      for (const path of paths) {
        try {
          const result = await openPDFFile(path)
          // Pre-dispatch dedup (mirrors the document:opened listener): if this
          // filePath is already open -- either in this reducer's tabs or, after
          // a dev-mode reload, in a still-live previous session (sessionOpenPaths)
          // -- free the new backend tab and still dispatch. The
          // lastOpenedTabIdRef orphan-close fallback does NOT cover drain-path
          // opens, so without this a drained duplicate leaks backend state.
          const alreadyOpen = result.filePath
            && (tabsRef.current.some((t) => t.filePath === result.filePath)
              || sessionOpenPaths.has(result.filePath))
          if (alreadyOpen) {
            Promise.resolve(CloseDocument(result.tabId)).catch(() => {})
          }
          if (result.filePath) sessionOpenPaths.add(result.filePath)
          dispatch({
            type: 'OPEN_DOCUMENT',
            payload: {
              tabId: result.tabId,
              fileName: result.fileName,
              filePath: result.filePath,
              pageCount: result.pageCount,
              rootNode: result.rootNode,
              rootChildren: result.rootChildren,
            },
          })
        } catch (err) {
          // Keep iterating so one bad file does not block the rest; defer the
          // error dispatch until after the loop (see lastError above).
          const msg = err instanceof Error ? err.message : String(err)
          lastError = mapErrorMessage(msg)
        }
      }
      if (lastError !== null) {
        dispatch({ type: 'SET_DOCUMENT_ERROR', payload: { message: lastError } })
      }
    }
    void drainPendingOpens()

    return () => {
      offOpened()
      offError()
      offWarning()
      offNavBack()
      offNavForward()
      offGoToPage()
      offPaletteOpen()
      offLoadStart()
      offBatchStart()
      offBatchComplete()
    }
  }, [dispatch])

  // Restore window geometry on startup. Apply size first, then position --
  // on Windows, the reverse order can transiently push the window off-screen.
  // Off-screen guard skips position restore (but not size) when the persisted
  // rectangle does not intersect any currently-connected display's WorkArea.
  useEffect(() => {
    const stored = windowGeometryOnMountRef.current
    if (!stored) return
    /** @type {{ x: number, y: number, width: number, height: number }} */
    const geometry = stored

    // Set the flag synchronously here (not inside the async restore body) to
    // close the microtask-sized gap between this effect and the listener
    // effect that registers the move/resize handlers. Otherwise an echo
    // event arriving before the first await could slip through with the
    // flag still false.
    restoreInProgressRef.current = true

    let cancelled = false
    /** @type {ReturnType<typeof setTimeout> | null} */
    let restoreFlagTimer = null

    async function restore() {
      try {
        // Optional clamp + off-screen guard: query screens for WorkArea info.
        let screens = null
        try {
          screens = await Screens.GetAll()
        } catch {
          screens = null
        }
        if (cancelled) return

        const fallback =
          typeof window !== 'undefined' && window.screen
            ? { availWidth: window.screen.availWidth, availHeight: window.screen.availHeight }
            : null
        const { width, height, positionAllowed } = computeRestorePlan(geometry, screens, fallback)

        await Window.SetSize(width, height)
        if (cancelled) return
        if (positionAllowed) {
          await Window.SetPosition(geometry.x, geometry.y)
        }
      } catch {
        // Wails runtime not ready or platform error -- fall back to defaults.
      } finally {
        // If unmounted mid-restore, the cleanup function has already cleared
        // the flag and is no longer tracking timers. Skip scheduling a new
        // timeout (would leak past unmount). Otherwise schedule a 750ms
        // delay so echo events from SetSize/SetPosition do not re-save.
        if (!cancelled) {
          restoreFlagTimer = setTimeout(() => {
            restoreFlagTimer = null
            restoreInProgressRef.current = false
          }, 750)
        }
      }
    }

    restore()

    return () => {
      cancelled = true
      if (restoreFlagTimer != null) {
        clearTimeout(restoreFlagTimer)
        restoreFlagTimer = null
      }
      // If unmount happens before the flag-clear timer was scheduled (i.e.
      // mid-await), force the flag off so a remount sees a clean slate.
      restoreInProgressRef.current = false
    }
  }, [])

  // Subscribe to OS window-move / window-resize events. Each event reads
  // the current geometry via the JS runtime and forwards to the debounced
  // saver. Echoes from the startup restore are suppressed via the flag.
  useEffect(() => {
    let unmounted = false

    async function captureAndSave() {
      if (restoreInProgressRef.current) return
      try {
        const [pos, size] = await Promise.all([Window.Position(), Window.Size()])
        // The promise above may resolve after unmount; if so, drop the
        // result so we don't schedule a write through a torn-down hook.
        if (unmounted) return
        if (
          pos &&
          size &&
          Number.isFinite(pos.x) &&
          Number.isFinite(pos.y) &&
          Number.isFinite(size.width) &&
          Number.isFinite(size.height) &&
          size.width > 0 &&
          size.height > 0
        ) {
          saveWindowGeometry({ x: pos.x, y: pos.y, width: size.width, height: size.height })
        }
      } catch {
        // Runtime unavailable -- skip this event.
      }
    }

    const offMove = Events.On('common:WindowDidMove', captureAndSave)
    const offResize = Events.On('common:WindowDidResize', captureAndSave)

    return () => {
      unmounted = true
      offMove()
      offResize()
    }
  }, [saveWindowGeometry])

  // Cmd+G (macOS) / Ctrl+G (Win/Linux) opens the Go to Page dialog. Skip
  // when focus is in a text input/area so the shortcut never steals typing,
  // and skip when no document is loaded (the reducer is also a no-op).
  // The native menu item in main.go also emits navigate:goToPage; this
  // listener exists so the shortcut works even before the menu is opened.
  useEffect(() => {
    /** @param {EventTarget | null} target */
    function isInTextField(target) {
      if (!(target instanceof HTMLElement)) return false
      const tag = target.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA') return true
      if (target.isContentEditable) return true
      return false
    }
    const wantsMeta = getPlatformModifier() === 'Cmd'
    /** @param {KeyboardEvent} e */
    function handler(e) {
      const mod = wantsMeta ? e.metaKey : e.ctrlKey
      if (!mod) return
      if (e.key !== 'g' && e.key !== 'G') return
      if (isInTextField(e.target)) return
      if (!hasDocument) return
      e.preventDefault()
      dispatch({ type: 'OPEN_GO_TO_PAGE' })
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [hasDocument, dispatch])

  return (
    <div className="flex flex-col h-full" data-file-drop-target>
      {documentError && (
        <ErrorBanner
          message={documentError}
          severity="error"
          onDismiss={() => dispatch({ type: 'DISMISS_ERROR' })}
        />
      )}
      {documentWarning && (
        <ErrorBanner
          message={documentWarning}
          severity="warning"
          onDismiss={() => dispatch({ type: 'DISMISS_WARNING' })}
        />
      )}
      {tabs.length > 0 && <TabBar />}
      <div className="flex-1 min-h-0">
        {hasDocument ? <MainLayout /> : <EmptyState />}
      </div>
      <GoToPageDialog />
      <BatchOpenDialog />
      <CommandPalette />
    </div>
  )
}

/**
 * Top-level App component. Wraps the content in AppProvider for global state.
 * @returns {JSX.Element}
 */
function App() {
  return (
    <AppProvider>
      <AppContent />
    </AppProvider>
  )
}

export default App
