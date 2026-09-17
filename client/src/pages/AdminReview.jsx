import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { errorMessage, adminApi, movieApi } from '../api/client'
import RankingBadge from '../components/RankingBadge.jsx'
import Spinner from '../components/Spinner.jsx'

export default function AdminReview() {
  const { imdbId } = useParams()
  const navigate = useNavigate()
  const [movie, setMovie] = useState(null)
  const [rankings, setRankings] = useState([])
  const [review, setReview] = useState('')
  const [manualRanking, setManualRanking] = useState('')
  const [needsManual, setNeedsManual] = useState(false)
  const [status, setStatus] = useState({ saving: false, error: '', message: '' })

  useEffect(() => {
    movieApi
      .get(imdbId)
      .then((m) => {
        setMovie(m)
        setReview(m.admin_review ?? '')
      })
      .catch((err) => setStatus((s) => ({ ...s, error: errorMessage(err) })))
    movieApi.rankings().then(setRankings).catch(() => {})
  }, [imdbId])

  const onSubmit = async (e) => {
    e.preventDefault()
    setStatus({ saving: true, error: '', message: '' })
    try {
      const { movie: updated, ranking_source } = await adminApi.updateReview(imdbId, review, manualRanking || undefined)
      setMovie(updated)
      setNeedsManual(false)
      setStatus({
        saving: false,
        error: '',
        message:
          ranking_source === 'ai'
            ? `AI ranked this review as "${updated.ranking.ranking_name}".`
            : `Saved with manual ranking "${updated.ranking.ranking_name}".`,
      })
    } catch (err) {
      if (err.response?.status === 422) setNeedsManual(true)
      setStatus({ saving: false, error: errorMessage(err), message: '' })
    }
  }

  const onDelete = async () => {
    if (!window.confirm(`Delete "${movie.title}" from the catalogue? This cannot be undone.`)) return
    try {
      await adminApi.deleteMovie(imdbId)
      navigate('/', { replace: true })
    } catch (err) {
      setStatus({ saving: false, error: errorMessage(err), message: '' })
    }
  }

  if (!movie) return status.error ? <p className="alert">{status.error}</p> : <Spinner />

  return (
    <div className="admin-page">
      <header className="page-header">
        <div>
          <p className="eyebrow">Admin · Review</p>
          <h1>{movie.title}</h1>
          <p className="muted">
            {movie.year} · <Link to={`/watch/${movie.imdb_id}`}>Watch trailer</Link>
          </p>
        </div>
        <RankingBadge ranking={movie.ranking} large />
      </header>

      <form className="card" onSubmit={onSubmit}>
        <label className="field">
          <span>Admin review</span>
          <textarea
            className="input"
            rows={6}
            minLength={10}
            maxLength={2000}
            required
            value={review}
            onChange={(e) => setReview(e.target.value)}
            placeholder="Write your take. The AI will read the sentiment and assign a ranking."
          />
        </label>

        {needsManual && (
          <label className="field">
            <span>AI is unavailable. Choose a ranking manually.</span>
            <select className="input" required value={manualRanking} onChange={(e) => setManualRanking(e.target.value)}>
              <option value="">Select ranking…</option>
              {rankings.map((r) => (
                <option key={r.ranking_name} value={r.ranking_name}>
                  {r.ranking_name}
                </option>
              ))}
            </select>
          </label>
        )}

        {status.error && <p className="alert">{status.error}</p>}
        {status.message && <p className="success">{status.message}</p>}

        <div className="form-actions">
          <button className="btn btn-primary" disabled={status.saving}>
            {status.saving ? 'Ranking with AI…' : 'Save & rank review'}
          </button>
          <button type="button" className="btn btn-danger" onClick={onDelete}>
            Delete movie
          </button>
        </div>
      </form>
    </div>
  )
}
