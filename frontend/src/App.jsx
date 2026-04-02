import { useEffect } from 'react'
import { Events } from '@wailsio/runtime'
import { AppProvider, useAppState, useAppDispatch } from './hooks/useDocumentState'
import { mapErrorMessage } from './hooks/usePDFService'
import { EmptyState } from './components/EmptyState'
import { MainLayout } from './components/MainLayout'
import { ErrorBanner } from './components/ErrorBanner'

function AppContent() {
  const { tabs, activeTabId, documentError } = useAppState()
  const dispatch = useAppDispatch()
  const hasDocument = activeTabId !== null
  const activeTab = tabs.find((t) => t.tabId === activeTabId)

  useEffect(() => {
    const offOpened = Events.On('document:opened', (event) => {
      const data = event?.data
      if (!data || !data.tabId || !data.fileName) return
      dispatch({
        type: 'OPEN_DOCUMENT',
        payload: {
          tabId: data.tabId,
          fileName: data.fileName,
          rootNode: data.rootNode ?? null,
          rootChildren: data.rootChildren ?? null,
        },
      })
    })

    const offError = Events.On('document:error', (event) => {
      const data = event?.data
      const msg = (data && data.message) || 'An unknown error occurred.'
      dispatch({ type: 'SET_DOCUMENT_ERROR', payload: { message: mapErrorMessage(msg) } })
    })

    return () => {
      offOpened()
      offError()
    }
  }, [dispatch])

  return (
    <div className="flex flex-col h-full">
      {documentError && (
        <ErrorBanner
          message={documentError}
          severity="error"
          onDismiss={() => dispatch({ type: 'DISMISS_ERROR' })}
        />
      )}
      {activeTab && (
        <div className="px-3 py-1.5 text-sm text-text-secondary border-b border-border truncate" data-testid="document-header">
          {activeTab.fileName}
        </div>
      )}
      <div className="flex-1 min-h-0">
        {hasDocument ? <MainLayout /> : <EmptyState />}
      </div>
    </div>
  )
}

function App() {
  return (
    <AppProvider>
      <AppContent />
    </AppProvider>
  )
}

export default App
