import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { errorMessage, movieApi } from '../api/client'
import MovieCard from '../components/MovieCard.jsx'
import Spinner from '../components/Spinner.jsx'
import { useAuth } from '../context/auth'

function formatTrainedAt(value) {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.valueOf()) ? '' : date.toLocaleDateString()
}

export default function Recommended() {
  const { user } = useAuth()
  const [reloadKey, setReloadKey] = useState(0)
  const [state, setState] = useState({ loading: true, error: '', recs: [], model: '', trainedAt: '' })

  useEffect(() => {
    let cancelled = false
    movieApi
      .recommendations(12)
      .then(({ recommendations, model, trained_at }) => {
        if (!cancelled) {
          setState({ loading: false, error: '', recs: recommendations, model, trainedAt: trained_at })
        }
      })
      .catch((err) => {
        if (!cancelled) setState({ loading: false, error: errorMessage(err), recs: [], model: '', trainedAt: '' })
      })
    return () => {
      cancelled = true
    }
  }, [reloadKey])

  const reload = () => {
    setState((s) => ({ ...s, loading: true, error: '' }))
    setReloadKey((k) => k + 1)
  }

  const trained = formatTrainedAt(state.trainedAt)

  return (
    <>
      <header className="page-header">
        <div>
          <p className="eyebrow">Picked for {user?.first_name}</p>
          <h1>Your recommendations</h1>
          <p className="muted">
            Our own recommendation model places you in the same latent space as the movies, using your favourite genres
            ({user?.favourite_genres?.map((g) => g.genre_name).join(', ') || 'none yet'}), what you've watched, and our
            critics' rankings. <Link to="/profile">Update preferences</Link>
          </p>
        </div>
        <div className="page-actions">
          {state.model && (
            <span className="source-pill model" title={trained ? `Trained ${trained}` : undefined}>
              ⚙ {state.model}
            </span>
          )}
          <button className="btn btn-ghost btn-sm" onClick={reload} disabled={state.loading}>
            ↻ Refresh
          </button>
        </div>
      </header>

      {state.error && <p className="alert">{state.error}</p>}
      {state.loading ? (
        <Spinner label="Scoring the catalogue for you…" />
      ) : state.recs.length === 0 ? (
        <p className="empty">No recommendations yet. Try picking a few favourite genres.</p>
      ) : (
        <div className="movie-grid">
          {state.recs.map((r) => (
            <MovieCard key={r.movie.imdb_id} movie={r.movie} reason={r.reason} />
          ))}
        </div>
      )}
    </>
  )
}
