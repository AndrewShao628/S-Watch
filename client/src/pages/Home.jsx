import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { errorMessage, movieApi } from '../api/client'
import MovieCard from '../components/MovieCard.jsx'
import Spinner from '../components/Spinner.jsx'
import { useAuth } from '../context/auth'

const PAGE_SIZE = 24

export default function Home() {
  const { isAuthenticated, user } = useAuth()
  const [params, setParams] = useSearchParams()
  const q = params.get('q') ?? ''
  const genre = params.get('genre') ?? ''
  const page = Math.max(1, Number(params.get('page')) || 1)

  const [genres, setGenres] = useState([])
  const [search, setSearch] = useState(q)
  // Results are tagged with the query they belong to, so loading is derived rather than stored.
  const queryKey = JSON.stringify([q, genre, page])
  const [result, setResult] = useState({ key: null, data: { movies: [], total: 0 }, error: '' })
  const loading = result.key !== queryKey
  const { data, error } = result

  useEffect(() => {
    movieApi.genres().then(setGenres).catch(() => {})
  }, [])

  // Debounce the search box into the URL.
  useEffect(() => {
    const t = setTimeout(() => {
      if (search === q) return
      const next = new URLSearchParams(params)
      if (search) next.set('q', search)
      else next.delete('q')
      next.delete('page')
      setParams(next, { replace: true })
    }, 300)
    return () => clearTimeout(t)
  }, [search, q, params, setParams])

  useEffect(() => {
    let cancelled = false
    movieApi
      .list({ q: q || undefined, genre: genre || undefined, page, limit: PAGE_SIZE })
      .then((res) => !cancelled && setResult({ key: queryKey, data: res, error: '' }))
      .catch((err) => !cancelled && setResult({ key: queryKey, data: { movies: [], total: 0 }, error: errorMessage(err) }))
    return () => {
      cancelled = true
    }
  }, [q, genre, page, queryKey])

  const setFilter = (key, value) => {
    const next = new URLSearchParams(params)
    if (value) next.set(key, value)
    else next.delete(key)
    if (key !== 'page') next.delete('page')
    setParams(next)
  }

  const featured = !q && !genre && page === 1 ? data.movies[0] : null
  const totalPages = Math.max(1, Math.ceil(data.total / PAGE_SIZE))

  return (
    <>
      {featured && (
        <section className="hero" style={{ '--hero-img': `url(${featured.poster_path})` }}>
          <div className="hero-content">
            <p className="eyebrow">{isAuthenticated ? `Welcome back, ${user.first_name}` : 'Now streaming'}</p>
            <h1>{featured.title}</h1>
            <p className="hero-overview">{featured.overview}</p>
            <div className="hero-actions">
              <Link to={`/watch/${featured.imdb_id}`} className="btn btn-primary">▶ Play</Link>
              <Link to={isAuthenticated ? '/recommended' : '/register'} className="btn btn-ghost">
                {isAuthenticated ? '✨ Get AI picks' : 'Join free for AI picks'}
              </Link>
            </div>
          </div>
        </section>
      )}

      <section className="toolbar">
        <input
          type="search"
          className="input search"
          placeholder="Search movies…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          aria-label="Search movies"
        />
        <div className="chips scroll" role="group" aria-label="Filter by genre">
          <button className={`chip ${!genre ? 'active' : ''}`} onClick={() => setFilter('genre', '')}>
            All
          </button>
          {genres.map((g) => (
            <button
              key={g.genre_id}
              className={`chip ${genre === String(g.genre_id) ? 'active' : ''}`}
              onClick={() => setFilter('genre', String(g.genre_id))}
            >
              {g.genre_name}
            </button>
          ))}
        </div>
      </section>

      {!loading && error && <p className="alert">{error}</p>}
      {loading ? (
        <Spinner label="Loading movies…" />
      ) : data.movies.length === 0 ? (
        <p className="empty">No movies match your filters.</p>
      ) : (
        <>
          <div className="movie-grid">
            {data.movies.map((m) => (
              <MovieCard key={m.imdb_id} movie={m} />
            ))}
          </div>
          {totalPages > 1 && (
            <nav className="pagination" aria-label="Pagination">
              <button className="btn btn-ghost btn-sm" disabled={page <= 1} onClick={() => setFilter('page', String(page - 1))}>
                ← Prev
              </button>
              <span>
                Page {page} of {totalPages}
              </span>
              <button
                className="btn btn-ghost btn-sm"
                disabled={page >= totalPages}
                onClick={() => setFilter('page', String(page + 1))}
              >
                Next →
              </button>
            </nav>
          )}
        </>
      )}
    </>
  )
}
