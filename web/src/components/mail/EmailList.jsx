import { EmailListItem } from './EmailListItem'

export function EmailList({ emails, selectedId, onSelect }) {
  if (!emails.length) {
    return (
      <div className="email-list-empty">
        <div className="empty-icon">📭</div>
        <p>No messages yet</p>
        <span>Emails sent to your address will appear here</span>
      </div>
    )
  }

  return (
    <div className="email-list">
      {emails.map((email) => (
        <EmailListItem
          key={email.id}
          email={email}
          selected={email.id === selectedId}
          onClick={() => onSelect(email.id)}
        />
      ))}
    </div>
  )
}
