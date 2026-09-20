package model

import (
	"math"
	"testing"
	"time"

	"swatch/internal/models"
)

func load(t *testing.T) *Engine {
	t.Helper()
	engine, err := Load("")
	if err != nil {
		t.Fatalf("load embedded artifacts: %v", err)
	}
	return engine
}

// The Python trainer exports the probabilities scikit-learn produced for a few
// reviews. If the ported TF-IDF transform drifts, this fails.
func TestClassifierMatchesScikitLearn(t *testing.T) {
	clf := load(t).Classifier
	if len(clf.ParityFixtures) == 0 {
		t.Fatal("artifact has no parity fixtures")
	}

	for _, fixture := range clf.ParityFixtures {
		got, err := clf.Classify(fixture.Text)
		if err != nil {
			t.Fatalf("classify %q: %v", fixture.Text, err)
		}
		for i, class := range clf.Classes {
			want := fixture.Probabilities[i]
			if diff := math.Abs(got.Probabilities[class.RankingName] - want); diff > 1e-3 {
				t.Errorf("%q: %s probability = %.4f, scikit-learn had %.4f (diff %.4f)",
					fixture.Text, class.RankingName, got.Probabilities[class.RankingName], want, diff)
			}
		}
	}
}

func TestClassifierSeparatesPraiseFromPans(t *testing.T) {
	clf := load(t).Classifier

	praise, err := clf.Classify("A masterful, deeply moving film with two extraordinary performances.")
	if err != nil {
		t.Fatal(err)
	}
	pan, err := clf.Classify("An absolute disaster. Nothing about this works on any level.")
	if err != nil {
		t.Fatal(err)
	}

	if praise.Ranking.RankingValue <= pan.Ranking.RankingValue {
		t.Errorf("praise ranked %d, pan ranked %d", praise.Ranking.RankingValue, pan.Ranking.RankingValue)
	}
	if praise.Confidence <= 0 || praise.Confidence > 1 {
		t.Errorf("confidence out of range: %v", praise.Confidence)
	}
}

func TestClassifierRejectsEmptyReview(t *testing.T) {
	if _, err := load(t).Classifier.Classify("!!! ???"); err == nil {
		t.Error("expected an error for a review with no recognisable words")
	}
}

func movie(id, title string, year int, genres ...models.Genre) models.Movie {
	return models.Movie{ImdbID: id, Title: title, Year: year, Genre: genres}
}

var (
	scifi  = models.Genre{GenreID: 12, GenreName: "Sci-Fi"}
	horror = models.Genre{GenreID: 8, GenreName: "Horror"}
	drama  = models.Genre{GenreID: 6, GenreName: "Drama"}
)

func TestCatalogueMoviesAllHaveVectors(t *testing.T) {
	rec := load(t).Recommender
	// Every seeded movie must resolve, either from training or the projection.
	for _, id := range []string{"tt0111161", "tt0133093", "tt15398776", "tt6710474"} {
		item, ok := rec.Items[id]
		if !ok {
			t.Errorf("%s missing from the artifact", id)
			continue
		}
		if len(item.F) != rec.Factors {
			t.Errorf("%s has %d factors, want %d", id, len(item.F), rec.Factors)
		}
	}
}

func TestProjectionCoversUnseenMovies(t *testing.T) {
	rec := load(t).Recommender
	unseen := movie("tt9999999", "Brand New Film", 2024, scifi)

	vec, trained := rec.Vector(unseen)
	if trained {
		t.Error("a movie that isn't in the artifact should not report as trained")
	}
	if len(vec) != rec.Factors || norm(vec) == 0 {
		t.Fatalf("projection produced a degenerate vector: len=%d norm=%f", len(vec), norm(vec))
	}
}

func TestUserVectorFavoursWatchedNeighbours(t *testing.T) {
	rec := load(t).Recommender
	now := time.Now()

	matrix := movie("tt0133093", "The Matrix", 1999, scifi)
	shawshank := movie("tt0111161", "The Shawshank Redemption", 1994, drama)
	unseen := movie("tt9999999", "Never Rated", 2024, horror)
	catalogue := map[string]models.Movie{matrix.ImdbID: matrix, shawshank.ImdbID: shawshank}

	user := &models.User{WatchHistory: []models.WatchEntry{{ImdbID: matrix.ImdbID, WatchedAt: now.Add(-time.Hour)}}}
	prefs := rec.Preferences(user, catalogue, now)
	if len(prefs) != 1 {
		t.Fatalf("expected one preference, got %d", len(prefs))
	}

	vector, err := rec.UserVector(prefs)
	if err != nil {
		t.Fatalf("fold-in failed: %v", err)
	}
	if rec.Score(vector, matrix) <= rec.Score(vector, unseen) {
		t.Error("the watched title should score above one nobody has rated")
	}

	// The model should connect a recommendation back to what was watched.
	// Shawshank and The Matrix share a large co-watching audience in MovieLens.
	if id, sim := rec.ClosestWatched(shawshank, prefs); id != matrix.ImdbID || sim <= similarityFloor {
		t.Errorf("closest watched = %q (similarity %.3f), want the watched title", id, sim)
	}

	// An unrelated, never-rated title should not claim a bogus connection.
	if id, _ := rec.ClosestWatched(unseen, prefs); id != "" {
		t.Errorf("unrelated title cited %q as a reason", id)
	}
}

func TestUserVectorFromGenresOnly(t *testing.T) {
	rec := load(t).Recommender
	user := &models.User{FavouriteGenres: []models.Genre{scifi}}

	prefs := rec.Preferences(user, map[string]models.Movie{}, time.Now())
	if len(prefs) != 1 {
		t.Fatalf("expected a genre preference, got %d", len(prefs))
	}
	if _, err := rec.UserVector(prefs); err != nil {
		t.Fatalf("cold-start fold-in failed: %v", err)
	}
}

func TestSolveSPD(t *testing.T) {
	a := [][]float64{{4, 1, 0}, {1, 3, 1}, {0, 1, 2}}
	want := []float64{1, 2, 3}
	b := make([]float64, 3)
	for i := range a {
		b[i] = dot(a[i], want)
	}

	got, err := solveSPD(a, b)
	if err != nil {
		t.Fatal(err)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Errorf("x[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}
