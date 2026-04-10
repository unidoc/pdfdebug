/**
 * @file Root application component. Wires Wails runtime events to app state
 * and renders either the empty state or the main three-panel layout.
 */
import { useEffect, useRef } from 'react'
import { Events } from '@wailsio/runtime'
import { AppProvider, useAppState, useAppDispatch } from './hooks/useDocumentState'
import { mapErrorMessage } from './hooks/usePDFService'
import { EmptyState } from './components/EmptyState'
import { MainLayout } from './components/MainLayout'
import { ErrorBanner } from './components/ErrorBanner'
import { TabBar } from './components/TabBar'

/**
 * Inner shell that subscribes to Wails backend events and delegates
 * document open/error handling to the app reducer.
 */
function AppContent() {
  const { tabs, activeTabId, documentError, documentWarning } = useAppState()
  const dispatch = useAppDispatch()
  const hasDocument = activeTabId !== null
  const activeTab = tabs.find((t) => t.tabId === activeTabId)
  const navHistory = activeTab?.navHistory ?? []
  const navHistoryIndex = activeTab?.navHistoryIndex ?? -1
  const canGoBack = navHistoryIndex > 0
  const canGoForward = navHistoryIndex < navHistory.length - 1

  // Sync navigation menu enabled state with backend
  const prevNavState = useRef({ canGoBack: false, canGoForward: false })
  useEffect(() => {
    if (prevNavState.current.canGoBack !== canGoBack || prevNavState.current.canGoForward !== canGoForward) {
      prevNavState.current = { canGoBack, canGoForward }
      Events.Emit('navigate:state-changed', { canGoBack, canGoForward })
    }
  }, [canGoBack, canGoForward])

  // Subscribe to Wails runtime events for backend-initiated document opens
  // and errors. Returns cleanup functions to unsubscribe on unmount.
  useEffect(() => {
    const offOpened = Events.On('document:opened', (event) => {
      const data = event?.data
      if (!data || !data.tabId || !data.fileName) return
      dispatch({
        type: 'OPEN_DOCUMENT',
        payload: {
          tabId: data.tabId,
          fileName: data.fileName,
          filePath: data.filePath ?? '',
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

    const offNavBack = Events.On('navigate:back', () => {
      dispatch({ type: 'NAVIGATE_BACK' })
    })

    const offNavForward = Events.On('navigate:forward', () => {
      dispatch({ type: 'NAVIGATE_FORWARD' })
    })

    return () => {
      offOpened()
      offError()
      offNavBack()
      offNavForward()
    }
  }, [dispatch])

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
