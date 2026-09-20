# S-Watch 🎬

**Movie streaming platform with its own trained recommendation model**, built with Go (Gin-Gonic), React, MongoDB, and models trained in Python and served from Go.

- **Streaming UI:** a responsive React front end with search, genre filters and pagination. Trailers play through React-Player.
- **Secure auth:** bcrypt password hashing and short-lived JWT access tokens. Refresh tokens rotate on every use, only their SHA-256 hash is stored, and a stolen token can be revoked on logout.
- **Recommendations:** a matrix-factorisation model trained on 100k real MovieLens ratings. Viewers are folded into the latent space at request time from their watch history and favourite genres.
- **Review ranking:** a sentiment classifier trained on SST-5 turns an admin's free-text review into Excellent, Good, Okay, Bad or Terrible.
- **No external AI API:** both models train offline in `ml/` and are embedded in the Go binary as JSON. Inference is a matrix solve and a dot product, so recommendations return in single-digit milliseconds with no API key, no per-request cost and no vendor dependency.
- **Cloud-ready:** the API runs on Render, the client on Vercel and the database on MongoDB Atlas.

```
S-Watch/
├── ml/                         Python training pipeline (offline)
│   ├── train_recommender.py        ALS matrix factorisation on MovieLens
│   ├── train_review_classifier.py  TF-IDF + logistic regression on SST-5
│   └── data/swatch_reviews.csv     in-house labelled reviews
├── server/                     Go + Gin REST API
│   ├── main.go                 bootstrap, CORS, graceful shutdown
│   └── internal/
│       ├── config/             env configuration
│       ├── database/           MongoDB connection + indexes
│       ├── models/             Movie, User, Genre, Ranking documents
│       ├── auth/               JWT access/refresh tokens, bcrypt
│       ├── middleware/         RequireAuth, RequireRole
│       ├── model/              trained artifacts + inference (fold-in, TF-IDF, softmax)
│       ├── recommend/          blends model scores with editorial signals
│       ├── handlers/           REST endpoints
│       └── seed/               genres, rankings, 24 starter movies, admin user
├── client/                     React 19 + Vite SPA
│   └── src/{api,context,components,pages}
└── render.yaml                 Render blueprint for the API
```

## How recommendations work

**Offline** (`ml/train_recommender.py`, see [ml/README.md](ml/README.md)): ALS matrix factorisation over 100k MovieLens ratings gives every movie a 32-dimensional vector. A ridge regression learns to predict those vectors from genres and release decade, covering movies too new to appear in the dataset.

**At request time** ([recommender.go](server/internal/model/recommender.go)): the viewer has no trained row, so the server builds one by solving the ALS user step over their watch history (weighted by recency, 21-day half-life) plus their favourite genres as projected pseudo-items. That is a 32x32 Cholesky solve per request.

**Blending** ([recommend.go](server/internal/recommend/recommend.go)): the latent dot product has no natural scale, so it is min-maxed across the catalogue and mixed with editorial signals:
- **60%: model affinity**
- **25%: critic ranking** (unranked movies get a neutral 0.5)
- **15%: favourite-genre match.** The latent space alone under-serves stated preferences: a recent or niche title has too few ratings in a 610-viewer dataset for its vector to carry its genre, so a horror fan was never shown the catalogue's horror film.
- **-30%: popularity debias.** Latent scores lean towards titles everyone watched, so a sci-fi fan was getting the same classics as everyone else. Subtracting popularity costs no hit rate and 1.6% of NDCG (the sweep is in [ml/README.md](ml/README.md)).
- **-0.35** for anything watched in the last 30 days

**Explanations** come from the model itself: each pick cites the watched title with the highest cosine similarity to it, falling back to genre and critic signals when nothing clears a 0.25 similarity floor.

**Sparse titles** are smoothed towards their content projection by `n / (n + 10)` for scoring, which both un-buries them and measures better. **Favourite genres** are folded in as popularity-weighted genre centroids. Both choices were measured against alternatives — the numbers are in [ml/README.md](ml/README.md).

**Measured:** hit rate @10 of 0.719 against a popularity baseline of 0.619 (leave-one-out, 100 sampled negatives). The `/api/model` endpoint reports the version and training metrics of whatever is currently serving.

## Running locally

**Prerequisites:** Go 1.23+, Node 20+, MongoDB (local or Atlas). Python 3.11+ only if you want to retrain the models - the trained artifacts are committed, so the server runs without it.

### 1. API

```bash
cd server
cp .env.example .env
```

