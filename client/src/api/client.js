import axios from 'axios'

const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080/api').replace(/\/$/, '')
const STORAGE_KEY = 'swatch.auth'
const AUTH_EVENT = 'swatch:auth'

// ---- session storage --------------------------------------------------------

export function loadSession() {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY)) || null
  } catch {
    return null
  }
}

export function saveSession(session) {
  try {
    if (session) localStorage.setItem(STORAGE_KEY, JSON.stringify(session))
    else localStorage.removeItem(STORAGE_KEY)
  } catch {
    // storage unavailable (private mode); the in-memory session still works for this tab
  }
  window.dispatchEvent(new CustomEvent(AUTH_EVENT, { detail: session }))
}

export function onSessionChange(listener) {
  const handler = (e) => listener(e.detail)
  window.addEventListener(AUTH_EVENT, handler)
  return () => window.removeEventListener(AUTH_EVENT, handler)
}

// ---- axios instance ---------------------------------------------------------

export const api = axios.create({ baseURL: API_BASE_URL, timeout: 40_000 })

api.interceptors.request.use((config) => {
  const session = loadSession()
  if (session?.access_token) {
    config.headers.Authorization = `Bearer ${session.access_token}`
  }
  return config
})

let refreshPromise = null

// Concurrent 401s share a single refresh request.
function refreshTokens() {
  if (!refreshPromise) {
    const session = loadSession()
    refreshPromise = (session?.refresh_token
      ? axios.post(`${API_BASE_URL}/auth/refresh`, { refresh_token: session.refresh_token })
      : Promise.reject(new Error('no refresh token'))
    )
      .then(({ data }) => {
        saveSession(data)
        return data
      })
      .catch((err) => {
        saveSession(null)
        throw err
      })
      .finally(() => {
        refreshPromise = null
      })
  }
  return refreshPromise
}

api.interceptors.response.use(
  (res) => res,
  async (error) => {
    const original = error.config
    const isAuthCall = original?.url?.startsWith('/auth/')
    if (error.response?.status === 401 && original && !original._retried && !isAuthCall && loadSession()) {
      original._retried = true
      await refreshTokens()
      return api(original)
    }
    return Promise.reject(error)
  },
)

export function errorMessage(err, fallback = 'Something went wrong. Please try again.') {
  if (err?.code === 'ECONNABORTED') return 'The request timed out. Please try again.'
  if (!err?.response) return 'Cannot reach the S-Watch server.'
  return err.response.data?.error || fallback
}

// ---- endpoints --------------------------------------------------------------

export const authApi = {
  login: (email, password) => api.post('/auth/login', { email, password }).then((r) => r.data),
  register: (payload) => api.post('/auth/register', payload).then((r) => r.data),
  logout: () => api.post('/auth/logout'),
}

export const movieApi = {
  list: (params) => api.get('/movies', { params }).then((r) => r.data),
  get: (imdbId) => api.get(`/movies/${encodeURIComponent(imdbId)}`).then((r) => r.data),
  genres: () => api.get('/genres').then((r) => r.data),
  rankings: () => api.get('/rankings').then((r) => r.data),
  recordWatch: (imdbId) => api.post(`/movies/${encodeURIComponent(imdbId)}/watch`),
  recommendations: (limit) => api.get('/recommendations', { params: { limit } }).then((r) => r.data),
  modelInfo: () => api.get('/model').then((r) => r.data),
}

export const userApi = {
  me: () => api.get('/me').then((r) => r.data),
  updateGenres: (favourite_genres) => api.put('/me/genres', { favourite_genres }).then((r) => r.data),
}

export const adminApi = {
  createMovie: (movie) => api.post('/admin/movies', movie).then((r) => r.data),
  updateReview: (imdbId, admin_review, ranking_name) =>
    api.patch(`/admin/movies/${encodeURIComponent(imdbId)}/review`, { admin_review, ranking_name }).then((r) => r.data),
  previewReview: (admin_review) => api.post('/admin/reviews/preview', { admin_review }).then((r) => r.data),
  deleteMovie: (imdbId) => api.delete(`/admin/movies/${encodeURIComponent(imdbId)}`),
}
