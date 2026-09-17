import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { errorMessage, movieApi } from '../api/client'
import MovieCard from '../components/MovieCard.jsx'
import Spinner from '../components/Spinner.jsx'
import { useAuth } from '../context/auth'

export default function Recommended() {
  const { user } = useAuth()
  const [reloadKey, setReloadKey] = useState(0)
  const [state, setState] = useState({ loading: true, error: '', recs: [], source: '' })

  useEffect(() => {
    let cancelled = false
    movieApi
      .recommendations(12)
      .then(({ recommendations, source }) => !cancelled && setState({ loading: false, error: '', recs: recommendations, source }))
      .catch((err) => !cancelled && setState({ loading: false, error: errorMessage(err), recs: [], source: '' }))
    return () => {
      cancelled = true
    }
  }, [reloadKey])

  const reload = () => {
    setState((s) => ({ ...s, loading: true, error: '' }))
    setReloadKey((k) => k + 1)
  }

  return (
    <>
      <header className="page-header">
        <div>
          <p className="eyebrow">Picked for {user?.first_name}</p>
          <h1>Your recommendations</h1>
          <p className="muted">
            Based on your favourite genres ({user?.favourite_genres?.map((g) => g.genre_name).join(', ') || 'none yet'}),
            what you've watched, and our critics' rankings. <Link to="/profile">Update preferences</Link>
          </p>
        </div>
        <div className="page-actions">
          {state.source && (
            <span className={`source-pill ${state.source}`}>
              {state.source === 'ai' ? '✨ AI-curated' : '⚙ Smart ranking'}
            </span>
          )}
          <button className="btn btn-ghost btn-sm" onClick={reload} disabled={state.loading}>
            ↻ Refresh
          </button>
        </div>
      </header>

      {state.error && <p className="alert">{state.error}</p>}
      {state.loading ? (
        <Spinner label="Asking the AI for your picks…" />
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