Edit `.env` and set `SECRET_KEY`, `REFRESH_SECRET_KEY` and `ADMIN_PASSWORD`. Then:

```bash
go mod tidy
go test ./...
go run .
```

The API starts on `http://localhost:8080`. On first boot it creates indexes and seeds genres, rankings, 24 movies and the admin account from `ADMIN_EMAIL` / `ADMIN_PASSWORD`.

### 2. Client

```bash
cd client
cp .env.example .env
npm install
npm run dev
```

Open `http://localhost:5173`.

## API reference

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| GET | `/health` | none | Health check and the serving model versions |
| GET | `/api/model` | none | Model versions, training data and metrics |
| POST | `/api/auth/register` | none | Create an account and get tokens |
| POST | `/api/auth/login` | none | Log in and get tokens |
| POST | `/api/auth/refresh` | none | Rotate the token pair using `refresh_token` |
| POST | `/api/auth/logout` | user | Revoke the refresh token |
| GET | `/api/genres` | none | List genres |
| GET | `/api/rankings` | none | List ranking levels |
| GET | `/api/movies?q=&genre=&page=&limit=` | none | Search and filter movies |
| GET | `/api/movies/:imdb_id` | none | Movie details |
| GET | `/api/me` | user | Current profile |
| PUT | `/api/me/genres` | user | Update favourite genres |
| POST | `/api/movies/:imdb_id/watch` | user | Record a view |
| GET | `/api/recommendations?limit=` | user | Personalised picks |
| POST | `/api/admin/movies` | admin | Add a movie (any review is classified) |
| PATCH | `/api/admin/movies/:imdb_id/review` | admin | Save a review; the classifier ranks it unless `ranking_name` overrides |
| POST | `/api/admin/reviews/preview` | admin | Classify a draft review without saving |
| DELETE | `/api/admin/movies/:imdb_id` | admin | Remove a movie |

Authenticated requests send `Authorization: Bearer <access_token>`. The client refreshes an expired token automatically and retries the request once.

## Deployment

### MongoDB Atlas
1. Create a free cluster and a database user.
2. Under *Network Access*, allow `0.0.0.0/0`, because Render's outbound IPs are dynamic.
3. Copy the `mongodb+srv://…` connection string.

### Render (API)
1. Push this repo to GitHub.
2. In Render, choose **New → Blueprint** and pick the repo. It reads `render.yaml`.
3. Fill in the prompted secrets:
   - `MONGODB_URI`
   - `ADMIN_EMAIL` and `ADMIN_PASSWORD`
   - `ALLOWED_ORIGINS`, e.g. `https://s-watch.vercel.app,https://*.vercel.app`

   The JWT secrets are generated for you.
4. Once it's live, check `https://<service>.onrender.com/health`.

### Vercel (client)
1. Import the repo and set **Root Directory** to `client`. Vercel detects Vite.
2. Add the environment variable `VITE_API_BASE_URL=https://<service>.onrender.com/api`.
3. Deploy. `vercel.json` rewrites every route to `index.html` so client-side routing works.

## How review ranking works

An admin writes a review; a TF-IDF + logistic-regression classifier trained on SST-5 predicts one of the five rankings. The Go server re-implements scikit-learn's TF-IDF transform, and a test asserts it matches the Python model's probabilities on fixed inputs.

The model gets the exact level right about 41% of the time and lands within one level about 82% of the time, so the admin UI shows the prediction, its confidence and the full distribution, and an admin can override it. The numbers and the reasoning are in [ml/README.md](ml/README.md).

## Retraining

```bash
cd ml && python train_recommender.py && python train_review_classifier.py
```

This rewrites the artifacts in `server/internal/model/artifacts/`; rebuild the server to embed them. Once real viewers have watch history, pass `--mongo-uri` to fold it into the training data.

## Tech stack

- **Backend:** Go, Gin-Gonic, MongoDB Go Driver v2, golang-jwt v5, bcrypt
- **ML:** Python, NumPy, pandas, scikit-learn, SciPy - training only; artifacts embed into the Go binary
- **Frontend:** React 19, React Router 7, Axios, React-Player 3, Vite
- **Infrastructure:** Render, Vercel, MongoDB Atlas

## Data credits

- [MovieLens](https://grouplens.org/datasets/movielens/) ml-latest-small, GroupLens Research. Used for research/personal projects; the raw data is not redistributed here.
- [Stanford Sentiment Treebank](https://nlp.stanford.edu/sentiment/) (Socher et al., 2013).
