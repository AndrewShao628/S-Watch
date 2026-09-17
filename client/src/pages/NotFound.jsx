import { Link } from 'react-router-dom'

export default function NotFound() {
  return (
    <div className="empty">
      <h1>404</h1>
      <p>This page wandered off the reel.</p>
      <Link to="/" className="btn btn-primary">Back to browse</Link>
    </div>
  )
}
