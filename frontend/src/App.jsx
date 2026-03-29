import { AppProvider, useAppState } from './hooks/useDocumentState'
import { EmptyState } from './components/EmptyState'
import { MainLayout } from './components/MainLayout'

function AppContent() {
  const { activeTabId } = useAppState()
  const hasDocument = activeTabId !== null

  if (hasDocument) {
    return <MainLayout />
  }
  return <EmptyState />
}

function App() {
  return (
    <AppProvider>
      <AppContent />
    </AppProvider>
  )
}

export default App
