import { Route, Routes } from 'react-router-dom'
import Navbar from './components/Navbar.jsx'
import ProtectedRoute from './components/ProtectedRoute.jsx'
import AddMovie from './pages/AddMovie.jsx'
import AdminReview from './pages/AdminReview.jsx'
import Home from './pages/Home.jsx'
import Login from './pages/Login.jsx'
import NotFound from './pages/NotFound.jsx'
import Profile from './pages/Profile.jsx'
import Recommended from './pages/Recommended.jsx'
import Register from './pages/Register.jsx'
import Watch from './pages/Watch.jsx'

export default function App() {
  return (
    <>
      <Navbar />
      <main className="container">
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/login" element={<Login />} />
          <Route path="/register" element={<Register />} />

          <Route element={<ProtectedRoute />}>
            <Route path="/watch/:imdbId" element={<Watch />} />
            <Route path="/recommended" element={<Recommended />} />
            <Route path="/profile" element={<Profile />} />
          </Route>

          <Route element={<ProtectedRoute adminOnly />}>
            <Route path="/admin/movies/new" element={<AddMovie />} />
            <Route path="/admin/review/:imdbId" element={<AdminReview />} />
          </Route>

          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>
      <footer className="footer">S-Watch · Go · Gin · React · MongoDB · OpenAI</footer>
    </>
  )
}
