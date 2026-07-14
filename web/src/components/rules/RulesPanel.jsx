import { useRules } from '../../hooks/useRules'
import { RulesTable } from './RulesTable'
import { RuleForm } from './RuleForm'

export function RulesPanel() {
  const { rules, loading, error, addRule, deleteRule } = useRules()

  return (
    <div className="rules-panel">
      <div className="panel-header">
        <div>
          <h2>Filter Rules</h2>
          <p className="panel-subtitle">
            Rules are applied in order — first match wins.
          </p>
        </div>
      </div>

      {error && <p className="form-error">{error}</p>}
      {loading && !rules.length && <p className="empty-text">Loading…</p>}

      <RulesTable rules={rules} onDelete={deleteRule} />

      <div className="rules-add-section">
        <h3>Add Rule</h3>
        <RuleForm onAdd={addRule} />
      </div>
    </div>
  )
}
