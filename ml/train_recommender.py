"""Train the S-Watch recommender: implicit-feedback matrix factorisation (ALS).

Data
----
* MovieLens ml-latest-small (100k real ratings) - downloaded at training time and
  joined to IMDb ids via links.csv, so it lines up with the S-Watch catalogue.
* Optionally, live S-Watch watch history read straight from MongoDB (--mongo-uri).

Model
-----
* Weighted-regularised ALS (Hu, Koren & Volinsky 2008) over implicit positives.
* A ridge regression from content features (genres + decade) to latent factors so
  movies nobody has rated yet - such as anything newer than the dataset - still
  get a usable vector. The server uses the same projection for admin-added movies.

Exports `server/internal/model/artifacts/recommender.json`, which the Go server
embeds. The server folds a viewer's watch history into a user vector at request
time using the exported YtY matrix, which is the same maths as the ALS user step.

Usage:
    python train_recommender.py [--factors 32] [--mongo-uri mongodb://...]
"""

from __future__ import annotations

import argparse
import math
import sys

import numpy as np
import pandas as pd
from scipy.sparse import csr_matrix
from sklearn.linear_model import Ridge

from swatch_ml.common import (
    CACHE_DIR,
    GENRE_MAP,
    download,
    extract,
    load_catalogue,
    round_matrix,
    write_artifact,
)

MOVIELENS_URL = "https://files.grouplens.org/datasets/movielens/ml-latest-small.zip"
POSITIVE_THRESHOLD = 4.0  # ratings >= this count as an implicit positive
MIN_RATINGS_FOR_PROJECTION = 5
# Shrinkage constant: an item with this many ratings is weighted half towards its
# learned vector and half towards its content projection.
SHRINKAGE_K = 10.0


# --------------------------------------------------------------------------- data


def load_movielens() -> tuple[pd.DataFrame, pd.DataFrame]:
    """Return (interactions, items) keyed by IMDb id."""
    archive = download(MOVIELENS_URL, CACHE_DIR / "ml-latest-small.zip")
    files = extract(archive, ".csv", CACHE_DIR / "ml-latest-small")

    ratings = pd.read_csv(files["ratings.csv"])
    movies = pd.read_csv(files["movies.csv"])
    links = pd.read_csv(files["links.csv"], dtype={"imdbId": "string"})

    links = links.dropna(subset=["imdbId"])
    links["imdb_id"] = "tt" + links["imdbId"].str.zfill(7)
    items = movies.merge(links[["movieId", "imdb_id"]], on="movieId", how="inner")
    items["year"] = items["title"].str.extract(r"\((\d{4})\)\s*$")[0].astype("Float64")

    positives = ratings[ratings["rating"] >= POSITIVE_THRESHOLD].merge(
        items[["movieId", "imdb_id"]], on="movieId", how="inner"
    )
    interactions = pd.DataFrame(
        {
            "user": "ml:" + positives["userId"].astype(str),
            "imdb_id": positives["imdb_id"],
            "weight": positives["rating"] - POSITIVE_THRESHOLD + 1.0,  # 4.0 -> 1.0, 5.0 -> 2.0
            "timestamp": positives["timestamp"],
        }
    )
    print(f"  MovieLens: {len(interactions):,} positives, "
          f"{interactions['user'].nunique():,} users, {interactions['imdb_id'].nunique():,} movies")
    return interactions, items


def load_live_interactions(mongo_uri: str, database: str) -> pd.DataFrame:
    """Read watch history from a running S-Watch database."""
    from pymongo import MongoClient

    with MongoClient(mongo_uri, serverSelectionTimeoutMS=10_000) as client:
        users = list(client[database]["users"].find({}, {"user_id": 1, "watch_history": 1}))

    rows = []
    for user in users:
        for entry in user.get("watch_history") or []:
            watched_at = entry.get("watched_at")
            rows.append(
                {
                    "user": "swatch:" + user["user_id"],
                    "imdb_id": entry["imdb_id"],
                    "weight": 1.0,
                    "timestamp": watched_at.timestamp() if watched_at else 0.0,
                }
            )
    df = pd.DataFrame(rows, columns=["user", "imdb_id", "weight", "timestamp"])
    print(f"  S-Watch: {len(df):,} watch events from {df['user'].nunique() if len(df) else 0:,} viewers")
    return df


# ---------------------------------------------------------------------------- ALS


