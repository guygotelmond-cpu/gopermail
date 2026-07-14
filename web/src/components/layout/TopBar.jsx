import { useAuth } from '../../hooks/useAuth'

export function TopBar({ lastUpdated }) {
  const { username, logout } = useAuth()

  return (
    <header className="topbar">
      <div className="topbar-left">
        {lastUpdated && (
          <span className="topbar-updated">
            Updated {lastUpdated.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
          </span>
        )}
      </div>
      <div className="topbar-right">
        <div className="user-badge">
          <span className="user-avatar">{(username || '?').charAt(0).toUpperCase()}</span>
          <span className="user-name">{username}</span>
        </div>
        <button className="btn-link" onClick={logout}>Sign out</button>
      </div>
    </header>
  )
}
