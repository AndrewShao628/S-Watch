import { useEffect, useState } from 'react'
import { errorMessage, movieApi } from '../api/client'

// Multi-select chip list of genres. `value` is an array of {genre_id, genre_name}.
export default function GenrePicker({ value, onChange }) {
  const [genres, setGenres] = useState([])
  const [error, setError] = useState('')

  useEffect(() => {
    movieApi.genres().then(setGenres).catch((err) => setError(errorMessage(err)))
  }, [])

  const selected = new Set(value.map((g) => g.genre_id))
  const toggle = (genre) =>
    onChange(selected.has(genre.genre_id) ? value.filter((g) => g.genre_id !== genre.genre_id) : [...value, genre])

  if (error) return <p className="form-error">{error}</p>

  return (
    <div className="chips" role="group" aria-label="Genres">
      {genres.map((g) => (
        <button
          type="button"
          key={g.genre_id}
          className={`chip ${selected.has(g.genre_id) ? 'active' : ''}`}
          aria-pressed={selected.has(g.genre_id)}
          onClick={() => toggle(g)}
        >
          {g.genre_name}
        </button>
      ))}
    </div>
  )
}
