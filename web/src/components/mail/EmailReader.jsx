import { useState, useEffect } from 'react'
import { api } from '../../utils/api'

function isHTML(headers) {
  const ct = headers?.['Content-Type'] || ''
  return ct.toLowerCase().includes('text/html')
}

function HeaderRow({ label, value }) {
  return (
    <tr className="email-header-row">
      <td className="email-header-label">{label}</td>
      <td className="email-header-value">{value}</td>
    </tr>
  )
}

export function EmailReader({ emailId }) {
  const [payload, setPayload] = useState(null)
  const [error, setError] = useState(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!emailId) { setPayload(null); setError(null); return }
    let cancelled = false
    setLoading(true)
    setPayload(null)
    setError(null)
    api.readEmail(emailId)
      .then((p) => { if (!cancelled) setPayload(p) })
      .catch((err) => { if (!cancelled) setError(err.message) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [emailId])

  if (!emailId) {
    return (
      <div className="email-reader-empty">
        <div className="empty-icon">✉️</div>
        <p>Select a message to read</p>
      </div>
    )
  }

  if (loading) return <div className="email-reader-loading">Loading…</div>
  if (error) return <div className="email-reader-error">{error}</div>
  if (!payload) return null

  const important = ['From', 'To', 'Subject', 'Date']
  const importantHeaders = {}
  const otherHeaders = {}
  if (payload.headers) {
    for (const [k, v] of Object.entries(payload.headers)) {
      if (important.includes(k)) importantHeaders[k] = v
      else otherHeaders[k] = v
    }
  }

  return (
    <div className="email-reader">
      <div className="email-reader-header">
        <h2 className="email-reader-subject">
          {importantHeaders['Subject'] || '(no subject)'}
        </h2>
        <table className="email-header-table">
          <tbody>
            {importantHeaders['From'] && <HeaderRow label="From" value={importantHeaders['From']} />}
            {importantHeaders['To'] && <HeaderRow label="To" value={importantHeaders['To']} />}
            {importantHeaders['Date'] && <HeaderRow label="Date" value={importantHeaders['Date']} />}
          </tbody>
        </table>
        {Object.keys(otherHeaders).length > 0 && (
          <details className="email-extra-headers">
            <summary>Show all headers</summary>
            <table className="email-header-table">
              <tbody>
                {Object.entries(otherHeaders).map(([k, v]) => (
                  <HeaderRow key={k} label={k} value={v} />
                ))}
              </tbody>
            </table>
          </details>
        )}
      </div>
      <div className="email-reader-body">
        {isHTML(payload.headers)
          ? <div className="email-body-html" dangerouslySetInnerHTML={{ __html: payload.body }} />
          : <pre className="email-body-text">{payload.body}</pre>
        }
      </div>
    </div>
  )
}
