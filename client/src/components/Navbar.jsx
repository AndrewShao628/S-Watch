import { useState } from 'react'
import { Link, NavLink, useNavigate } from 'react-router-dom'
import { useAuth } from '../context/auth'

export default function Navbar() {
  const { user, isAuthenticated, isAdmin, logout } = useAuth()
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const close = () => setOpen(false)

  const handleLogout = async () => {
    close()
    await logout()
    navigate('/')
  }

  return (
    <header className="navbar">
      <div className="navbar-inner">
        <Link to="/" className="brand" onClick={close}>
          <span className="brand-mark" aria-hidden="true">▶</span>
          S-Watch
        </Link>

        <button
          className="nav-toggle"
          aria-label="Toggle navigation"
          aria-expanded={open}
          onClick={() => setOpen((o) => !o)}
        >
          ☰
        </button>

        <nav className={`nav-links ${open ? 'open' : ''}`}>
          <NavLink to="/" end onClick={close}>Browse</NavLink>
          {isAuthenticated && <NavLink to="/recommended" onClick={close}>For You ✨</NavLink>}
          {isAdmin && <NavLink to="/admin/movies/new" onClick={close}>Add Movie</NavLink>}

          <span className="nav-spacer" />

          {isAuthenticated ? (
            <>
              <NavLink to="/profile" onClick={close} className="nav-user">
                <span className="avatar" aria-hidden="true">{user?.first_name?.[0]?.toUpperCase() ?? '?'}</span>
                {user?.first_name}
              </NavLink>
              <button className="btn btn-ghost btn-sm" onClick={handleLogout}>Log out</button>
            </>
          ) : (
            <>
              <NavLink to="/login" onClick={close}>Log in</NavLink>
              <Link to="/register" className="btn btn-primary btn-sm" onClick={close}>Sign up</Link>
            </>
          )}
        </nav>
      </div>
    </header>
  )
}
