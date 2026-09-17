import { useState } from 'react'
import { Link, Navigate, useNavigate } from 'react-router-dom'
import { errorMessage } from '../api/client'
import GenrePicker from '../components/GenrePicker.jsx'
import { useAuth } from '../context/auth'

export default function Register() {
  const { register, isAuthenticated } = useAuth()
  const navigate = useNavigate()

  const [form, setForm] = useState({ first_name: '', last_name: '', email: '', password: '', confirm: '' })
  const [genres, setGenres] = useState([])
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  if (isAuthenticated && !submitting) return <Navigate to="/" replace />

  const set = (key) => (e) => setForm({ ...form, [key]: e.target.value })

  const onSubmit = async (e) => {
    e.preventDefault()
    setError('')
    if (form.password.length < 8) return setError('Password must be at least 8 characters.')
    if (form.password !== form.confirm) return setError('Passwords do not match.')
    if (genres.length === 0) return setError('Pick at least one favourite genre so we can personalise your picks.')

    setSubmitting(true)
    try {
      const { confirm: _confirm, ...payload } = form
      await register({ ...payload, favourite_genres: genres })
      navigate('/recommended', { replace: true })
    } catch (err) {
      setError(errorMessage(err, 'Registration failed.'))
      setSubmitting(false)
    }
  }

  return (
    <div className="auth-page">
      <form className="card auth-card wide" onSubmit={onSubmit}>
        <h1>Create your account</h1>
        <p className="muted">Tell us what you love and our AI will do the rest.</p>

        {error && <p className="alert">{error}</p>}

        <div className="field-row">
          <label className="field">
            <span>First name</span>
            <input className="input" required autoComplete="given-name" value={form.first_name} onChange={set('first_name')} />
          </label>
          <label className="field">
            <span>Last name</span>
            <input className="input" required autoComplete="family-name" value={form.last_name} onChange={set('last_name')} />
          </label>
        </div>
        <label className="field">
          <span>Email</span>
          <input className="input" type="email" required autoComplete="email" value={form.email} onChange={set('email')} />
        </label>
        <div className="field-row">
          <label className="field">
            <span>Password</span>
            <input
              className="input"
              type="password"
              required
              minLength={8}
              maxLength={72}
              autoComplete="new-password"
              value={form.password}
              onChange={set('password')}
            />
          </label>
          <label className="field">
            <span>Confirm password</span>
            <input
              className="input"
              type="password"
              required
              autoComplete="new-password"
              value={form.confirm}
              onChange={set('confirm')}
            />
          </label>
        </div>

        <div className="field">
          <span>Favourite genres</span>
          <GenrePicker value={genres} onChange={setGenres} />
        </div>

        <button className="btn btn-primary btn-block" disabled={submitting}>
          {submitting ? 'Creating account…' : 'Create account'}
        </button>
        <p className="muted center">
          Already have an account? <Link to="/login">Log in</Link>
        </p>
      </form>
    </div>
  )
}
