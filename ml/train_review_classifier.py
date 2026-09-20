"""Train the S-Watch review classifier: TF-IDF + multinomial logistic regression.

Data
----
* SST-5 (Stanford Sentiment Treebank) - ~11k Rotten Tomatoes review snippets whose
  five root labels (very negative .. very positive) line up exactly with the
  S-Watch rankings Terrible .. Excellent.
* `data/swatch_reviews.csv` - S-Watch style full-sentence critic reviews, carrying
  extra sample weight because they match what admins actually write.

Exports `server/internal/model/artifacts/review_classifier.json`. The Go server
re-implements the TF-IDF transform and the softmax, so inference needs no Python.
Parity fixtures are exported alongside so the Go tests can prove the two agree.

Usage:
    python train_review_classifier.py
"""

from __future__ import annotations

import argparse
import re
import sys

import numpy as np
import pandas as pd
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import accuracy_score, classification_report, f1_score
from sklearn.model_selection import train_test_split

from swatch_ml.common import CACHE_DIR, ML_DIR, download, extract, round_matrix, write_artifact

SST_URL = "https://nlp.stanford.edu/sentiment/trainDevTestTrees_PTB.zip"
# SST-5 root labels (0..4) map onto the S-Watch ranking scale (1..5).
RANKING_NAMES = {1: "Terrible", 2: "Bad", 3: "Okay", 4: "Good", 5: "Excellent"}
LEAF = re.compile(r"\(\d+ ([^()]+)\)")
TOKEN_PATTERN = r"(?u)\b\w\w+\b"


def parse_sst(path) -> pd.DataFrame:
    """Read PTB-format trees, keeping the root label and the reconstructed sentence."""
    rows = []
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line:
            continue
        words = LEAF.findall(line)
        if not words:
            continue
        # The root label is the first number in the tree.
        rows.append({"ranking_value": int(line[1]) + 1, "text": " ".join(words)})
    return pd.DataFrame(rows)


def load_sst() -> dict[str, pd.DataFrame]:
    archive = download(SST_URL, CACHE_DIR / "trainDevTestTrees_PTB.zip")
    files = extract(archive, ".txt", CACHE_DIR / "sst")
    splits = {name: parse_sst(files[f"{name}.txt"]) for name in ("train", "dev", "test")}
    for name, df in splits.items():
        print(f"  SST-5 {name}: {len(df):,} sentences")
    return splits


def load_domain() -> pd.DataFrame:
    df = pd.read_csv(ML_DIR / "data" / "swatch_reviews.csv")
    print(f"  S-Watch reviews: {len(df)} examples")
    return df


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--max-features", type=int, default=20000)
    parser.add_argument("--min-df", type=int, default=2)
    parser.add_argument("--domain-weight", type=float, default=8.0,
                        help="how much more each S-Watch review counts than an SST snippet")
    parser.add_argument("--seed", type=int, default=42)
    args = parser.parse_args()

    print("1. Loading data")
    sst = load_sst()
    domain = load_domain()
    domain_train, domain_test = train_test_split(
        domain, test_size=0.25, random_state=args.seed, stratify=domain["ranking_value"]
    )

    train = pd.concat([sst["train"], sst["dev"], domain_train], ignore_index=True)
    weights = np.concatenate(
        [
            np.ones(len(sst["train"]) + len(sst["dev"])),
            np.full(len(domain_train), args.domain_weight),
        ]
    )

    print("2. Fitting TF-IDF + logistic regression")
    vectorizer = TfidfVectorizer(
        lowercase=True,
        ngram_range=(1, 2),
        min_df=args.min_df,
        max_features=args.max_features,
        sublinear_tf=True,
        token_pattern=TOKEN_PATTERN,
    )
    x_train = vectorizer.fit_transform(train["text"])
    classifier = LogisticRegression(C=4.0, max_iter=2000, random_state=args.seed)
    classifier.fit(x_train, train["ranking_value"], sample_weight=weights)
    print(f"  vocabulary: {len(vectorizer.vocabulary_):,} terms, {len(classifier.classes_)} classes")

    print("3. Evaluating")

    def score(name: str, df: pd.DataFrame) -> dict:
        predicted = classifier.predict(vectorizer.transform(df["text"]))
        actual = df["ranking_value"].to_numpy()
        within_one = float(np.mean(np.abs(predicted - actual) <= 1))
        result = {
            "examples": len(df),
            "accuracy": round(float(accuracy_score(actual, predicted)), 4),
            "macro_f1": round(float(f1_score(actual, predicted, average="macro")), 4),
            "within_one_ranking": round(within_one, 4),
        }
        print(f"  {name}: {result}")
        return result

    metrics = {
        "sst5_test": score("SST-5 test", sst["test"]),
        "swatch_holdout": score("S-Watch holdout", domain_test),
    }
    print(classification_report(
        domain_test["ranking_value"],
        classifier.predict(vectorizer.transform(domain_test["text"])),
        zero_division=0,
    ))

    print("4. Exporting artifact")
    vocabulary = {term: int(idx) for term, idx in vectorizer.vocabulary_.items()}
    fixture_texts = [
        "A masterful, deeply moving film with two extraordinary performances.",
        "Watchable and competently made, though it never justifies its length.",
        "An absolute disaster. Nothing about this works on any level.",
        "Gripping and well paced, with a satisfying payoff despite predictable beats.",
        "Flat, poorly paced and weirdly joyless.",
    ]
    probabilities = classifier.predict_proba(vectorizer.transform(fixture_texts))

    write_artifact(
        "review_classifier.json",
        {
            "kind": "tfidf-logreg",
            "version": "tfidf-logreg-v1",
            "dataset": {
                "source": "SST-5 (Stanford Sentiment Treebank) + S-Watch critic reviews",
                "sst_train": len(sst["train"]) + len(sst["dev"]),
                "domain_train": len(domain_train),
                "domain_weight": args.domain_weight,
            },
            "metrics": metrics,
            "params": {
                "lowercase": True,
                "ngram_max": 2,
                "sublinear_tf": True,
                "norm": "l2",
                "token_pattern": TOKEN_PATTERN,
            },
            "classes": [
                {"ranking_value": int(c), "ranking_name": RANKING_NAMES[int(c)]} for c in classifier.classes_
            ],
            "vocabulary": vocabulary,
            "idf": round_matrix(vectorizer.idf_, 6),
            "coef": round_matrix(classifier.coef_, 6),
            "intercept": round_matrix(classifier.intercept_, 6),
            # Lets the Go tests prove the ported transform matches scikit-learn.
            "parity_fixtures": [
                {"text": text, "probabilities": round_matrix(probs, 6)}
                for text, probs in zip(fixture_texts, probabilities)
            ],
        },
    )
    print("Done.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
