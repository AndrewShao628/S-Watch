package model

import (
	"errors"
	"math"
	"strconv"
	"time"

	"swatch/internal/models"
)

// Recommender is the implicit-feedback matrix factorisation model trained by
// ml/train_recommender.py.
type Recommender struct {
	ArtifactInfo
	Factors int `json:"factors"`
	Params  struct {
		Regularisation float64 `json:"regularisation"`
		Alpha          float64 `json:"alpha"`
	} `json:"params"`

	// YtY over every trained item, needed for the ALS user step.
	YtY   [][]float64            `json:"yty"`
	Items map[string]*ItemVector `json:"items"`

	ContentProjection struct {
		Features  []string    `json:"features"`
		Weights   [][]float64 `json:"weights"` // factors x features
		Intercept []float64   `json:"intercept"`
	} `json:"content_projection"`

	// Popularity-weighted centroid per genre, built from real trained vectors.
	GenreVectors map[string][]float64 `json:"genre_vectors"`
	// Popularity assumed for movies that were never rated in the training data.
	NeutralPopularity float64 `json:"neutral_popularity"`

	// S-Watch genre name -> the MovieLens genres the projection was trained on.
	GenreMap map[string][]string `json:"genre_map"`

	featureIndex map[string]int
}

// ItemVector is one movie's latent factors plus its popularity in the training
// data. F scores candidates and is shrunk towards the content prior when the
// movie has few ratings; R is the raw ALS vector used to build viewer vectors.
type ItemVector struct {
	F    []float64 `json:"f"`
	R    []float64 `json:"r"`
	P    float64   `json:"p"` // popularity, normalised to [0,1]
	N    int       `json:"n"` // rating count in the training data
	Cold bool      `json:"cold"`
}

const (
	// Watch-history confidence decays with this half-life, so recent views matter more.
	historyHalfLife = 21 * 24 * time.Hour
	// A favourite genre counts for less than an actual view.
	genrePreferenceWeight = 0.45
	// Below this cosine similarity we don't claim two movies are related.
	similarityFloor = 0.25
)

func (r *Recommender) prepare() error {
	if r.Factors == 0 || len(r.Items) == 0 {
		return errors.New("artifact has no items or factors")
	}
	if len(r.YtY) != r.Factors {
		return errors.New("yty does not match the factor count")
	}
	r.featureIndex = make(map[string]int, len(r.ContentProjection.Features))
	for i, name := range r.ContentProjection.Features {
		r.featureIndex[name] = i
	}
	for id, item := range r.Items {
		if len(item.F) != r.Factors || (len(item.R) > 0 && len(item.R) != r.Factors) {
			return errors.New("item " + id + " has the wrong vector length")
		}
	}
	return nil
}

// Vector returns the factors used to SCORE a movie, projecting from genres and
// release decade when the movie was never seen during training (for example, a
// title an admin just added).
func (r *Recommender) Vector(movie models.Movie) ([]float64, bool) {
	if item, ok := r.Items[movie.ImdbID]; ok {
		return item.F, !item.Cold
	}
	return r.Project(movie), false
}

// rawVector returns the unsmoothed factors used to build a viewer's vector. The
// fold-in solves against YtY, which is built from these, so mixing in the
// shrunk vectors here measurably hurts (see ml/train_recommender.py).
func (r *Recommender) rawVector(movie models.Movie) []float64 {
	if item, ok := r.Items[movie.ImdbID]; ok && len(item.R) > 0 {
		return item.R
	}
	vec, _ := r.Vector(movie)
	return vec
}

// Project maps content features (genres + decade) into the latent space using the
// ridge regression trained alongside the factorisation.
func (r *Recommender) Project(movie models.Movie) []float64 {
	features := make([]float64, len(r.ContentProjection.Features))
	for _, g := range movie.Genre {
		for _, mapped := range r.GenreMap[g.GenreName] {
			if idx, ok := r.featureIndex["genre:"+mapped]; ok {
				features[idx] = 1
			}
		}
	}
	if movie.Year > 0 {
		decade := movie.Year / 10 * 10
		if idx, ok := r.featureIndex["decade:"+strconv.Itoa(decade)]; ok {
			features[idx] = 1
		}
	}
	return r.projectFeatures(features)
}

