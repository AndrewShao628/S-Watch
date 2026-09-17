import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { adminApi, errorMessage } from '../api/client'
import GenrePicker from '../components/GenrePicker.jsx'

// Accepts a bare 11-char id or any common YouTube URL form.
function parseYouTubeId(input) {
  const trimmed = input.trim()
  if (/^[\w-]{11}$/.test(trimmed)) return trimmed
  const match = trimmed.match(/(?:youtu\.be\/|v=|embed\/|shorts\/)([\w-]{11})/)
  return match ? match[1] : ''
}

export default function AddMovie() {
  const navigate = useNavigate()
  const [form, setForm] = useState({ imdb_id: '', title: '', year: '', overview: '', youtube: '', admin_review: '' })
  const [genres, setGenres] = useState([])
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  const set = (key) => (e) => setForm({ ...form, [key]: e.target.value })
  const youtubeId = parseYouTubeId(form.youtube)

  const onSubmit = async (e) => {
    e.preventDefault()
    setError('')
    if (!youtubeId) return setError('Enter a valid YouTube URL or video id.')
    if (genres.length === 0) return setError('Pick at least one genre.')

    setSaving(true)
    try {
      const movie = await adminApi.createMovie({
        imdb_id: form.imdb_id.trim(),
        title: form.title,
        year: Number(form.year),
        overview: form.overview,
        youtube_id: youtubeId,
        genre: genres,
        admin_review: form.admin_review,
      })
      navigate(`/watch/${movie.imdb_id}`)
    } catch (err) {
      setError(errorMessage(err))
      setSaving(false)
    }
  }

  return (
    <div className="admin-page">
      <header className="page-header">
        <div>
          <p className="eyebrow">Admin</p>
          <h1>Add a movie</h1>
        </div>
      </header>

      <form className="card" onSubmit={onSubmit}>
        <div className="field-row">
          <label className="field">
            <span>IMDb ID</span>
            <input className="input" required pattern="tt\d{7,9}" placeholder="tt1234567" value={form.imdb_id} onChange={set('imdb_id')} />
          </label>
          <label className="field">
            <span>Year</span>
            <input className="input" type="number" required min={1888} max={2100} value={form.year} onChange={set('year')} />
          </label>
        </div>
        <label className="field">
          <span>Title</span>
          <input className="input" required maxLength={300} value={form.title} onChange={set('title')} />
        </label>
        <label className="field">
          <span>Overview</span>
          <textarea className="input" rows={3} maxLength={2000} value={form.overview} onChange={set('overview')} />
        </label>
        <label className="field">
          <span>YouTube trailer URL or id</span>
          <input className="input" required placeholder="https://www.youtube.com/watch?v=…" value={form.youtube} onChange={set('youtube')} />
        </label>
        {youtubeId && (
          <img className="thumb-preview" src={`https://img.youtube.com/vi/${youtubeId}/hqdefault.jpg`} alt="Trailer thumbnail preview" />
        )}
        <div className="field">
          <span>Genres</span>
          <GenrePicker value={genres} onChange={setGenres} />
        </div>
        <label className="field">
          <span>Admin review (optional, ranked automatically by AI)</span>
          <textarea className="input" rows={4} maxLength={2000} value={form.admin_review} onChange={set('admin_review')} />
        </label>

        {error && <p className="alert">{error}</p>}
        <div className="form-actions">
          <button className="btn btn-primary" disabled={saving}>
            {saving ? 'Saving…' : 'Add movie'}
          </button>
        </div>
      </form>
    </div>
  )
}