def build_matrix(interactions: pd.DataFrame) -> tuple[csr_matrix, list[str], list[str]]:
    users = sorted(interactions["user"].unique())
    items = sorted(interactions["imdb_id"].unique())
    user_index = {u: i for i, u in enumerate(users)}
    item_index = {m: i for i, m in enumerate(items)}

    rows = interactions["user"].map(user_index).to_numpy()
    cols = interactions["imdb_id"].map(item_index).to_numpy()
    vals = interactions["weight"].to_numpy(dtype=np.float64)
    matrix = csr_matrix((vals, (rows, cols)), shape=(len(users), len(items)))
    matrix.sum_duplicates()
    return matrix, users, items


def _solve_side(
    matrix: csr_matrix, other: np.ndarray, reg: float, alpha: float
) -> np.ndarray:
    """One ALS half-step: solve for every row of `matrix` given the other factors."""
    factors = other.shape[1]
    out = np.zeros((matrix.shape[0], factors))
    base = other.T @ other + reg * np.eye(factors)

    for row in range(matrix.shape[0]):
        start, end = matrix.indptr[row], matrix.indptr[row + 1]
        idx = matrix.indices[start:end]
        if idx.size == 0:
            continue
        # Confidence c = 1 + alpha * w; preference p = 1 for observed entries.
        conf = 1.0 + alpha * matrix.data[start:end]
        vecs = other[idx]
        # A = base + Y_u^T (C_u - I) Y_u, b = Y_u^T C_u p_u
        a = base + vecs.T @ (vecs * (conf - 1.0)[:, None])
        b = vecs.T @ conf
        out[row] = np.linalg.solve(a, b)
    return out


def train_als(
    matrix: csr_matrix, factors: int, reg: float, alpha: float, iterations: int, seed: int
) -> tuple[np.ndarray, np.ndarray]:
    rng = np.random.default_rng(seed)
    user_f = rng.normal(scale=0.01, size=(matrix.shape[0], factors))
    item_f = rng.normal(scale=0.01, size=(matrix.shape[1], factors))
    matrix_t = matrix.T.tocsr()

    for it in range(iterations):
        user_f = _solve_side(matrix, item_f, reg, alpha)
        item_f = _solve_side(matrix_t, user_f, reg, alpha)
        if it == iterations - 1 or it % 5 == 0:
            print(f"    iteration {it + 1}/{iterations}")
    return user_f, item_f


def fold_in(item_f: np.ndarray, idx: np.ndarray, weights: np.ndarray, reg: float, alpha: float) -> np.ndarray:
    """Build a user vector from item factors - the same step the Go server runs."""
    factors = item_f.shape[1]
    conf = 1.0 + alpha * weights
    vecs = item_f[idx]
    a = item_f.T @ item_f + reg * np.eye(factors) + vecs.T @ (vecs * (conf - 1.0)[:, None])
    return np.linalg.solve(a, vecs.T @ conf)


# --------------------------------------------------------------- content + shrinkage


def fit_projection(features: np.ndarray, item_f: np.ndarray, counts: np.ndarray) -> Ridge:
    """Ridge from content features to latent factors, fitted on well-rated items."""
    trainable = counts >= MIN_RATINGS_FOR_PROJECTION
    return Ridge(alpha=1.0).fit(features[trainable], item_f[trainable])


def shrink_factors(
    item_f: np.ndarray, counts: np.ndarray, rows: list[int], features: np.ndarray, k: float
) -> np.ndarray:
    """Pull thinly-rated items towards their content projection, for SCORING only.

    A movie with a handful of ratings gets a near-zero vector, so it scores near
    zero for everyone and stays buried no matter who is looking. Blending it with
    the content prior by `n / (n + k)` fixes that and measures better: k=10
    lifts HR@10 from 0.708 to 0.719 on the leave-one-out protocol.

    The viewer's own vector is still folded in from the raw factors. Smoothing
    both sides instead drops HR@10 to 0.669 - the fold-in wants the geometry ALS
    actually learned, while candidate scoring benefits from the smoothing.
    """
    if k <= 0:
        return item_f
    ridge = fit_projection(features, item_f[rows], counts[rows])
    projected = ridge.predict(features)

    out = item_f.copy()
    weight = (counts[rows] / (counts[rows] + k))[:, None]
    out[rows] = weight * item_f[rows] + (1.0 - weight) * projected
    return out


