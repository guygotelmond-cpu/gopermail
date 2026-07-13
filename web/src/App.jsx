import { useState, useEffect, useCallback } from 'react'
import { api } from './api'

function LoginPanel({ onAuthed }) {
  const [mode, setMode] = useState('login') // 'login' | 'signup'
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e) => {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      if (mode === 'signup') {
        await api.signup(username, password)
      }
      await api.login(username, password)
      onAuthed(username)
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="card auth-card">
      <h1>GoMail</h1>
      <p className="subtitle">Sign in to manage your inbox filter rules</p>
      <form onSubmit={submit}>
        <label>
          Username / Email
          <input value={username} onChange={(e) => setUsername(e.target.value)} required />
        </label>
        <label>
          Password
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        </label>
        {error && <p className="error">{error}</p>}
        <button type="submit" disabled={busy}>
          {busy ? 'Please wait…' : mode === 'login' ? 'Log in' : 'Create account & log in'}
        </button>
      </form>
      <button className="link" onClick={() => setMode(mode === 'login' ? 'signup' : 'login')}>
        {mode === 'login' ? "Need an account? Sign up" : 'Already have an account? Log in'}
      </button>
    </div>
  )
}

const emptyRule = { field: 'sender', operator: 'contains', value: '', action: 'move_to', action_value: '' }

function RuleForm({ username, onAdded }) {
  const [rule, setRule] = useState(emptyRule)
  const [error, setError] = useState(null)
  const [busy, setBusy] = useState(false)

  const update = (key) => (e) => setRule({ ...rule, [key]: e.target.value })

  const submit = async (e) => {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      await api.addRule({ username, ...rule })
      setRule(emptyRule)
      onAdded()
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="rule-form" onSubmit={submit}>
      <div className="rule-form-row">
        <select value={rule.field} onChange={update('field')}>
          <option value="sender">Sender</option>
          <option value="subject">Subject</option>
          <option value="body">Body</option>
        </select>
        <select value={rule.operator} onChange={update('operator')}>
          <option value="contains">contains</option>
          <option value="equals">equals</option>
        </select>
        <input placeholder="value to match" value={rule.value} onChange={update('value')} required />
      </div>
      <div className="rule-form-row">
        <select value={rule.action} onChange={update('action')}>
          <option value="move_to">move to folder</option>
          <option value="mark_spam">mark as spam</option>
          <option value="delete">delete</option>
        </select>
        {rule.action === 'move_to' && (
          <input placeholder="destination folder" value={rule.action_value} onChange={update('action_value')} required />
        )}
        <button type="submit" disabled={busy}>{busy ? 'Adding…' : 'Add rule'}</button>
      </div>
      {error && <p className="error">{error}</p>}
    </form>
  )
}

