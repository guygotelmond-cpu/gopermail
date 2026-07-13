const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

async function request(path, options = {}) {
  const res = await fetch(`${API_URL}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  const isJSON = res.headers.get('content-type')?.includes('application/json')
  const body = isJSON ? await res.json() : null
  if (!res.ok) {
    throw new Error(body?.error || `Request failed with status ${res.status}`)
  }
  return body
}

export const api = {
  signup: (username, password) =>
    request('/api/signup', { method: 'POST', body: JSON.stringify({ username, password }) }),

  login: (username, password) =>
    request('/api/login', { method: 'POST', body: JSON.stringify({ username, password }) }),

  listRules: (username) => request(`/api/rules?username=${encodeURIComponent(username)}`),

  addRule: (rule) => request('/api/rules', { method: 'POST', body: JSON.stringify(rule) }),

  deleteRule: (username, id) =>
    request(`/api/rules/${id}?username=${encodeURIComponent(username)}`, { method: 'DELETE' }),

  recentEmails: (username, limit = 50) =>
    request(`/api/emails?username=${encodeURIComponent(username)}&limit=${limit}`),

  readEmail: (id, username) =>
    request(`/api/emails/${encodeURIComponent(id)}?username=${encodeURIComponent(username)}`),

  sendMail: ({ from, to, subject, body }) =>
    request('/api/send', { method: 'POST', body: JSON.stringify({ from, to, subject, body }) }),
}
