import { useState } from 'react'

const emptyRule = { field: 'sender', operator: 'contains', value: '', action: 'move_to', action_value: '' }

export function RuleForm({ onAdd }) {
  const [rule, setRule] = useState(emptyRule)
  const [error, setError] = useState(null)
  const [busy, setBusy] = useState(false)

  const update = (key) => (e) => setRule({ ...rule, [key]: e.target.value })

  const submit = async (e) => {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      await onAdd(rule)
      setRule(emptyRule)
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
        <button className="btn-primary" type="submit" disabled={busy}>
          {busy ? 'Adding…' : 'Add rule'}
        </button>
      </div>
      {error && <p className="form-error">{error}</p>}
    </form>
  )
}
