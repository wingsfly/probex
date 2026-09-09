import { useEffect, useState } from 'react'
import { authRequired, getToken, login } from '../api/auth'

// LoginGate wraps the app. When the backend reports auth_required and no token
// is stored, it shows a password screen instead of the app. Instances without
// PROBEX_AUTH_PASSWORD report auth_required=false and render children directly.
export default function LoginGate({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<'checking' | 'locked' | 'open'>('checking')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    authRequired().then(required => {
      if (!required || getToken()) setStatus('open')
      else setStatus('locked')
    })
  }, [])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setSubmitting(true)
    try {
      await login(password)
      setStatus('open')
    } catch {
      setError('密码错误')
    } finally {
      setSubmitting(false)
    }
  }

  if (status === 'checking') return null
  if (status === 'open') return <>{children}</>

  return (
    <div style={{
      minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: '#f9fafb',
    }}>
      <form onSubmit={handleSubmit} style={{
        background: '#fff', padding: '2rem', borderRadius: 12, width: 320,
        boxShadow: '0 1px 3px rgba(0,0,0,0.1)', border: '1px solid #e5e7eb',
      }}>
        <h1 style={{ fontSize: '1.25rem', fontWeight: 700, marginBottom: 4 }}>ProbeX</h1>
        <p style={{ fontSize: '0.8rem', color: '#6b7280', marginBottom: '1.5rem' }}>
          请输入访问密码
        </p>
        <input
          type="password"
          value={password}
          onChange={e => setPassword(e.target.value)}
          placeholder="密码"
          autoFocus
          style={{
            width: '100%', padding: '0.6rem 0.75rem', fontSize: '0.9rem',
            border: '1px solid #d1d5db', borderRadius: 8, marginBottom: '0.75rem',
            boxSizing: 'border-box',
          }}
        />
        {error && (
          <p style={{ color: '#dc2626', fontSize: '0.8rem', marginBottom: '0.75rem' }}>{error}</p>
        )}
        <button
          type="submit"
          disabled={submitting || !password}
          style={{
            width: '100%', padding: '0.6rem', fontSize: '0.9rem', fontWeight: 600,
            color: '#fff', background: submitting || !password ? '#93c5fd' : '#3b82f6',
            border: 'none', borderRadius: 8, cursor: submitting || !password ? 'default' : 'pointer',
          }}
        >
          {submitting ? '登录中…' : '登录'}
        </button>
      </form>
    </div>
  )
}