def content_rows(items: pd.DataFrame, item_ids: list[str]) -> tuple[list[int], np.ndarray, list[str]]:
    """Content features for every item that has MovieLens metadata."""
    meta = items.drop_duplicates("imdb_id").set_index("imdb_id")
    known = [i for i in item_ids if i in meta.index]
    features, names = build_content_features(meta.loc[known])
    pos = {m: i for i, m in enumerate(item_ids)}
    return [pos[i] for i in known], features, names


# --------------------------------------------------------------------- evaluation


def evaluate(
    interactions: pd.DataFrame,
    items: pd.DataFrame,
    factors: int,
    reg: float,
    alpha: float,
    iterations: int,
    seed: int,
    shrinkage: float,
    popularity_debias: float,
    k: int = 10,
) -> dict:
    """Leave-one-out: hide each user's most recent positive and try to rank it back.

    Scoring mirrors what the server does with the model signal - shrunk vectors,
    min-max normalisation and the popularity debias - so the reported numbers
    describe the deployed behaviour. The editorial terms (critic ranking,
    favourite-genre match) are business rules and are not evaluated here.
    """
    counts = interactions.groupby("user")["imdb_id"].transform("size")
    eligible = interactions[counts >= 5].copy()
    if eligible.empty:
        return {"note": "not enough interactions to evaluate"}

    eligible = eligible.sort_values("timestamp")
    held_out = eligible.groupby("user").tail(1)
    train = eligible.drop(held_out.index)

    matrix, users, item_ids = build_matrix(train)
    item_index = {m: i for i, m in enumerate(item_ids)}
    user_index = {u: i for i, u in enumerate(users)}
    _, item_f = train_als(matrix, factors, reg, alpha, iterations, seed)

    counts = np.asarray((matrix > 0).sum(axis=0)).ravel().astype(float)
    rows, features, _ = content_rows(items, item_ids)
    scoring_f = shrink_factors(item_f, counts, rows, features, shrinkage)
    popularity = counts / counts.max()
    rng = np.random.default_rng(seed)

    hits = ndcg = pop_hits = evaluated = 0
    sample = held_out.sample(n=min(300, len(held_out)), random_state=seed)
    for _, row in sample.iterrows():
        if row["user"] not in user_index or row["imdb_id"] not in item_index:
            continue
        target = item_index[row["imdb_id"]]
        seen = set(matrix.indices[matrix.indptr[user_index[row["user"]]] : matrix.indptr[user_index[row["user"]] + 1]])

        # Rank the held-out item against 100 sampled negatives (standard protocol).
        negatives = []
        while len(negatives) < 100:
            cand = int(rng.integers(0, len(item_ids)))
            if cand != target and cand not in seen:
                negatives.append(cand)
        candidates = np.array([target] + negatives)

        user_idx = user_index[row["user"]]
        start, end = matrix.indptr[user_idx], matrix.indptr[user_idx + 1]
        user_vec = fold_in(item_f, matrix.indices[start:end], matrix.data[start:end], reg, alpha)

        raw = scoring_f[candidates] @ user_vec
        spread = raw.max() - raw.min()
        scores = (raw - raw.min()) / spread if spread > 0 else np.full_like(raw, 0.5)
        scores = scores - popularity_debias * popularity[candidates]
        rank = int((scores > scores[0]).sum())
        if rank < k:
            hits += 1
            ndcg += 1.0 / math.log2(rank + 2)
        if int((popularity[candidates] > popularity[candidates][0]).sum()) < k:
            pop_hits += 1
        evaluated += 1

    return {
        "protocol": f"leave-one-out, 100 sampled negatives, k={k}",
        "scoring": f"shrinkage k={shrinkage}, popularity debias {popularity_debias}",
        "users_evaluated": evaluated,
        "hit_rate_at_k": round(hits / evaluated, 4) if evaluated else None,
        "ndcg_at_k": round(ndcg / evaluated, 4) if evaluated else None,
        "popularity_baseline_hit_rate": round(pop_hits / evaluated, 4) if evaluated else None,
    }


# ------------------------------------------------------------------- cold starts


