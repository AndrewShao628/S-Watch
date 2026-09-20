"""Shared helpers for the S-Watch training scripts."""

from __future__ import annotations

import json
import zipfile
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path

import numpy as np
import requests

ML_DIR = Path(__file__).resolve().parent.parent
REPO_DIR = ML_DIR.parent
CACHE_DIR = ML_DIR / "data" / "cache"
SEED_DIR = REPO_DIR / "server" / "internal" / "seed" / "data"
ARTIFACT_DIR = REPO_DIR / "server" / "internal" / "model" / "artifacts"

# S-Watch genre name -> MovieLens genre names used for the cold-start projection.
# S-Watch has a few genres MovieLens doesn't model, so they map to the closest match.
GENRE_MAP: dict[str, list[str]] = {
    "Action": ["Action"],
    "Adventure": ["Adventure"],
    "Animation": ["Animation"],
    "Biography": ["Drama"],
    "Comedy": ["Comedy"],
    "Crime": ["Crime"],
    "Drama": ["Drama"],
    "Family": ["Children"],
    "Fantasy": ["Fantasy"],
    "History": ["Drama", "War"],
    "Horror": ["Horror"],
    "Music": ["Musical"],
    "Mystery": ["Mystery"],
    "Romance": ["Romance"],
    "Sci-Fi": ["Sci-Fi"],
    "Thriller": ["Thriller"],
}


@dataclass(frozen=True)
class CatalogueMovie:
    imdb_id: str
    title: str
    year: int
    genres: tuple[str, ...]


def load_catalogue() -> list[CatalogueMovie]:
    """Read the movies the S-Watch server seeds into MongoDB."""
    raw = json.loads((SEED_DIR / "movies.json").read_text(encoding="utf-8"))
    return [
        CatalogueMovie(
            imdb_id=m["imdb_id"],
            title=m["title"],
            year=int(m["year"]),
            genres=tuple(g["genre_name"] for g in m["genre"]),
        )
        for m in raw
    ]


def download(url: str, dest: Path, *, force: bool = False) -> Path:
    """Download `url` to `dest` once, reusing the cached copy on later runs."""
    dest.parent.mkdir(parents=True, exist_ok=True)
    if dest.exists() and not force:
        print(f"  using cached {dest.name} ({dest.stat().st_size / 1e6:.1f} MB)")
        return dest

    print(f"  downloading {url}")
    with requests.get(url, stream=True, timeout=120) as resp:
        resp.raise_for_status()
        tmp = dest.with_suffix(dest.suffix + ".part")
        with tmp.open("wb") as fh:
            for chunk in resp.iter_content(chunk_size=1 << 16):
                fh.write(chunk)
        tmp.replace(dest)
    print(f"  saved {dest.name} ({dest.stat().st_size / 1e6:.1f} MB)")
    return dest


def extract(archive: Path, member_suffix: str, out_dir: Path) -> dict[str, Path]:
    """Extract archive members whose name ends with `member_suffix`."""
    out_dir.mkdir(parents=True, exist_ok=True)
    found: dict[str, Path] = {}
    with zipfile.ZipFile(archive) as zf:
        for info in zf.infolist():
            if info.is_dir() or not info.filename.endswith(member_suffix):
                continue
            name = Path(info.filename).name
            target = out_dir / name
            if not target.exists():
                target.write_bytes(zf.read(info))
            found[name] = target
    return found


def round_matrix(matrix: np.ndarray, decimals: int = 5) -> list:
    """Round to keep the exported JSON artifacts compact."""
    return np.round(np.asarray(matrix, dtype=np.float64), decimals).tolist()


def write_artifact(name: str, payload: dict) -> Path:
    """Write a model artifact for the Go server to embed."""
    ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)
    path = ARTIFACT_DIR / name
    payload = {"trained_at": datetime.now(timezone.utc).isoformat(timespec="seconds"), **payload}
    path.write_text(json.dumps(payload, separators=(",", ":"), sort_keys=False), encoding="utf-8")
    print(f"  wrote {path.relative_to(REPO_DIR)} ({path.stat().st_size / 1e6:.2f} MB)")
    return path
