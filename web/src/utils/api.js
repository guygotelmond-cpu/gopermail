// Empty string → relative URLs handled by nginx reverse proxy in docker.
// Non-empty (e.g. http://localhost:8080) → used directly (local dev).
const API_URL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

function getToken() {
  return localStorage.getItem('gm_token')
}

async function request(path, options = {}) {
  const token = getToken()
  const headers = { 'Content-Type': 'application/json', ...options.headers }
  if (token) headers['Authorization'] = `Bearer ${token}`
  const res = await fetch(`${API_URL}${path}`, { ...options, headers })
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

  listRules: () => request('/api/rules'),

  addRule: (rule) => request('/api/rules', { method: 'POST', body: JSON.stringify(rule) }),

  deleteRule: (id) =>
    request(`/api/rules/${id}`, { method: 'DELETE' }),

  recentEmails: (limit = 50) =>
    request(`/api/emails?limit=${limit}`),

  readEmail: (id) =>
    request(`/api/emails/${encodeURIComponent(id)}`),

  sendMail: ({ from, to, subject, body, isHTML = false }) =>
    request('/api/send', { method: 'POST', body: JSON.stringify({ from, to, subject, body, is_html: isHTML }) }),

  changePassword: (oldPassword, newPassword) =>
    request('/api/password', {
      method: 'PUT',
      body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
    }),

  totpStatus: () => request('/api/totp/status'),

  totpSetup: () => request('/api/totp/setup', { method: 'POST' }),

  totpEnable: (code) =>
    request('/api/totp/enable', { method: 'POST', body: JSON.stringify({ code }) }),

  totpDisable: (password, code) =>
    request('/api/totp/disable', { method: 'POST', body: JSON.stringify({ password, code }) }),

  totpLogin: (partialToken, code) =>
    request('/api/totp/login', { method: 'POST', body: JSON.stringify({ partial_token: partialToken, code }) }),
}
