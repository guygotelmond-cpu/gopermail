import { useState, useEffect } from 'react'
import { useAuth } from '../hooks/useAuth'
import { api } from '../utils/api'

function Section({ title, children }) {
  return (
    <section className="settings-section">
      <h3 className="settings-section-title">{title}</h3>
      <div className="settings-section-body">{children}</div>
    </section>
  )
}

function FieldRow({ label, hint, children }) {
  return (
    <div className="settings-field-row">
      <div className="settings-field-label">
        <span>{label}</span>
        {hint && <span className="settings-field-hint">{hint}</span>}
      </div>
      <div className="settings-field-control">{children}</div>
    </div>
  )
}

// ─── 2FA Section ─────────────────────────────────────────────────────────────

function TwoFactorSection() {
  const [enabled, setEnabled] = useState(null) // null = loading
  const [step, setStep] = useState('idle')     // idle | setup | disable
  const [qr, setQr] = useState(null)
  const [secret, setSecret] = useState(null)
  const [code, setCode] = useState('')
  const [disablePassword, setDisablePassword] = useState('')
  const [disableCode, setDisableCode] = useState('')
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState(null) // {ok, text}

  useEffect(() => {
    api.totpStatus()
      .then(res => setEnabled(res.enabled))
      .catch(() => setEnabled(false))
  }, [])

  const startSetup = async () => {
    setMsg(null)
    setBusy(true)
    try {
      const res = await api.totpSetup()
      setQr(res.qr)
      setSecret(res.secret)
      setCode('')
      setStep('setup')
    } catch (err) {
      setMsg({ ok: false, text: err.message })
    } finally {
      setBusy(false)
    }
  }

  const confirmEnable = async (e) => {
    e.preventDefault()
    setMsg(null)
    setBusy(true)
    try {
      await api.totpEnable(code)
      setEnabled(true)
      setStep('idle')
      setQr(null)
      setSecret(null)
      setCode('')
      setMsg({ ok: true, text: '2FA enabled successfully' })
    } catch (err) {
      setMsg({ ok: false, text: err.message })
    } finally {
      setBusy(false)
    }
  }

  const confirmDisable = async (e) => {
    e.preventDefault()
    setMsg(null)
    setBusy(true)
    try {
      await api.totpDisable(disablePassword, disableCode)
      setEnabled(false)
      setStep('idle')
      setDisablePassword('')
      setDisableCode('')
      setMsg({ ok: true, text: '2FA disabled' })
    } catch (err) {
      setMsg({ ok: false, text: err.message })
    } finally {
      setBusy(false)
    }
  }

  if (enabled === null) return <p className="settings-field-hint">Loading…</p>

  return (
    <div className="totp-section">
      <FieldRow
        label="Status"
        hint={enabled ? 'Your account is protected with 2FA' : 'Add an extra layer of login security'}
      >
        <span className={`totp-badge${enabled ? ' totp-badge--on' : ' totp-badge--off'}`}>
          {enabled ? 'Enabled' : 'Disabled'}
        </span>
      </FieldRow>

      {msg && <p className={msg.ok ? 'form-success' : 'form-error'}>{msg.text}</p>}

      {step === 'idle' && !enabled && (
        <button className="btn-primary" onClick={startSetup} disabled={busy}>
          {busy ? 'Generating…' : 'Enable 2FA'}
        </button>
      )}

      {step === 'idle' && enabled && (
        <button
          className="btn-danger-link"
          style={{ fontSize: '13px', padding: '6px 12px' }}
          onClick={() => { setStep('disable'); setMsg(null) }}
        >
          Disable 2FA
        </button>
      )}

      {step === 'setup' && (
        <div className="totp-setup">
          <p className="totp-instructions">
            Scan this QR code with an authenticator app (Google Authenticator, Authy, etc.), then enter the 6-digit code to confirm.
          </p>
          {qr && <img src={qr} alt="TOTP QR code" className="totp-qr" />}
          {secret && (
            <p className="totp-secret-hint">
              Can't scan? Enter this key manually: <code className="totp-secret">{secret}</code>
            </p>
          )}
          <form onSubmit={confirmEnable} className="totp-confirm-form">
            <input
              value={code}
              onChange={e => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
              placeholder="000000"
              className="totp-input"
              inputMode="numeric"
              autoComplete="one-time-code"
              required
            />
            <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
              <button className="btn-primary" type="submit" disabled={busy || code.length !== 6}>
                {busy ? 'Verifying…' : 'Confirm'}
              </button>
              <button
                type="button"
                className="btn-link"
                onClick={() => { setStep('idle'); setQr(null); setSecret(null); setCode(''); setMsg(null) }}
              >
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      {step === 'disable' && (
        <form onSubmit={confirmDisable} className="totp-disable-form">
          <p className="totp-instructions">Confirm your password and current authenticator code to disable 2FA.</p>
          <div className="form-field" style={{ border: 'none', padding: 0, flexDirection: 'column', alignItems: 'flex-start', gap: 8 }}>
            <input
              type="password"
              value={disablePassword}
              onChange={e => setDisablePassword(e.target.value)}
              placeholder="Current password"
              required
            />
            <input
              value={disableCode}
              onChange={e => setDisableCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
              placeholder="6-digit code"
              className="totp-input"
              inputMode="numeric"
              autoComplete="one-time-code"
              required
            />
          </div>
          <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
            <button
              className="btn-primary"
              style={{ background: 'var(--c-danger)' }}
              type="submit"
              disabled={busy || disableCode.length !== 6 || !disablePassword}
            >
              {busy ? 'Disabling…' : 'Disable 2FA'}
            </button>
            <button
              type="button"
              className="btn-link"
              onClick={() => { setStep('idle'); setDisablePassword(''); setDisableCode(''); setMsg(null) }}
            >
              Cancel
            </button>
          </div>
        </form>
      )}
    </div>
  )
}

// ─── Main settings page ───────────────────────────────────────────────────────

export function SettingsPage({ theme, onThemeChange }) {
  const { username } = useAuth()

  const [oldPw, setOldPw] = useState('')
  const [newPw, setNewPw] = useState('')
  const [confirmPw, setConfirmPw] = useState('')
  const [pwStatus, setPwStatus] = useState(null)
  const [pwBusy, setPwBusy] = useState(false)

  async function handleChangePassword(e) {
    e.preventDefault()
    if (newPw !== confirmPw) { setPwStatus({ ok: false, msg: 'New passwords do not match' }); return }
    if (newPw.length < 6) { setPwStatus({ ok: false, msg: 'New password must be at least 6 characters' }); return }
    setPwBusy(true)
    setPwStatus(null)
    try {
      await api.changePassword(oldPw, newPw)
      setPwStatus({ ok: true, msg: 'Password updated successfully' })
      setOldPw(''); setNewPw(''); setConfirmPw('')
    } catch (err) {
      setPwStatus({ ok: false, msg: err.message })
    } finally {
      setPwBusy(false)
    }
  }

  return (
    <div className="settings-page">
      <div className="settings-inner">
        <div className="settings-header">
          <h2>Settings</h2>
          <p className="settings-subtitle">Manage your account and preferences</p>
        </div>

        <Section title="Profile">
          <FieldRow label="Username" hint="Cannot be changed">
            <span className="settings-value">{username}</span>
          </FieldRow>
        </Section>

        <Section title="Security">
          <form onSubmit={handleChangePassword} className="settings-pw-form">
            <div className="settings-form-fields">
              <div className="form-field">
                <label>Current password</label>
                <input
                  type="password"
                  value={oldPw}
                  onChange={e => setOldPw(e.target.value)}
                  autoComplete="current-password"
                />
              </div>
              <div className="form-field">
                <label>New password</label>
                <input
                  type="password"
                  value={newPw}
                  onChange={e => setNewPw(e.target.value)}
                  autoComplete="new-password"
                />
              </div>
              <div className="form-field">
                <label>Confirm new password</label>
                <input
                  type="password"
                  value={confirmPw}
                  onChange={e => setConfirmPw(e.target.value)}
                  autoComplete="new-password"
                />
              </div>
            </div>
            {pwStatus && (
              <p className={pwStatus.ok ? 'form-success' : 'form-error'}>{pwStatus.msg}</p>
            )}
            <button type="submit" className="btn-primary" disabled={pwBusy}>
              {pwBusy ? 'Updating…' : 'Update password'}
            </button>
          </form>
        </Section>

        <Section title="Two-Factor Authentication">
          <TwoFactorSection />
        </Section>

        <Section title="Appearance">
          <FieldRow label="Theme" hint="Changes take effect immediately">
            <div className="settings-theme-toggle">
              {['light', 'dark'].map(t => (
                <button
                  key={t}
                  className={`theme-option${theme === t ? ' active' : ''}`}
                  onClick={() => onThemeChange(t)}
                >
                  {t === 'light' ? '☀ Light' : '🌙 Dark'}
                </button>
              ))}
            </div>
          </FieldRow>
        </Section>

        <Section title="About">
          <FieldRow label="Version">
            <span className="settings-value">GoPerMail 1.0</span>
          </FieldRow>
          <FieldRow label="Stack">
            <span className="settings-value">Go · PostgreSQL · MongoDB · React</span>
          </FieldRow>
        </Section>
      </div>
    </div>
  )
}
