import { Sidebar } from './Sidebar'
import { TopBar } from './TopBar'

export function AppShell({
  view, onViewChange,
  folderCounts, selectedFolder, onFolderSelect,
  lastUpdated, children, onCompose,
}) {
  return (
    <div className="app-shell">
      <Sidebar
        view={view}
        onViewChange={onViewChange}
        onCompose={onCompose}
        folderCounts={folderCounts}
        selectedFolder={selectedFolder}
        onFolderSelect={onFolderSelect}
      />
      <div className="app-main">
        <TopBar lastUpdated={lastUpdated} />
        <div className="app-content">
          {children}
        </div>
      </div>
    </div>
  )
}
