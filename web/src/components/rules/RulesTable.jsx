export function RulesTable({ rules, onDelete }) {
  if (!rules.length) {
    return <p className="empty-text">No rules yet — add one below.</p>
  }

  return (
    <table className="data-table">
      <thead>
        <tr>
          <th>Field</th>
          <th>Operator</th>
          <th>Value</th>
          <th>Action</th>
          <th>Target</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {rules.map((r) => (
          <tr key={r.id}>
            <td><span className="badge">{r.field}</span></td>
            <td>{r.operator}</td>
            <td><code>{r.value}</code></td>
            <td>{r.action.replace('_', ' ')}</td>
            <td>{r.action_value || '—'}</td>
            <td>
              <button className="btn-danger-link" onClick={() => onDelete(r.id)}>
                Delete
              </button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