// ProjectGenre builds a pseudo-item vector representing a genre on its own.
// The trained centroid is preferred: the ridge projection regresses towards the
// mean, so a genre put through it lands nearer other projections than the
// genre's own movies.
func (r *Recommender) ProjectGenre(genre models.Genre) []float64 {
	if vec, ok := r.GenreVectors[genre.GenreName]; ok {
		return vec
	}
	features := make([]float64, len(r.ContentProjection.Features))
	for _, mapped := range r.GenreMap[genre.GenreName] {
		if idx, ok := r.featureIndex["genre:"+mapped]; ok {
			features[idx] = 1
		}
	}
	return r.projectFeatures(features)
}

func (r *Recommender) projectFeatures(features []float64) []float64 {
	out := make([]float64, r.Factors)
	for f := 0; f < r.Factors; f++ {
		out[f] = r.ContentProjection.Intercept[f] + dot(r.ContentProjection.Weights[f], features)
	}
	return out
}

// Preference is one signal folded into a viewer's latent vector.
type Preference struct {
	Vector []float64
	Weight float64
	// ImdbID is set for watch-history signals, so a pick can cite the title.
	ImdbID string
}

// Preferences turns a viewer's watch history and favourite genres into weighted
// vectors. Watch weights decay with age.
func (r *Recommender) Preferences(user *models.User, catalogue map[string]models.Movie, now time.Time) []Preference {
	prefs := make([]Preference, 0, len(user.WatchHistory)+len(user.FavouriteGenres))

	seen := make(map[string]int, len(user.WatchHistory))
	for _, entry := range user.WatchHistory {
		movie, ok := catalogue[entry.ImdbID]
		if !ok {
			continue
		}
		vec := r.rawVector(movie)
		age := now.Sub(entry.WatchedAt)
		if age < 0 {
			age = 0
		}
		weight := 1.0 / (1.0 + float64(age)/float64(historyHalfLife))

		// Re-watching a title reinforces it rather than adding a second signal.
		if idx, dup := seen[entry.ImdbID]; dup {
			prefs[idx].Weight = math.Max(prefs[idx].Weight, weight)
			continue
		}
		seen[entry.ImdbID] = len(prefs)
		prefs = append(prefs, Preference{Vector: vec, Weight: weight, ImdbID: entry.ImdbID})
	}

	for _, genre := range user.FavouriteGenres {
		prefs = append(prefs, Preference{Vector: r.ProjectGenre(genre), Weight: genrePreferenceWeight})
	}
	return prefs
}

// UserVector folds the preferences into the latent space. This is the ALS user
// step: solve (YtY + Yu^T(Cu - I)Yu + lambda*I) u = Yu^T Cu p.
func (r *Recommender) UserVector(prefs []Preference) ([]float64, error) {
	if len(prefs) == 0 {
		return nil, errors.New("no preferences to fold in")
	}

	a := make([][]float64, r.Factors)
	for i := range a {
		a[i] = make([]float64, r.Factors)
		copy(a[i], r.YtY[i])
		a[i][i] += r.Params.Regularisation
	}
	b := make([]float64, r.Factors)

	for _, pref := range prefs {
		confidence := 1.0 + r.Params.Alpha*pref.Weight
		for i := 0; i < r.Factors; i++ {
			b[i] += confidence * pref.Vector[i]
			scaled := (confidence - 1.0) * pref.Vector[i]
			for j := 0; j <= i; j++ {
				v := scaled * pref.Vector[j]
				a[i][j] += v
				if i != j {
					a[j][i] += v
				}
			}
		}
	}
	return solveSPD(a, b)
}

// Score is the model's affinity between a viewer vector and a movie.
func (r *Recommender) Score(userVector []float64, movie models.Movie) float64 {
	vec, _ := r.Vector(movie)
	return dot(userVector, vec)
}

// Popularity is a movie's normalised popularity in the training data. Movies
// that were never rated report a neutral prior instead of zero, so the server's
// popularity debias neither rewards nor punishes them for being unknown.
func (r *Recommender) Popularity(imdbID string) float64 {
	if item, ok := r.Items[imdbID]; ok && !item.Cold {
		return item.P
	}
	return r.NeutralPopularity
}

// ClosestWatched returns the watched title most similar to the candidate, so a
// recommendation can explain itself ("because you watched ...").
func (r *Recommender) ClosestWatched(movie models.Movie, prefs []Preference) (string, float64) {
	target, _ := r.Vector(movie)
	best, bestSim := "", similarityFloor
	for _, pref := range prefs {
		if pref.ImdbID == "" || pref.ImdbID == movie.ImdbID {
			continue
		}
		if sim := cosine(target, pref.Vector); sim > bestSim {
			best, bestSim = pref.ImdbID, sim
		}
	}
	return best, bestSim
}
