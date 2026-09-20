import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { adminApi, errorMessage, movieApi } from '../api/client'
import RankingBadge from '../components/RankingBadge.jsx'
import Spinner from '../components/Spinner.jsx'

export default function AdminReview() {
  const { imdbId } = useParams()
  const navigate = useNavigate()
  const [movie, setMovie] = useState(null)
  const [rankings, setRankings] = useState([])
  const [review, setReview] = useState('')
  const [override, setOverride] = useState('')
  const [prediction, setPrediction] = useState(null)
  const [status, setStatus] = useState({ saving: false, previewing: false, error: '', message: '' })

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

  const preview = async () => {
    setStatus((s) => ({ ...s, previewing: true, error: '', message: '' }))
    try {
      const { prediction: p } = await adminApi.previewReview(review)
      setPrediction(p)
      setStatus((s) => ({ ...s, previewing: false }))
    } catch (err) {
      setPrediction(null)
      setStatus((s) => ({ ...s, previewing: false, error: errorMessage(err) }))
    }
  }

  const onSubmit = async (e) => {
    e.preventDefault()
    setStatus((s) => ({ ...s, saving: true, error: '', message: '' }))
    try {
      const { movie: updated, ranking_source, confidence } = await adminApi.updateReview(
        imdbId,
        review,
        override || undefined,
      )
      setMovie(updated)
      setPrediction(null)
      setStatus({
        saving: false,
        previewing: false,
        error: '',
        message:
          ranking_source === 'model'
            ? `The model ranked this review "${updated.ranking.ranking_name}" (${Math.round(confidence * 100)}% confident).`
            : `Saved with your ranking "${updated.ranking.ranking_name}".`,
      })
    } catch (err) {
      setStatus({ saving: false, previewing: false, error: errorMessage(err), message: '' })
    }
  }

  const onDelete = async () => {
    if (!window.confirm(`Delete "${movie.title}" from the catalogue? This cannot be undone.`)) return
    try {
      await adminApi.deleteMovie(imdbId)
      navigate('/', { replace: true })
    } catch (err) {
      setStatus({ saving: false, previewing: false, error: errorMessage(err), message: '' })
    }
  }

  if (!movie) return status.error ? <p className="alert">{status.error}</p> : <Spinner />

  const sortedProbabilities = prediction
    ? [...rankings]
        .map((r) => ({ ...r, probability: prediction.probabilities?.[r.ranking_name] ?? 0 }))
        .sort((a, b) => b.ranking_value - a.ranking_value)
    : []

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
            placeholder="Write your take. Our review classifier reads the sentiment and suggests a ranking."
          />
        </label>

        <div className="form-actions">
          <button type="button" className="btn btn-ghost btn-sm" onClick={preview} disabled={review.length < 10 || status.previewing}>
            {status.previewing ? 'Classifying…' : '⚙ Preview ranking'}
          </button>
        </div>

        {prediction && (
          <div className="prediction">
            <p>
              The model predicts <strong>{prediction.ranking.ranking_name}</strong> ·{' '}
              {Math.round(prediction.confidence * 100)}% confident
            </p>
            <ul className="prob-bars">
              {sortedProbabilities.map((r) => (
                <li key={r.ranking_name}>
                  <span className="prob-label">{r.ranking_name}</span>
                  <span className="prob-track">
                    <span className="prob-fill" style={{ width: `${Math.round(r.probability * 100)}%` }} />
                  </span>
                  <span className="prob-value">{Math.round(r.probability * 100)}%</span>
                </li>
              ))}
            </ul>
            <p className="muted small">
              The classifier picks the exact level about 4 times in 10, and lands within one level about 8 times in 10.
              Override it below if it reads your review wrong.
            </p>
          </div>
        )}

        <label className="field">
          <span>Ranking</span>
          <select className="input" value={override} onChange={(e) => setOverride(e.target.value)}>
            <option value="">Let the model decide</option>
            {rankings.map((r) => (
              <option key={r.ranking_name} value={r.ranking_name}>
                Override: {r.ranking_name}
              </option>
            ))}
          </select>
        </label>

        {status.error && <p className="alert">{status.error}</p>}
        {status.message && <p className="success">{status.message}</p>}

        <div className="form-actions">
          <button className="btn btn-primary" disabled={status.saving}>
            {status.saving ? 'Saving…' : 'Save & rank review'}
          </button>
          <button type="button" className="btn btn-danger" onClick={onDelete}>
            Delete movie
          </button>
        </div>
      </form>
    </div>
  )
}
