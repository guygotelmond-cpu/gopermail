const FOLDER_ICONS = {
  INBOX: '📥',
  inbox: '📥',
  spam: '🚫',
  Spam: '🚫',
  trash: '🗑',
  Trash: '🗑',
  sent: '📤',
  Sent: '📤',
}

function folderIcon(name) {
  return FOLDER_ICONS[name] || '📁'
}

export function Sidebar({ view, onViewChange, onCompose, folderCounts, selectedFolder, onFolderSelect }) {
  const inboxCount = folderCounts?.['INBOX'] || folderCounts?.['inbox'] || 0

  const customFolders = Object.entries(folderCounts || {})
    .filter(([name]) => name !== 'INBOX' && name !== 'inbox')
    .sort(([a], [b]) => a.localeCompare(b))

  const inboxActive = view === 'inbox' && (selectedFolder === 'INBOX' || selectedFolder === 'inbox')

  return (
    <aside className="sidebar">
      <div className="sidebar-brand">
        <span className="brand-icon">✉</span>
        <span className="brand-name">GoPerMail</span>
      </div>

      <button className="compose-btn" onClick={onCompose}>
        + Compose
      </button>

      <nav className="sidebar-nav">
        <button
          className={`sidebar-nav-item${inboxActive ? ' active' : ''}`}
          onClick={() => onFolderSelect('INBOX')}
        >
          <span className="nav-icon">📥</span>
          <span className="nav-label">Inbox</span>
          {inboxCount > 0 && <span className="nav-badge">{inboxCount}</span>}
        </button>

        {customFolders.length > 0 && (
          <>
            <div className="sidebar-section-label">Labels</div>
            {customFolders.map(([name, count]) => {
              const isActive = view === 'inbox' && selectedFolder === name
              return (
                <button
                  key={name}
                  className={`sidebar-nav-item${isActive ? ' active' : ''}`}
                  onClick={() => onFolderSelect(name)}
                >
                  <span className="nav-icon">{folderIcon(name)}</span>
                  <span className="nav-label">{name}</span>
                  {count > 0 && <span className="nav-badge">{count}</span>}
                </button>
              )
            })}
          </>
        )}

        <div className="sidebar-divider" />

        <button
          className={`sidebar-nav-item${view === 'rules' ? ' active' : ''}`}
          onClick={() => onViewChange('rules')}
        >
          <span className="nav-icon">🔀</span>
          <span className="nav-label">Filter Rules</span>
        </button>

        <button
          className={`sidebar-nav-item${view === 'settings' ? ' active' : ''}`}
          onClick={() => onViewChange('settings')}
        >
          <span className="nav-icon">⚙️</span>
          <span className="nav-label">Settings</span>
        </button>
      </nav>
    </aside>
  )
}
