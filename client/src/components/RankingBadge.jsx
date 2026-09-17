export default function RankingBadge({ ranking, large = false }) {
  if (!ranking || !ranking.ranking_value) return null
  return (
    <span className={`ranking-badge rank-${ranking.ranking_value} ${large ? 'large' : ''}`}>
      {'★'.repeat(ranking.ranking_value)}
      <span className="ranking-name">{ranking.ranking_name}</span>
    </span>
  )
}
