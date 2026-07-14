import { useState, useEffect, useCallback } from 'react'
import { api } from '../utils/api'

const API_BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

export function useEmails() {
  const [emails, setEmails] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const [lastUpdated, setLastUpdated] = useState(null)

  const fetchEmails = useCallback(async ({ silent } = {}) => {
    if (!silent) setLoading(true)
    try {
      const result = await api.recentEmails()
      setEmails(result || [])
      setLastUpdated(new Date())
      setError(null)
    } catch (err) {
      setError(err.message)
    } finally {
      if (!silent) setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchEmails()

    const token = localStorage.getItem('gm_token')
    if (!token) return

    const url = `${API_BASE}/api/events?token=${encodeURIComponent(token)}`
    const es = new EventSource(url)

    es.onmessage = (e) => {
      if (e.data === 'new_mail') fetchEmails({ silent: true })
    }

    // EventSource reconnects automatically on error — no manual retry needed.
    es.onerror = () => {}

    return () => es.close()
  }, [fetchEmails])

  return { emails, loading, error, lastUpdated, refresh: fetchEmails }
}
