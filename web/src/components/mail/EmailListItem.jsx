const AVATAR_COLORS = [
  '#2563eb', '#16a34a', '#dc2626', '#9333ea',
  '#ea580c', '#0891b2', '#be185d', '#65a30d',
]

function avatarColor(str) {
  let hash = 0
  for (let i = 0; i < str.length; i++) hash = str.charCodeAt(i) + ((hash << 5) - hash)
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length]
}

function formatDate(iso) {
  const d = new Date(iso)
  const now = new Date()
  const isToday = d.toDateString() === now.toDateString()
  return isToday
    ? d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    : d.toLocaleDateString([], { month: 'short', day: 'numeric' })
}

function senderName(sender) {
  // Extract display name from "Name <email>" or use the raw string
  const match = sender?.match(/^([^<]+)</)
  return match ? match[1].trim() : (sender || 'Unknown')
}

function senderInitial(sender) {
  return senderName(sender).replace(/[^a-zA-Z0-9]/g, '').charAt(0).toUpperCase() || '?'
}

export function EmailListItem({ email, selected, onClick }) {
  const initial = senderInitial(email.sender)
  const color = avatarColor(email.sender || '')
  const name = senderName(email.sender)
  const subject = email.subject || '(no subject)'
  const preview = email.preview || ''

  return (
    <div
      className={`email-list-item${selected ? ' selected' : ''}`}
      onClick={onClick}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === 'Enter' && onClick()}
    >
      <div className="email-avatar" style={{ background: color }}>
        {initial}
      </div>
      <div className="email-item-body">
        <div className="email-item-top">
          <span className="email-item-sender">{name}</span>
          <span className="email-item-date">{formatDate(email.received_at)}</span>
        </div>
        <div className="email-item-subject">{subject}</div>
        {preview && <div className="email-item-preview">{preview}</div>}
      </div>
    </div>
  )
}
