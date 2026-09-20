package recommend

import (
	"strings"
	"testing"
	"time"

	"swatch/internal/model"
	"swatch/internal/models"
)

var (
	scifi     = models.Genre{GenreID: 12, GenreName: "Sci-Fi"}
	horror    = models.Genre{GenreID: 8, GenreName: "Horror"}
	drama     = models.Genre{GenreID: 6, GenreName: "Drama"}
	excellent = models.Ranking{RankingValue: 5, RankingName: "Excellent"}
	bad       = models.Ranking{RankingValue: 2, RankingName: "Bad"}
)

func recommender(t *testing.T) *model.Recommender {
	t.Helper()
	engine, err := model.Load("")
	if err != nil {
		t.Fatalf("load model: %v", err)
	}
	return engine.Recommender
}

// Catalogue titles that exist in the trained artifact.
func catalogue() []models.Movie {
	return []models.Movie{
		{ImdbID: "tt0133093", Title: "The Matrix", Year: 1999, Genre: []models.Genre{scifi}, Ranking: excellent},
		{ImdbID: "tt1856101", Title: "Blade Runner 2049", Year: 2017, Genre: []models.Genre{scifi}, Ranking: excellent},
		{ImdbID: "tt5052448", Title: "Get Out", Year: 2017, Genre: []models.Genre{horror}, Ranking: excellent},
		{ImdbID: "tt0111161", Title: "The Shawshank Redemption", Year: 1994, Genre: []models.Genre{drama}, Ranking: excellent},
	}
}

func find(recs []models.Recommendation, imdbID string) (models.Recommendation, bool) {
	for _, r := range recs {
		if r.Movie.ImdbID == imdbID {
			return r, true
		}
	}
	return models.Recommendation{}, false
}

func TestRankPenalisesRecentlyWatched(t *testing.T) {
	now := time.Now()
	rec := recommender(t)
	user := &models.User{
		FavouriteGenres: []models.Genre{scifi},
		WatchHistory:    []models.WatchEntry{{ImdbID: "tt0133093", WatchedAt: now.Add(-time.Hour)}},
	}

	ranked := Rank(user, catalogue(), rec, DefaultWeights, now)

	watched, ok := find(ranked, "tt0133093")
	if !ok {
		t.Fatal("watched movie missing from results")
	}
	if ranked[0].Movie.ImdbID == "tt0133093" {
		t.Error("a title watched an hour ago should not be the top pick")
	}
	if !strings.Contains(watched.Reason, "watched this recently") {
		t.Errorf("reason should mention the recent view, got %q", watched.Reason)
	}
}

func TestRankExplainsPicksFromWatchHistory(t *testing.T) {
	now := time.Now()
	user := &models.User{WatchHistory: []models.WatchEntry{{ImdbID: "tt0133093", WatchedAt: now.Add(-48 * time.Hour)}}}

	ranked := Rank(user, catalogue(), recommender(t), DefaultWeights, now)

	var explained int
	for _, r := range ranked {
		if strings.Contains(strings.ToLower(r.Reason), "viewers who watched the matrix") {
			explained++
		}
		if !strings.HasSuffix(r.Reason, ".") {
			t.Errorf("reason is not a sentence: %q", r.Reason)
		}
	}
	if explained == 0 {
		t.Error("expected at least one pick to cite the watched title")
	}
}

func TestRankFavoursCriticScoreWhenNothingIsKnown(t *testing.T) {
	movies := catalogue()
	movies[2].Ranking = bad // Get Out

	ranked := Rank(&models.User{}, movies, recommender(t), DefaultWeights, time.Now())

	poorlyRated, _ := find(ranked, "tt5052448")
	if ranked[0].Movie.ImdbID == "tt5052448" {
		t.Error("a badly reviewed title should not lead for a viewer with no profile")
	}
	if poorlyRated.Score >= ranked[0].Score {
		t.Errorf("bad review scored %.2f, top pick scored %.2f", poorlyRated.Score, ranked[0].Score)
	}
}

func TestRankHandlesEmptyCatalogue(t *testing.T) {
	if got := Rank(&models.User{}, nil, recommender(t), DefaultWeights, time.Now()); len(got) != 0 {
		t.Errorf("expected no recommendations, got %d", len(got))
	}
}
