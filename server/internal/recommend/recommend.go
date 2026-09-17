// Package recommend implements the weighted content-based ranking used to
// pre-score movies before (optional) LLM re-ranking.
package recommend

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"swatch/internal/models"
)

// Weights tune how much each signal contributes to the final score.
type Weights struct {
	Critic          float64 // admin review ranking
	FavouriteGenres float64 // overlap with genres the user picked
	History         float64 // affinity learned from watch history
	RecentPenalty   float64 // subtracted when the user watched the title recently
}

var DefaultWeights = Weights{Critic: 0.45, FavouriteGenres: 0.35, History: 0.20, RecentPenalty: 0.35}

const (
	maxRanking = 5.0
	// Unranked movies get a neutral critic score instead of being buried.
	neutralCritic = 0.5
	// Watch-history signal decays with this half-life.
	historyHalfLife = 14 * 24 * time.Hour
	recentWindow    = 30 * 24 * time.Hour
)

// Rank scores every movie for the user and returns them best-first.
func Rank(user *models.User, movies []models.Movie, w Weights, now time.Time) []models.Recommendation {
	favourites := make(map[int]string, len(user.FavouriteGenres))
	for _, g := range user.FavouriteGenres {
		favourites[g.GenreID] = g.GenreName
	}
	affinity, recent := historySignals(user.WatchHistory, movies, now)

	recs := make([]models.Recommendation, 0, len(movies))
	for _, m := range movies {
		critic := neutralCritic
		if m.Ranking.RankingValue > models.NotRankedValue {
			critic = float64(m.Ranking.RankingValue) / maxRanking
		}

		var matched []string
		for _, g := range m.Genre {
			if name, ok := favourites[g.GenreID]; ok {
				matched = append(matched, name)
			}
		}
		fav := 0.0
		if len(matched) > 0 && len(m.Genre) > 0 {
			// Any match is a strong signal; covering more of the movie's genres helps further.
			fav = 0.6 + 0.4*float64(len(matched))/float64(len(m.Genre))
		}

		hist := 0.0
		for _, g := range m.Genre {
			hist = max(hist, affinity[g.GenreID])
		}

		score := w.Critic*critic + w.FavouriteGenres*fav + w.History*hist
		if recent[m.ImdbID] {
			score -= w.RecentPenalty
		}

		recs = append(recs, models.Recommendation{
			Movie:  m,
			Score:  round2(score),
			Reason: reason(m, matched, hist, recent[m.ImdbID]),
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

// historySignals builds a time-decayed, normalised [0,1] affinity per genre
// and the set of titles watched within the recent window.
func historySignals(history []models.WatchEntry, movies []models.Movie, now time.Time) (map[int]float64, map[string]bool) {
	byID := make(map[string]models.Movie, len(movies))
	for _, m := range movies {
		byID[m.ImdbID] = m
	}

	affinity := make(map[int]float64)
	recent := make(map[string]bool)
	peak := 0.0
	for _, h := range history {
		age := now.Sub(h.WatchedAt)
		if age < recentWindow {
			recent[h.ImdbID] = true
		}
		m, ok := byID[h.ImdbID]
		if !ok {
			continue
		}
		decay := 1.0 / (1.0 + float64(age)/float64(historyHalfLife))
		for _, g := range m.Genre {
			affinity[g.GenreID] += decay
			peak = max(peak, affinity[g.GenreID])
		}
	}
	if peak > 0 {
		for id := range affinity {
			affinity[id] /= peak
		}
	}
	return affinity, recent
}

func reason(m models.Movie, matched []string, hist float64, recentlyWatched bool) string {
	var parts []string
	if len(matched) > 0 {
		parts = append(parts, fmt.Sprintf("matches your love of %s", joinNatural(matched)))
	}
	if hist >= 0.5 {
		parts = append(parts, "similar to what you've been watching")
	}
	if m.Ranking.RankingValue >= 4 {
		parts = append(parts, fmt.Sprintf("rated %s by our critics", strings.ToLower(m.Ranking.RankingName)))
	}
	if len(parts) == 0 {
		parts = append(parts, "a popular pick to broaden your horizons")
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
