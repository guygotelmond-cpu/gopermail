import { useState, useMemo, useEffect } from 'react'
import { AuthProvider } from './context/AuthContext'
import { useAuth } from './hooks/useAuth'
import { useEmails } from './hooks/useEmails'
import { AuthPage } from './components/auth/AuthPage'
import { AppShell } from './components/layout/AppShell'
import { InboxPage } from './pages/InboxPage'
import { RulesPage } from './pages/RulesPage'
import { SettingsPage } from './pages/SettingsPage'
import { ComposeModal } from './components/compose/ComposeModal'

const THEME_KEY = 'gm_theme'

// Renders only after the user is authenticated so useEmails mounts with a
// token already in localStorage — avoids the "missing auth header" error on
// the initial fetch that fires before login completes.
function AuthenticatedApp() {
  const { emails, loading, error, lastUpdated, refresh } = useEmails()
  const [view, setView] = useState('inbox')
  const [selectedFolder, setSelectedFolder] = useState('INBOX')
  const [composing, setComposing] = useState(false)
  const [theme, setTheme] = useState(() => localStorage.getItem(THEME_KEY) || 'light')

  useEffect(() => {
    document.documentElement.classList.toggle('dark', theme === 'dark')
    localStorage.setItem(THEME_KEY, theme)
  }, [theme])

  const folderCounts = useMemo(() => {
    return emails.reduce((acc, email) => {
      const f = email.folder || 'INBOX'
      acc[f] = (acc[f] || 0) + 1
      return acc
    }, {})
  }, [emails])

  const handleFolderSelect = (folder) => {
    setSelectedFolder(folder)
    setView('inbox')
  }

  return (
    <AppShell
      view={view}
      onViewChange={setView}
      folderCounts={folderCounts}
      selectedFolder={selectedFolder}
      onFolderSelect={handleFolderSelect}
      lastUpdated={lastUpdated}
      onCompose={() => setComposing(true)}
    >
      {view === 'inbox' && (
        <InboxPage
          emails={emails}
          selectedFolder={selectedFolder}
          loading={loading}
          error={error}
        />
      )}
      {view === 'rules' && <RulesPage />}
      {view === 'settings' && (
        <SettingsPage theme={theme} onThemeChange={setTheme} />
      )}

      {composing && (
        <ComposeModal
          onClose={() => setComposing(false)}
          onSent={() => { refresh(); setComposing(false) }}
        />
      )}
    </AppShell>
  )
}

function MailApp() {
  const { username } = useAuth()
  if (!username) return <AuthPage />
  return <AuthenticatedApp />
}

export default function App() {
  return (
    <AuthProvider>
      <MailApp />
    </AuthProvider>
  )
}
