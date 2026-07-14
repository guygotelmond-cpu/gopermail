import { useState, useEffect, useMemo } from 'react'
import { EmailList } from '../components/mail/EmailList'
import { EmailReader } from '../components/mail/EmailReader'

export function InboxPage({ emails: allEmails = [], selectedFolder, loading, error }) {
  const [selectedId, setSelectedId] = useState(null)

  useEffect(() => { setSelectedId(null) }, [selectedFolder])

  const emails = useMemo(() => {
    return allEmails.filter(e => {
      const folder = e.folder || 'INBOX'
      if (selectedFolder === 'INBOX') return folder === 'INBOX' || folder === 'inbox'
      return folder === selectedFolder
    })
  }, [allEmails, selectedFolder])

  const folderLabel = selectedFolder === 'INBOX' ? 'Inbox' : selectedFolder

  return (
    <div className="inbox-page">
      <div className="inbox-list-pane">
        <div className="list-pane-header">
          <h2>{folderLabel}</h2>
          <span className="email-count">{emails.length} message{emails.length !== 1 ? 's' : ''}</span>
        </div>
        {error && <p className="form-error">{error}</p>}
        {loading && !allEmails.length && <p className="empty-text">Loading…</p>}
        <EmailList
          emails={emails}
          selectedId={selectedId}
          onSelect={setSelectedId}
        />
      </div>
      <div className="inbox-reader-pane">
        <EmailReader emailId={selectedId} />
      </div>
    </div>
  )
}
