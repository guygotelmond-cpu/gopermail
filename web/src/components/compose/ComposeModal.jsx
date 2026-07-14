import { useState, useEffect } from 'react'
import { useEditor, EditorContent } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { TextStyle, Color, FontFamily, FontSize } from '@tiptap/extension-text-style'
import { api } from '../../utils/api'
import { useAuth } from '../../hooks/useAuth'

const FONTS = [
  { label: 'Default', value: '' },
  { label: 'Sans-serif', value: 'Arial, sans-serif' },
  { label: 'Serif', value: 'Georgia, serif' },
  { label: 'Monospace', value: 'Courier New, monospace' },
]

const SIZES = ['12px', '14px', '16px', '18px', '24px', '32px']

const COLORS = [
  '#0f172a', '#dc2626', '#16a34a', '#2563eb',
  '#9333ea', '#ea580c', '#0891b2', '#64748b',
]

function ToolbarButton({ active, onClick, title, children }) {
  return (
    <button
      type="button"
      className={`compose-tb-btn${active ? ' active' : ''}`}
      onClick={onClick}
      title={title}
    >
      {children}
    </button>
  )
}

function Divider() {
  return <span className="compose-tb-divider" />
}

function ComposeToolbar({ editor }) {
  const [fontSize, setFontSize] = useState('14px')

  if (!editor) return null

  const applyFontSize = (size) => {
    setFontSize(size)
    editor.chain().focus().setFontSize(size).run()
  }

  const applyFontFamily = (e) => {
    const val = e.target.value
    if (val) editor.chain().focus().setFontFamily(val).run()
    else editor.chain().focus().unsetFontFamily().run()
  }

  const applyColor = (color) => {
    editor.chain().focus().setColor(color).run()
  }

  return (
    <div className="compose-toolbar">
      <ToolbarButton
        active={editor.isActive('bold')}
        onClick={() => editor.chain().focus().toggleBold().run()}
        title="Bold"
      >
        <b>B</b>
      </ToolbarButton>
      <ToolbarButton
        active={editor.isActive('italic')}
        onClick={() => editor.chain().focus().toggleItalic().run()}
        title="Italic"
      >
        <i>I</i>
      </ToolbarButton>
      <ToolbarButton
        active={editor.isActive('strike')}
        onClick={() => editor.chain().focus().toggleStrike().run()}
        title="Strikethrough"
      >
        <s>S</s>
      </ToolbarButton>

      <Divider />

      <ToolbarButton
        active={editor.isActive('bulletList')}
        onClick={() => editor.chain().focus().toggleBulletList().run()}
        title="Bullet list"
      >
        ≡
      </ToolbarButton>
      <ToolbarButton
        active={editor.isActive('orderedList')}
        onClick={() => editor.chain().focus().toggleOrderedList().run()}
        title="Ordered list"
      >
        1.
      </ToolbarButton>

      <Divider />

      <select
        className="compose-tb-select"
        onChange={applyFontFamily}
        defaultValue=""
        title="Font family"
      >
        {FONTS.map(f => (
          <option key={f.value} value={f.value}>{f.label}</option>
        ))}
      </select>

      <select
        className="compose-tb-select compose-tb-size"
        value={fontSize}
        onChange={e => applyFontSize(e.target.value)}
        title="Font size"
      >
        {SIZES.map(s => (
          <option key={s} value={s}>{s}</option>
        ))}
      </select>

      <Divider />

      <div className="compose-tb-colors" title="Text color">
        {COLORS.map(c => (
          <button
            key={c}
            type="button"
            className="compose-tb-color-dot"
            style={{ background: c }}
            onClick={() => applyColor(c)}
            title={c}
          />
        ))}
      </div>
    </div>
  )
}

export function ComposeModal({ onClose, onSent }) {
  const { username } = useAuth()
  const [form, setForm] = useState({ from: username || '', to: '', subject: '' })
  const [error, setError] = useState(null)
  const [status, setStatus] = useState(null)
  const [busy, setBusy] = useState(false)

  const editor = useEditor({
    extensions: [
      StarterKit,
      TextStyle,
      Color,
      FontFamily,
      FontSize,
    ],
    content: '',
    editorProps: {
      attributes: {
        class: 'compose-editor-content',
      },
    },
  })

  useEffect(() => {
    const onKey = (e) => {
      if (e.key !== 'Escape') return
      const hasContent = form.to || form.subject || (editor?.getText().trim())
      if (!hasContent) onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose, form, editor])

  const update = (key) => (e) => setForm({ ...form, [key]: e.target.value })

  const submit = async (e) => {
    e.preventDefault()
    setError(null)
    setStatus(null)
    setBusy(true)
    try {
      const html = editor?.getHTML() || ''
      await api.sendMail({ ...form, body: html, isHTML: true })
      setStatus('Message sent!')
      editor?.commands.clearContent()
      setForm({ from: username || '', to: '', subject: '' })
      onSent?.()
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="compose-backdrop">
      <div className="compose-modal compose-modal--rich" onClick={(e) => e.stopPropagation()}>
        <div className="compose-header">
          <span>New Message</span>
          <button className="btn-icon" onClick={onClose} title="Close">✕</button>
        </div>
        <form onSubmit={submit}>
          <div className="compose-field">
            <label>From</label>
            <input value={form.from} onChange={update('from')} placeholder="sender@example.com" required />
          </div>
          <div className="compose-field">
            <label>To</label>
            <input value={form.to} onChange={update('to')} placeholder="recipient@example.com" required />
          </div>
          <div className="compose-field">
            <label>Subject</label>
            <input value={form.subject} onChange={update('subject')} placeholder="Subject" />
          </div>

          <ComposeToolbar editor={editor} />
          <EditorContent editor={editor} className="compose-editor-wrap" />

          {error && <p className="form-error">{error}</p>}
          {status && <p className="form-success">{status}</p>}
          <div className="compose-actions">
            <button className="btn-primary" type="submit" disabled={busy}>
              {busy ? 'Sending…' : 'Send'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
