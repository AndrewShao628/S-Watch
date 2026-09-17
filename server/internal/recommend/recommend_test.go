package recommend

import (
	"testing"
	"time"

	"swatch/internal/models"
)

var (
	drama  = models.Genre{GenreID: 6, GenreName: "Drama"}
	scifi  = models.Genre{GenreID: 12, GenreName: "Sci-Fi"}
	horror = models.Genre{GenreID: 8, GenreName: "Horror"}
)

func movie(id string, rank int, genres ...models.Genre) models.Movie {
	return models.Movie{ImdbID: id, Title: id, Genre: genres, Ranking: models.Ranking{RankingValue: rank, RankingName: "x"}}
}

func TestRankPrefersFavouriteGenresAndCriticScore(t *testing.T) {
	user := &models.User{FavouriteGenres: []models.Genre{scifi}}
	movies := []models.Movie{
		movie("horror-excellent", 5, horror),
		movie("scifi-good", 4, scifi),
		movie("scifi-bad", 2, scifi),
	}

	got := Rank(user, movies, DefaultWeights, time.Now())

	if got[0].Movie.ImdbID != "scifi-good" {
		t.Fatalf("expected scifi-good first, got %s", got[0].Movie.ImdbID)
	}
	if got[len(got)-1].Movie.ImdbID != "horror-excellent" && got[len(got)-1].Movie.ImdbID != "scifi-bad" {
		t.Fatalf("unexpected last result %s", got[len(got)-1].Movie.ImdbID)
	}
}

func TestRankPenalisesRecentlyWatchedAndLearnsFromHistory(t *testing.T) {
	now := time.Now()
	user := &models.User{
		WatchHistory: []models.WatchEntry{{ImdbID: "drama-watched", WatchedAt: now.Add(-time.Hour)}},
	}
	movies := []models.Movie{
		movie("drama-watched", 5, drama),
		movie("drama-new", 4, drama),
		movie("scifi-new", 4, scifi),
	}

	got := Rank(user, movies, DefaultWeights, now)

	if got[0].Movie.ImdbID != "drama-new" {
		t.Fatalf("expected history affinity to surface drama-new first, got %s", got[0].Movie.ImdbID)
	}
	if got[len(got)-1].Movie.ImdbID != "drama-watched" {
		t.Fatalf("expected recently watched title last, got %s", got[len(got)-1].Movie.ImdbID)
	}
}

func TestUnrankedMoviesGetNeutralCriticScore(t *testing.T) {
	user := &models.User{}
	got := Rank(user, []models.Movie{movie("unranked", 0, drama), movie("terrible", 1, drama)}, DefaultWeights, time.Now())
	if got[0].Movie.ImdbID != "unranked" {
		t.Fatalf("expected unranked above terrible, got %s", got[0].Movie.ImdbID)
	}
}
