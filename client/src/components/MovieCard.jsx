import { Link } from 'react-router-dom'
import { useAuth } from '../context/auth'
import RankingBadge from './RankingBadge.jsx'

export default function MovieCard({ movie, reason }) {
  const { isAdmin } = useAuth()

  return (
    <article className="movie-card">
      <Link to={`/watch/${movie.imdb_id}`} className="poster" aria-label={`Watch ${movie.title}`}>
        <img src={movie.poster_path} alt="" loading="lazy" />
        <span className="play-overlay" aria-hidden="true">▶</span>
        <RankingBadge ranking={movie.ranking} />
      </Link>
      <div className="movie-card-body">
        <h3 className="movie-title" title={movie.title}>{movie.title}</h3>
        <p className="movie-meta">
          {movie.year} · {movie.genre?.map((g) => g.genre_name).join(', ')}
        </p>
        {reason && <p className="movie-reason">✨ {reason}</p>}
        {isAdmin && (
          <Link to={`/admin/review/${movie.imdb_id}`} className="link-sm">
            ✎ Review
          </Link>
        )}
      </div>
    </article>
  )
}
