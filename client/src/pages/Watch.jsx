import { useEffect, useRef, useState } from 'react'
import ReactPlayer from 'react-player'
import { Link, useParams } from 'react-router-dom'
import { errorMessage, movieApi } from '../api/client'
import RankingBadge from '../components/RankingBadge.jsx'
import Spinner from '../components/Spinner.jsx'
import { useAuth } from '../context/auth'

// Keying by id remounts the player page with fresh state when navigating between movies.
export default function Watch() {
  const { imdbId } = useParams()
  return <WatchMovie key={imdbId} imdbId={imdbId} />
}

function WatchMovie({ imdbId }) {
  const { isAdmin } = useAuth()
  const [movie, setMovie] = useState(null)
  const [error, setError] = useState('')
  const [playerError, setPlayerError] = useState(false)
  const recorded = useRef(false)

  useEffect(() => {
    let cancelled = false
    movieApi
      .get(imdbId)
      .then((m) => !cancelled && setMovie(m))
      .catch((err) => !cancelled && setError(errorMessage(err, 'Movie not found.')))
    return () => {
      cancelled = true
    }
  }, [imdbId])

  // Record a view once per movie when playback actually starts.
  const handleStart = () => {
    if (recorded.current) return
    recorded.current = true
    movieApi.recordWatch(imdbId).catch(() => {
      recorded.current = false
    })
  }

  if (error) {
    return (
      <div className="empty">
        <p>{error}</p>
        <Link to="/" className="btn btn-ghost">Back to browse</Link>
      </div>
    )
  }
  if (!movie) return <Spinner label="Loading movie…" />

  return (
    <div className="watch">
      <div className="player-wrap">
        {playerError ? (
          <div className="player-fallback">
            <p>This video can't be played here.</p>
            <a className="btn btn-ghost" href={`https://www.youtube.com/watch?v=${movie.youtube_id}`} target="_blank" rel="noreferrer">
              Open on YouTube ↗
            </a>
          </div>
        ) : (
          <ReactPlayer
            key={movie.youtube_id}
            src={`https://www.youtube.com/watch?v=${movie.youtube_id}`}
            controls
            playing
            playsInline
            width="100%"
            height="100%"
            onStart={handleStart}
            onPlay={handleStart}
            onError={() => setPlayerError(true)}
          />
        )}
      </div>

      <section className="watch-details">
        <div className="watch-heading">
          <div>
            <h1>{movie.title}</h1>
            <p className="movie-meta">
              {movie.year} · {movie.genre?.map((g) => g.genre_name).join(' · ')}
            </p>
          </div>
          <RankingBadge ranking={movie.ranking} large />
        </div>

        <p className="overview">{movie.overview}</p>

        <div className="card review-card">
          <div className="review-head">
            <h2>Critic's review</h2>
            {isAdmin && (
              <Link to={`/admin/review/${movie.imdb_id}`} className="btn btn-ghost btn-sm">✎ Edit review</Link>
            )}
          </div>
          {movie.admin_review ? (
            <blockquote>{movie.admin_review}</blockquote>
          ) : (
            <p className="muted">No review yet.</p>
          )}
        </div>
      </section>
    </div>
  )
}
