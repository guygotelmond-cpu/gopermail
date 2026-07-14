import { useState } from 'react'
import { api } from '../../utils/api'
import { useAuth } from '../../hooks/useAuth'

export function AuthPage() {
  const { login } = useAuth()
  const [mode, setMode] = useState('login')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState(null)
  const [busy, setBusy] = useState(false)

  // TOTP step
  const [partialToken, setPartialToken] = useState(null)
  const [totpCode, setTotpCode] = useState('')

  const submit = async (e) => {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      if (mode === 'signup') await api.signup(username, password)
      const res = await api.login(username, password)
      if (res.requires_totp) {
        setPartialToken(res.partial_token)
      } else {
        login(res.username, res.token)
      }
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  const submitTOTP = async (e) => {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      const res = await api.totpLogin(partialToken, totpCode)
      login(res.username, res.token)
    } catch (err) {
      setError(err.message)
      setTotpCode('')
    } finally {
      setBusy(false)
    }
  }

  if (partialToken) {
    return (
      <div className="auth-page">
        <div className="auth-card">
          <div className="auth-logo">
            <span className="auth-logo-icon">🔐</span>
            <h1>Two-Factor Auth</h1>
          </div>
          <p className="auth-subtitle">Enter the 6-digit code from your authenticator app</p>
          <form onSubmit={submitTOTP}>
            <div className="form-field">
              <label>Code</label>
              <input
                value={totpCode}
                onChange={(e) => setTotpCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                placeholder="000000"
                className="totp-input"
                inputMode="numeric"
                autoComplete="one-time-code"
                autoFocus
                required
              />
            </div>
            {error && <p className="form-error">{error}</p>}
            <button className="btn-primary btn-full" type="submit" disabled={busy || totpCode.length !== 6}>
              {busy ? 'Verifying…' : 'Verify'}
            </button>
          </form>
          <p className="auth-toggle">
            <button className="btn-link" onClick={() => { setPartialToken(null); setTotpCode(''); setError(null) }}>
              Back to sign in
            </button>
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="auth-page">
      <div className="auth-card">
        <div className="auth-logo">
          <span className="auth-logo-icon">✉</span>
          <h1>GoPerMail</h1>
        </div>
        <p className="auth-subtitle">
          {mode === 'login' ? 'Sign in to your inbox' : 'Create a new account'}
        </p>
        <form onSubmit={submit}>
          <div className="form-field">
            <label>Username</label>
            <input
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="you@example.com"
              required
              autoFocus
            />
          </div>
          <div className="form-field">
            <label>Password</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              required
            />
          </div>
          {error && <p className="form-error">{error}</p>}
          <button className="btn-primary btn-full" type="submit" disabled={busy}>
            {busy ? 'Please wait…' : mode === 'login' ? 'Sign in' : 'Create account'}
          </button>
        </form>
        <p className="auth-toggle">
          {mode === 'login' ? "Don't have an account? " : 'Already have an account? '}
          <button className="btn-link" onClick={() => setMode(mode === 'login' ? 'signup' : 'login')}>
            {mode === 'login' ? 'Sign up' : 'Sign in'}
          </button>
        </p>
      </div>
    </div>
  )
}
