# S-Watch training pipeline

Trains the two models S-Watch serves. Both export a JSON artifact into
`server/internal/model/artifacts/`, which the Go server embeds at build time, so
**inference needs no Python and no external API**.

| Script | Model | Artifact |
| --- | --- | --- |
| `train_recommender.py` | Implicit-feedback matrix factorisation (ALS) + a ridge cold-start projection | `recommender.json` |
| `train_review_classifier.py` | TF-IDF + multinomial logistic regression over 5 sentiment classes | `review_classifier.json` |

## Setup

```bash
cd ml
python -m venv .venv
.venv/Scripts/activate      # Windows; use source .venv/bin/activate elsewhere
pip install -r requirements.txt
```

## Training

```bash
python train_recommender.py
python train_review_classifier.py
```

Datasets download to `ml/data/cache/` on first run and are reused afterwards.
That directory is gitignored: MovieLens may be used for research and personal
projects but not redistributed, so only the learned weights are committed.

Rebuild the server afterwards (`go build ./...`) to embed the new artifacts, or
point the running server at them with `MODEL_DIR=./internal/model/artifacts`.

## The recommender

**Data.** MovieLens ml-latest-small: 100k real ratings from 610 viewers. Ratings
of 4.0+ become implicit positives, and `links.csv` maps MovieLens ids to IMDb
ids, which is what the S-Watch catalogue is keyed on. 17 of the 24 seeded movies
appear in the dataset; the rest are too recent for it.

**Model.** Weighted-regularised ALS (Hu, Koren & Volinsky 2008). Every movie gets
a 32-dimensional vector. Confidence is `1 + alpha * weight`, so a 5-star rating
counts roughly twice a 4-star one.

**Cold start.** A ridge regression learns to map content features (MovieLens
genres + release decade) onto the latent factors, fitted on movies with at least
5 ratings. Movies missing from the dataset — the 7 newest seeded titles, and
anything an admin adds later — get a projected vector instead. Held-out cosine
similarity against the true factors is ~0.44, so it's a rough stand-in rather
than a replacement for real co-watch data.

**Shrinkage.** A movie with a handful of ratings gets a near-zero vector, so it
scores near zero for *everyone* and stays buried regardless of who is looking —
Get Out, the catalogue's only horror film, sat at rank 13 for a horror fan. Each
item's scoring vector is therefore blended with its content projection by
`n / (n + 10)`. This is applied to **scoring only**; the viewer's own vector is
folded in from the raw ALS factors, because the fold-in wants the geometry the
model actually learned. Measured both ways:

| variant | HR@10 | NDCG@10 |
| --- | --- | --- |
| no shrinkage | 0.708 | 0.468 |
| **shrink scoring only (shipped)** | **0.719** | **0.474** |
| shrink both sides | 0.669 | 0.432 |

So the artifact exports two vectors per movie: `f` (shrunk, for scoring) and
`r` (raw, for the fold-in and for `YtY`).

**Genre preferences.** A viewer's favourite genres are folded in as pseudo-items,
using a popularity-weighted **centroid** of the real movies in that genre rather
than the ridge projection. The projection regresses towards the mean, so a genre
put through it lands nearer other projections than that genre's own films: the
projected "Animation" vector's nearest catalogue titles were Oppenheimer and
Dune, while the centroid's was Spirited Away (cosine 0.55 vs 0.31). Ridge is
still the better choice for reconstructing an *individual* unrated movie
(holdout cosine 0.433 vs 0.365), so each method is used where it wins.

**Serving.** The server never needs a trained row for the *viewer*. It folds the
viewer in at request time by solving the ALS user step against the exported
`YtY` matrix, using their watch history (weighted by recency) and their
favourite genres (as projected pseudo-items). See `server/internal/model/`.

**Measured.** Leave-one-out with 100 sampled negatives, scoring the model signal
the way the server does: hit rate @10 **0.719**, NDCG@10 **0.474**, against a
popularity baseline of **0.619**. So it beats "just show what's popular", but
610 viewers is a small dataset and the gap is modest. The editorial terms
(critic ranking, favourite-genre match) are business rules layered on top and
are not part of this measurement.

**Popularity debias.** Latent scores lean heavily towards universally watched
titles — a sci-fi fan was being served Shawshank and Forrest Gump ahead of
Inception. The server subtracts 0.30x normalised popularity from the blended
score. Sweeping that coefficient (without shrinkage, so comparable to the 0.708
baseline above): 0.0 → HR 0.708 / NDCG 0.476, **0.30 → HR 0.708 / NDCG 0.468**,
0.6 → HR 0.690 / NDCG 0.447, 1.0 → HR 0.637 / NDCG 0.401. So 0.30 buys
noticeably more relevant lists for free, and beyond that accuracy starts paying
for diversity. Movies with no ratings report a neutral popularity prior rather
than zero, so they don't dodge the debias and float to the top.

**Live data.** Once S-Watch has real viewers, fold their history in:

```bash
python train_recommender.py --mongo-uri "mongodb+srv://..." --database swatch
```

## The review classifier

**Data.** SST-5 (Stanford Sentiment Treebank): ~11k Rotten Tomatoes snippets
labelled 0-4, which maps directly onto Terrible..Excellent. Plus
`data/swatch_reviews.csv`, 56 full-sentence critic reviews in the house style,
weighted 8x because they match what admins actually write.

**Measured on SST-5's test split.** Exact-level accuracy **0.41**, macro-F1
**0.39**, within-one-level **0.82**. Five-class sentiment is genuinely hard and a
linear bag-of-words model is near its ceiling here (published SOTA is ~0.55).
Treat the prediction as a suggestion: the admin UI shows the confidence and the
full distribution, and an admin can always override it.

**Parity.** The Go server re-implements the TF-IDF transform. The artifact ships
the probabilities scikit-learn produced for five fixed reviews, and
`TestClassifierMatchesScikitLearn` fails if the two implementations drift.

## Ideas worth trying

- Train on `ml-25m` (162k viewers) for materially better item vectors.
- BM25-weight the interaction matrix. Tried it — on this small dataset it pushed
  all similarities toward 0.99 and made neighbours worse, but it usually helps at
  larger scale.
- Replace bag-of-words with fine-tuned sentence embeddings for the classifier.
