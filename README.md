# S-Watch 🎬

**AI-powered movie streaming platform** built with Go (Gin-Gonic), React, MongoDB and OpenAI (via LangChainGo).

- **Streaming UI:** a responsive React front end with search, genre filters and pagination. Trailers play through React-Player.
- **Secure auth:** bcrypt password hashing and short-lived JWT access tokens. Refresh tokens rotate on every use, only their SHA-256 hash is stored, and a stolen token can be revoked on logout.
- **Recommendations:** a weighted ranking algorithm pre-scores the catalogue, then OpenAI re-ranks the top candidates and writes a short reason for each pick.
- **AI review ranking:** an admin writes a free-text review and the LLM classifies it as Excellent, Good, Okay, Bad or Terrible.
- **Cloud-ready:** the API runs on Render, the client on Vercel and the database on MongoDB Atlas.

```
S-Watch/
├── server/                     Go + Gin REST API
│   ├── main.go                 bootstrap, CORS, graceful shutdown
│   └── internal/
│       ├── config/             env configuration
│       ├── database/           MongoDB connection + indexes
│       ├── models/             Movie, User, Genre, Ranking documents
│       ├── auth/               JWT access/refresh tokens, bcrypt
│       ├── middleware/         RequireAuth, RequireRole
│       ├── ai/                 LangChainGo + OpenAI (review ranking, re-ranking)
│       ├── recommend/          weighted scoring algorithm (+ tests)
│       ├── handlers/           REST endpoints
│       └── seed/               genres, rankings, 24 starter movies, admin user
├── client/                     React 19 + Vite SPA
│   └── src/{api,context,components,pages}
└── render.yaml                 Render blueprint for the API
```

## How recommendations work

1. **Heuristic scoring** in [`recommend.go`](server/internal/recommend/recommend.go) gives every movie a score:
   - **45%: critic ranking.** The admin review ranking is normalised to [0, 1]. Unranked movies get a neutral 0.5.
   - **35%: favourite-genre fit.** Any match scores at least 0.6, and the score rises as the match covers more of the movie's genres.
   - **20%: watch-history affinity.** This is per-genre, weighted by recency (14-day half-life decay) and normalised.
   - **Recent-watch penalty (−0.35).** Applies to titles watched in the last 30 days.
2. **LLM re-ranking.** When `OPENAI_API_KEY` is set, the top 15 candidates and the viewer's profile go to OpenAI through LangChainGo in JSON mode. The model picks the best N and explains each one. The server discards any IDs that weren't in the candidate list.
3. **Fallback.** If the key isn't set or the LLM call fails, the API returns the heuristic ranking with rule-based reasons. The UI labels the results *AI-curated* or *Smart ranking*.

## Running locally

**Prerequisites:** Go 1.23+, Node 20+, MongoDB (local or Atlas). An OpenAI API key is optional.

### 1. API

```bash
cd server
cp .env.example .env
```

Edit `.env` and set `SECRET_KEY`, `REFRESH_SECRET_KEY`, `ADMIN_PASSWORD` and (optionally) `OPENAI_API_KEY`. Then:

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
| GET | `/health` | none | Health check and whether AI is enabled |
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
| POST | `/api/admin/movies` | admin | Add a movie (review is AI-ranked) |
| PATCH | `/api/admin/movies/:imdb_id/review` | admin | Update the review and let AI re-rank it |
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
   - `OPENAI_API_KEY`
   - `ADMIN_EMAIL` and `ADMIN_PASSWORD`
   - `ALLOWED_ORIGINS`, e.g. `https://s-watch.vercel.app,https://*.vercel.app`

   The JWT secrets are generated for you.
4. Once it's live, check `https://<service>.onrender.com/health`.

### Vercel (client)
1. Import the repo and set **Root Directory** to `client`. Vercel detects Vite.
2. Add the environment variable `VITE_API_BASE_URL=https://<service>.onrender.com/api`.
3. Deploy. `vercel.json` rewrites every route to `index.html` so client-side routing works.

## Tech stack

- **Backend:** Go, Gin-Gonic, MongoDB Go Driver v2, golang-jwt v5, bcrypt, LangChainGo, OpenAI
- **Frontend:** React 19, React Router 7, Axios, React-Player 3, Vite
- **Infrastructure:** Render, Vercel, MongoDB Atlas