def build_content_features(items: pd.DataFrame) -> tuple[np.ndarray, list[str]]:
    """Multi-hot MovieLens genres plus a decade bucket."""
    genres = sorted({g for row in items["genres"] for g in row.split("|") if g != "(no genres listed)"})
    decades = sorted({int(y // 10 * 10) for y in items["year"].dropna().astype(int)})
    names = [f"genre:{g}" for g in genres] + [f"decade:{d}" for d in decades]
    index = {n: i for i, n in enumerate(names)}

    features = np.zeros((len(items), len(names)))
    for row, (_, item) in enumerate(items.iterrows()):
        for g in item["genres"].split("|"):
            if f"genre:{g}" in index:
                features[row, index[f"genre:{g}"]] = 1.0
        if not pd.isna(item["year"]):
            decade = f"decade:{int(item['year']) // 10 * 10}"
            if decade in index:
                features[row, index[decade]] = 1.0
    return features, names


def genre_centroids(
    item_ids: list[str], item_f: np.ndarray, popularity: np.ndarray, items: pd.DataFrame
) -> tuple[dict[str, list], dict]:
    """Popularity-weighted mean vector of the real movies in each S-Watch genre.

    The ridge projection reconstructs an individual movie's vector better, but it
    regresses towards the mean, so a genre projected through it lands closest to
    other projections rather than to that genre's movies. Centroids are built
    from actual trained vectors, so they point where the genre really lives.
    """
    meta = items.drop_duplicates("imdb_id").set_index("imdb_id")
    pos = {m: i for i, m in enumerate(item_ids)}

    vectors: dict[str, list] = {}
    sizes: dict[str, int] = {}
    for swatch_genre, movielens_genres in GENRE_MAP.items():
        rows = [
            pos[imdb_id]
            for imdb_id in item_ids
            if imdb_id in meta.index
            and popularity[pos[imdb_id]] >= MIN_RATINGS_FOR_PROJECTION
            and any(g in str(meta.loc[imdb_id, "genres"]).split("|") for g in movielens_genres)
        ]
        if not rows:
            continue
        weights = np.log1p(popularity[rows])[:, None]
        vectors[swatch_genre] = round_matrix((item_f[rows] * weights).sum(axis=0) / weights.sum())
        sizes[swatch_genre] = len(rows)
    return vectors, {"genres": len(vectors), "movies_per_genre": sizes}


def train_projection(
    features: np.ndarray, item_f: np.ndarray, counts: np.ndarray, seed: int
) -> tuple[Ridge, dict]:
    """Learn content features -> latent factors, so unrated movies still get a vector."""
    trainable = counts >= MIN_RATINGS_FOR_PROJECTION
    x, y = features[trainable], item_f[trainable]
    rng = np.random.default_rng(seed)
    mask = rng.random(len(x)) < 0.85

    model = Ridge(alpha=1.0).fit(x[mask], y[mask])
    predicted = model.predict(x[~mask])
    actual = y[~mask]
    cos = (
        (predicted * actual).sum(axis=1)
        / (np.linalg.norm(predicted, axis=1) * np.linalg.norm(actual, axis=1) + 1e-9)
    )
    metrics = {
        "trained_on_items": int(mask.sum()),
        "holdout_items": int((~mask).sum()),
        "holdout_cosine_similarity": round(float(cos.mean()), 4),
    }
    return Ridge(alpha=1.0).fit(x, y), metrics


# --------------------------------------------------------------------------- main


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--factors", type=int, default=32)
    parser.add_argument("--regularisation", type=float, default=0.08)
    parser.add_argument("--alpha", type=float, default=40.0, help="implicit confidence scaling")
    parser.add_argument("--iterations", type=int, default=15)
    parser.add_argument("--shrinkage", type=float, default=SHRINKAGE_K,
                        help="pull thinly-rated items towards their content projection (0 disables)")
    parser.add_argument("--popularity-debias", type=float, default=0.30,
                        help="must match recommend.DefaultWeights.PopularityDebias in the server")
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--extra-items", type=int, default=1500,
                        help="popular non-catalogue movies to keep, so admin-added titles get real vectors")
    parser.add_argument("--mongo-uri", default=None, help="blend in live S-Watch watch history")
    parser.add_argument("--database", default="swatch")
    parser.add_argument("--skip-eval", action="store_true")
    args = parser.parse_args()

    print("1. Loading data")
    interactions, items = load_movielens()
    if args.mongo_uri:
        live = load_live_interactions(args.mongo_uri, args.database)
        if not live.empty:
            interactions = pd.concat([interactions, live], ignore_index=True)

    catalogue = load_catalogue()
    catalogue_ids = {m.imdb_id for m in catalogue}
    covered = catalogue_ids & set(interactions["imdb_id"])
    print(f"  catalogue: {len(covered)}/{len(catalogue_ids)} movies have ratings; "
          f"{len(catalogue_ids - covered)} will use the content projection")

    metrics: dict = {}
    if not args.skip_eval:
        print("2. Evaluating (leave-one-out)")
        metrics["recommender"] = evaluate(
            interactions, items, args.factors, args.regularisation, args.alpha,
            args.iterations, args.seed, args.shrinkage, args.popularity_debias,
        )
        print(f"  {metrics['recommender']}")
    else:
        print("2. Evaluation skipped")

    print("3. Training final model on all interactions")
    matrix, users, item_ids = build_matrix(interactions)
    _, item_f = train_als(matrix, args.factors, args.regularisation, args.alpha, args.iterations, args.seed)
    popularity = np.asarray((matrix > 0).sum(axis=0)).ravel().astype(float)
    print(f"  {len(users):,} users x {len(item_ids):,} items, {args.factors} factors")

    print("4. Building genre centroids and the cold-start projection")
    rows, features, feature_names = content_rows(items, item_ids)
    scoring_f = shrink_factors(item_f, popularity, rows, features, args.shrinkage)
    print(f"  shrinkage k={args.shrinkage} applied to {len(rows):,} items with metadata")
    projection, metrics["projection"] = train_projection(features, item_f[rows], popularity[rows], args.seed)
    print(f"  ridge projection: {metrics['projection']}")
    genre_vectors, metrics["genre_centroids"] = genre_centroids(item_ids, item_f, popularity, items)
    print(f"  genre centroids: {metrics['genre_centroids']['genres']} genres")

    print("5. Exporting artifacts")
    # Keep every catalogue movie plus the most-rated others, so the file stays small.
    ranked = sorted(range(len(item_ids)), key=lambda i: -popularity[i])
    keep = [i for i in ranked if item_ids[i] in catalogue_ids]
    keep += [i for i in ranked if item_ids[i] not in catalogue_ids][: args.extra_items]

    max_pop = float(popularity.max()) or 1.0
    exported_items = {
        item_ids[i]: {
            # "f" scores candidates (shrunk); "r" builds the viewer (raw ALS).
            "f": round_matrix(scoring_f[i]),
            "r": round_matrix(item_f[i]),
            "p": round(float(popularity[i]) / max_pop, 5),
            "n": int(popularity[i]),
        }
        for i in keep
    }

    # Catalogue movies with no ratings at all get a projected vector now.
    genre_feature_index = {name: i for i, name in enumerate(feature_names)}
    for movie in catalogue:
        if movie.imdb_id in exported_items:
            continue
        vec = np.zeros(len(feature_names))
        for genre in movie.genres:
            for mapped in GENRE_MAP.get(genre, []):
                if f"genre:{mapped}" in genre_feature_index:
                    vec[genre_feature_index[f"genre:{mapped}"]] = 1.0
        decade = f"decade:{movie.year // 10 * 10}"
        if decade in genre_feature_index:
            vec[genre_feature_index[decade]] = 1.0
        projected = round_matrix(projection.predict(vec[None, :])[0])
        exported_items[movie.imdb_id] = {"f": projected, "r": projected, "p": 0.0, "n": 0, "cold": True}

    write_artifact(
        "recommender.json",
        {
            "kind": "implicit-als",
            "version": f"als-{args.factors}f-v1",
            "factors": args.factors,
            "params": {
                "regularisation": args.regularisation,
                "alpha": args.alpha,
                "iterations": args.iterations,
                "positive_threshold": POSITIVE_THRESHOLD,
                "shrinkage_k": args.shrinkage,
            },
            "dataset": {
                "source": "MovieLens ml-latest-small" + (" + S-Watch watch history" if args.mongo_uri else ""),
                "users": len(users),
                "items": len(item_ids),
                "interactions": int(matrix.nnz),
                "exported_items": len(exported_items),
            },
            "metrics": metrics,
            # YtY over every trained item, from the RAW factors: the Go server
            # needs it for the fold-in, which runs in the unsmoothed geometry.
            "yty": round_matrix(item_f.T @ item_f, 6),
            "items": exported_items,
            "neutral_popularity": round(
                float(np.mean([popularity[i] / max_pop for i in keep if popularity[i] > 0])), 5
            ),
            "genre_vectors": genre_vectors,
            "content_projection": {
                "features": feature_names,
                "weights": round_matrix(projection.coef_),
                "intercept": round_matrix(projection.intercept_),
            },
            "genre_map": GENRE_MAP,
        },
    )
    print("Done.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