function RulesTable({ username, rules, onChanged }) {
  const remove = async (id) => {
    await api.deleteRule(username, id)
    onChanged()
  }

  if (!rules.length) return <p className="empty">No rules yet — add one below.</p>

  return (
    <table className="rules-table">
      <thead>
        <tr>
          <th>Field</th><th>Operator</th><th>Value</th><th>Action</th><th>Target</th><th></th>
        </tr>
      </thead>
      <tbody>
        {rules.map((r) => (
          <tr key={r.id}>
            <td>{r.field}</td>
            <td>{r.operator}</td>
            <td>{r.value}</td>
            <td>{r.action}</td>
            <td>{r.action_value || '—'}</td>
            <td><button className="link danger" onClick={() => remove(r.id)}>Delete</button></td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function EmailLog({ emails, onOpen }) {
  if (!emails.length) return <p className="empty">No emails received yet.</p>
  return (
    <table className="rules-table">
      <thead>
        <tr><th>Received</th><th>Sender</th><th>Folder</th><th>Spam</th><th></th></tr>
      </thead>
      <tbody>
        {emails.map((m) => (
          <tr key={m.id} className="clickable" onClick={() => onOpen(m.id)}>
            <td>{new Date(m.received_at).toLocaleString()}</td>
            <td>{m.sender}</td>
            <td>{m.folder}</td>
            <td>{m.is_spam ? 'Yes' : 'No'}</td>
            <td><span className="link">Read</span></td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function EmailReader({ id, username, onClose }) {
  const [payload, setPayload] = useState(null)
  const [error, setError] = useState(null)

  useEffect(() => {
    let cancelled = false
    api.readEmail(id, username)
      .then((p) => { if (!cancelled) setPayload(p) })
      .catch((err) => { if (!cancelled) setError(err.message) })
    return () => { cancelled = true }
  }, [id, username])

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal card" onClick={(e) => e.stopPropagation()}>
        <div className="section-header">
          <h2>Message</h2>
          <button className="link" onClick={onClose}>Close</button>
        </div>
        {error && <p className="error">{error}</p>}
        {!payload && !error && <p className="empty">Loading…</p>}
        {payload && (
          <>
            {payload.headers && Object.keys(payload.headers).length > 0 && (
              <dl className="headers-list">
                {Object.entries(payload.headers).map(([k, v]) => (
                  <div key={k}><dt>{k}</dt><dd>{v}</dd></div>
                ))}
              </dl>
            )}
            <pre className="mail-body">{payload.body}</pre>
          </>
        )}
      </div>
    </div>
  )
}

function ComposeForm({ defaultTo, onSent }) {
  const [form, setForm] = useState({ from: '', to: defaultTo || '', subject: '', body: '' })
  const [error, setError] = useState(null)
  const [status, setStatus] = useState(null)
  const [busy, setBusy] = useState(false)

  const update = (key) => (e) => setForm({ ...form, [key]: e.target.value })

  const submit = async (e) => {
    e.preventDefault()
    setError(null)
    setStatus(null)
    setBusy(true)
    try {
      await api.sendMail(form)
      setStatus('Sent!')
      setForm({ ...form, subject: '', body: '' })
      onSent?.()
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="compose-form" onSubmit={submit}>
      <div className="rule-form-row">
        <input placeholder="from" value={form.from} onChange={update('from')} required />
        <input placeholder="to" value={form.to} onChange={update('to')} required />
      </div>
      <input placeholder="subject" value={form.subject} onChange={update('subject')} />
      <textarea placeholder="message body" rows={4} value={form.body} onChange={update('body')} />
      <div className="rule-form-row">
        <button type="submit" disabled={busy}>{busy ? 'Sending…' : 'Send'}</button>
        {status && <span className="status-ok">{status}</span>}
      </div>
      {error && <p className="error">{error}</p>}
    </form>
  )
}

function Dashboard({ username, onLogout }) {
  const [rules, setRules] = useState([])
  const [emails, setEmails] = useState([])
  const [error, setError] = useState(null)
  const [lastUpdated, setLastUpdated] = useState(null)
  const [refreshing, setRefreshing] = useState(false)
  const [openEmailId, setOpenEmailId] = useState(null)

  const refresh = useCallback(async ({ silent } = {}) => {
    if (!silent) setRefreshing(true)
    try {
      const [r, e] = await Promise.all([api.listRules(username), api.recentEmails(username)])
      setRules(r || [])
      setEmails(e || [])
      setError(null)
      setLastUpdated(new Date())
    } catch (err) {
      setError(err.message)
    } finally {
      if (!silent) setRefreshing(false)
    }
  }, [username])

  // Initial load + poll for new mail/rule changes so you don't have to
  // manually reload the page. Pauses while the tab is hidden.
  useEffect(() => {
    refresh()
    const interval = setInterval(() => {
      if (document.visibilityState === 'visible') refresh({ silent: true })
    }, 4000)
    return () => clearInterval(interval)
  }, [refresh])

  return (
    <div className="dashboard">
      <header>
        <h1>GoMail</h1>
        <div>
          <span className="username">{username}</span>
          <button className="link" onClick={onLogout}>Log out</button>
        </div>
      </header>

      {error && <p className="error">{error}</p>}

      <section className="card">
        <div className="section-header">
          <h2>Filter rules</h2>
          <RefreshControl lastUpdated={lastUpdated} refreshing={refreshing} onRefresh={() => refresh()} />
        </div>
        <RulesTable username={username} rules={rules} onChanged={refresh} />
        <RuleForm username={username} onAdded={refresh} />
      </section>

      <section className="card">
        <div className="section-header">
          <h2>Recent mail</h2>
          <RefreshControl lastUpdated={lastUpdated} refreshing={refreshing} onRefresh={() => refresh()} />
        </div>
        <EmailLog emails={emails} onOpen={setOpenEmailId} />
      </section>

      <section className="card">
        <h2>Compose</h2>
        <p className="subtitle">Send a test email through the SMTP server — useful for testing your rules.</p>
        <ComposeForm defaultTo={username} onSent={() => refresh()} />
      </section>

      {openEmailId && (
        <EmailReader id={openEmailId} username={username} onClose={() => setOpenEmailId(null)} />
      )}
    </div>
  )
}

function RefreshControl({ lastUpdated, refreshing, onRefresh }) {
  return (
    <div className="refresh-control">
      {lastUpdated && (
        <span className="updated-at">Updated {lastUpdated.toLocaleTimeString()}</span>
      )}
      <button className="link" onClick={onRefresh} disabled={refreshing}>
        {refreshing ? 'Refreshing…' : 'Refresh'}
      </button>
    </div>
  )
}

export default function App() {
  const [username, setUsername] = useState(null)

  // Full reset on logout: clear the authed user so Dashboard unmounts
  // (dropping rules/emails/polling state) rather than lingering stale data.
  const handleLogout = () => {
    setUsername(null)
  }

  if (!username) return <LoginPanel onAuthed={setUsername} />
  return <Dashboard key={username} username={username} onLogout={handleLogout} />
}
