import { useState, useEffect, useCallback } from 'react'
import { api } from '../utils/api'

export function useRules() {
  const [rules, setRules] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)

  const fetchRules = useCallback(async () => {
    setLoading(true)
    try {
      const result = await api.listRules()
      setRules(result || [])
      setError(null)
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchRules() }, [fetchRules])

  const addRule = useCallback(async (rule) => {
    await api.addRule(rule)
    await fetchRules()
  }, [fetchRules])

  const deleteRule = useCallback(async (id) => {
    await api.deleteRule(id)
    await fetchRules()
  }, [fetchRules])

  return { rules, loading, error, refresh: fetchRules, addRule, deleteRule }
}
