// Package recommend blends the trained matrix-factorisation model with editorial
// and freshness signals, and explains each pick.
package recommend

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"swatch/internal/model"
	"swatch/internal/models"
)

// Weights tune how much each signal contributes to the final score.
type Weights struct {
	Model  float64 // trained model affinity
	Critic float64 // admin review ranking
	// GenreMatch gives the viewer's stated favourite genres a direct say. The
	// latent space alone under-serves them: with only 610 training viewers, a
	// recent or niche title has too few ratings for its vector to carry its
	// genre, so a horror fan was never shown the catalogue's horror film.
	GenreMatch float64
	// PopularityDebias is SUBTRACTED. Latent scores already lean towards widely
	// watched titles, so without this a sci-fi fan is served the same handful of
	// classics as everyone else. Measured on the leave-one-out protocol, 0.30
	// costs no hit rate (0.708 either way) and 1.6% of NDCG.
	PopularityDebias float64
	RecentPenalty    float64 // subtracted when the viewer watched it recently
}

var DefaultWeights = Weights{Model: 0.60, Critic: 0.25, GenreMatch: 0.15, PopularityDebias: 0.30, RecentPenalty: 0.35}

const (
	maxRanking = 5.0
	// Unranked movies get a neutral critic score instead of being buried.
	neutralCritic = 0.5
	recentWindow  = 30 * 24 * time.Hour
)

// Rank scores every movie for the viewer and returns them best-first.
func Rank(
	user *models.User,
	movies []models.Movie,
	rec *model.Recommender,
	w Weights,
	now time.Time,
) []models.Recommendation {
	catalogue := make(map[string]models.Movie, len(movies))
	for _, m := range movies {
		catalogue[m.ImdbID] = m
	}

	prefs := rec.Preferences(user, catalogue, now)
	userVector, err := rec.UserVector(prefs)
	if err != nil {
		// A viewer with no history and no favourite genres has nothing to fold in;
		// fall back to editorial signals only.
		userVector = nil
	}

	recent := recentlyWatched(user.WatchHistory, now)
	favourites := make(map[int]string, len(user.FavouriteGenres))
	for _, g := range user.FavouriteGenres {
		favourites[g.GenreID] = g.GenreName
	}

	raw := make([]float64, len(movies))
	for i, m := range movies {
		if userVector != nil {
			raw[i] = rec.Score(userVector, m)
		}
	}
	normalised := minMax(raw)

	recs := make([]models.Recommendation, 0, len(movies))
	for i, m := range movies {
		critic := neutralCritic
		if m.Ranking.RankingValue > models.NotRankedValue {
			critic = float64(m.Ranking.RankingValue) / maxRanking
		}

		score := w.Model*normalised[i] + w.Critic*critic +
			w.GenreMatch*genreMatch(m, favourites) -
			w.PopularityDebias*rec.Popularity(m.ImdbID)
		if recent[m.ImdbID] {
			score -= w.RecentPenalty
		}

		recs = append(recs, models.Recommendation{
			Movie:  m,
			Score:  round2(score),
			Reason: reason(m, rec, prefs, catalogue, favourites, recent[m.ImdbID]),
		})
	}

	sort.SliceStable(recs, func(i, j int) bool {
		if recs[i].Score != recs[j].Score {
			return recs[i].Score > recs[j].Score
		}
		return recs[i].Movie.Title < recs[j].Movie.Title
	})
	return recs
}

// minMax rescales model scores into [0,1] so they can be blended with the other
// signals. Latent dot products have no fixed range on their own.
func minMax(values []float64) []float64 {
	out := make([]float64, len(values))
	if len(values) == 0 {
		return out
	}
	lo, hi := values[0], values[0]
	for _, v := range values {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	spread := hi - lo
	for i, v := range values {
		if spread == 0 {
			out[i] = 0.5
			continue
		}
		out[i] = (v - lo) / spread
	}
	return out
}

// genreMatch scores how well a movie covers the viewer's favourite genres. Any
// match counts for most of the score; covering more of the movie's genres adds
// the rest.
func genreMatch(m models.Movie, favourites map[int]string) float64 {
	if len(m.Genre) == 0 {
		return 0
	}
	matched := 0
	for _, g := range m.Genre {
		if _, ok := favourites[g.GenreID]; ok {
			matched++
		}
	}
	if matched == 0 {
		return 0
	}
	return 0.6 + 0.4*float64(matched)/float64(len(m.Genre))
}

func recentlyWatched(history []models.WatchEntry, now time.Time) map[string]bool {
	recent := make(map[string]bool)
	for _, h := range history {
		if now.Sub(h.WatchedAt) < recentWindow {
			recent[h.ImdbID] = true
		}
	}
	return recent
}

// reason explains a pick using the model's own nearest-neighbour signal where it
// can, and editorial signals otherwise.
func reason(
	m models.Movie,
	rec *model.Recommender,
	prefs []model.Preference,
	catalogue map[string]models.Movie,
	favourites map[int]string,
	recentlyWatched bool,
) string {
	var parts []string

	if id, _ := rec.ClosestWatched(m, prefs); id != "" {
		if watched, ok := catalogue[id]; ok {
			parts = append(parts, fmt.Sprintf("viewers who watched %s tend to like this", watched.Title))
		}
	}

	var matched []string
	for _, g := range m.Genre {
		if name, ok := favourites[g.GenreID]; ok {
			matched = append(matched, name)
		}
	}
	if len(matched) > 0 {
		parts = append(parts, fmt.Sprintf("it matches your love of %s", joinNatural(matched)))
	}
	if m.Ranking.RankingValue >= 4 {
		parts = append(parts, fmt.Sprintf("our critics rated it %s", strings.ToLower(m.Ranking.RankingName)))
	}
	if len(parts) == 0 {
		parts = append(parts, "it's a popular pick to broaden your horizons")
	}

	s := joinNatural(parts)
	if recentlyWatched {
		s += " (you watched this recently)"
	}
	return strings.ToUpper(s[:1]) + s[1:] + "."
}

func joinNatural(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
	}
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
