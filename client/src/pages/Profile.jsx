import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { errorMessage, movieApi, userApi } from '../api/client'
import GenrePicker from '../components/GenrePicker.jsx'
import Spinner from '../components/Spinner.jsx'
import { useAuth } from '../context/auth'

export default function Profile() {
  const { updateUser } = useAuth()
  const [profile, setProfile] = useState(null)
  const [genres, setGenres] = useState([])
  const [history, setHistory] = useState([])
  const [status, setStatus] = useState({ saving: false, message: '', error: '' })

  useEffect(() => {
    let cancelled = false
    userApi
      .me()
      .then(async (me) => {
        if (cancelled) return
        setProfile(me)
        setGenres(me.favourite_genres ?? [])
        updateUser(me)

        // Resolve the most recent distinct titles in the watch history.
        const ids = [...new Set((me.watch_history ?? []).map((w) => w.imdb_id).reverse())].slice(0, 8)
        const movies = await Promise.all(ids.map((id) => movieApi.get(id).catch(() => null)))
        if (!cancelled) setHistory(movies.filter(Boolean))
      })
      .catch((err) => !cancelled && setStatus((s) => ({ ...s, error: errorMessage(err) })))
    return () => {
      cancelled = true
    }
  }, [updateUser])

  const save = async () => {
    if (genres.length === 0) {
      setStatus({ saving: false, message: '', error: 'Pick at least one genre.' })
      return
    }
    setStatus({ saving: true, message: '', error: '' })
    try {
      const me = await userApi.updateGenres(genres)
      setProfile(me)
      updateUser(me)
      setStatus({ saving: false, message: 'Preferences saved. Your recommendations will update.', error: '' })
    } catch (err) {
      setStatus({ saving: false, message: '', error: errorMessage(err) })
    }
  }

  if (!profile) return status.error ? <p className="alert">{status.error}</p> : <Spinner />

  return (
    <div className="profile">
      <header className="page-header">
        <div>
          <p className="eyebrow">{profile.role === 'ADMIN' ? 'Administrator' : 'Member'}</p>
          <h1>
            {profile.first_name} {profile.last_name}
          </h1>
          <p className="muted">{profile.email}</p>
        </div>
      </header>

      <section className="card">
        <h2>Favourite genres</h2>
        <p className="muted">These drive your personalised recommendations.</p>
        <GenrePicker value={genres} onChange={setGenres} />
        {status.error && <p className="alert">{status.error}</p>}
        {status.message && <p className="success">{status.message}</p>}
        <div className="form-actions">
          <button className="btn btn-primary" onClick={save} disabled={status.saving}>
            {status.saving ? 'Saving…' : 'Save preferences'}
          </button>
          <Link to="/recommended" className="btn btn-ghost">See my picks</Link>
        </div>
      </section>

      <section className="card">
        <h2>Recently watched</h2>
        {history.length === 0 ? (
          <p className="muted">Nothing yet. Press play on something!</p>
        ) : (
          <ul className="history">
            {history.map((m) => (
              <li key={m.imdb_id}>
                <Link to={`/watch/${m.imdb_id}`}>
                  <img src={m.poster_path} alt="" loading="lazy" />
                  <span>{m.title}</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
