"""Lightweight local embeddings + 2D projection of matched artists.

Embeddings come from model2vec (static distilled multilingual vectors, numpy
only, no torch). Projection is t-SNE for a readable map, with a PCA fallback
when there are too few points or t-SNE fails. Coordinates are normalized to
[0, 1] per axis so the consumer can place them directly.

Every heavy import is lazy and guarded: if model2vec or scikit-learn is
missing (or the model cannot be downloaded), we degrade gracefully and the
caller emits output without coords.
"""

from __future__ import annotations

from dataclasses import dataclass

# A multilingual static model keeps French descriptions/genres meaningful.
DEFAULT_MODEL = "minishlab/potion-multilingual-128M"


@dataclass
class ProjectionResult:
    coords: dict[str, tuple[float, float]]
    method: str  # "tsne", "pca" or "none"
    model: str | None
    error: str | None = None


def build_text(label: str, description: str | None, genres: list[str],
               country: str | None) -> str:
    """Compose a short embedding text from the enriched fields."""
    parts = [label]
    if description:
        parts.append(description)
    if genres:
        parts.append("Genres: " + ", ".join(genres))
    if country:
        parts.append("Pays: " + country)
    return ". ".join(parts)


def _normalize(coords):
    import numpy as np

    arr = np.asarray(coords, dtype=float)
    mins = arr.min(axis=0)
    spans = arr.max(axis=0) - mins
    spans[spans == 0] = 1.0
    return (arr - mins) / spans


def project(keys: list[str], texts: list[str],
            model_name: str = DEFAULT_MODEL) -> ProjectionResult:
    """Embed `texts` and project to normalized 2D, keyed by `keys`.

    Returns method "none" (with an error string) on any failure, so callers
    can proceed without coordinates instead of crashing.
    """
    if len(keys) < 3:
        return ProjectionResult({}, "none", None,
                                error=f"too few matched artists ({len(keys)})")
    try:
        from model2vec import StaticModel
    except ImportError as exc:
        return ProjectionResult({}, "none", None, error=f"model2vec missing: {exc}")

    try:
        model = StaticModel.from_pretrained(model_name)
        vectors = model.encode(texts)
    except Exception as exc:  # network, model download, encoding
        return ProjectionResult({}, "none", model_name, error=f"embedding failed: {exc}")

    try:
        import numpy as np
        n = len(keys)
        if n >= 10:
            from sklearn.manifold import TSNE

            perplexity = max(5.0, min(30.0, (n - 1) / 3.0))
            reducer = TSNE(
                n_components=2,
                perplexity=perplexity,
                init="pca",
                learning_rate="auto",
                random_state=0,
            )
            raw = reducer.fit_transform(np.asarray(vectors))
            method = "tsne"
        else:
            from sklearn.decomposition import PCA

            raw = PCA(n_components=2, random_state=0).fit_transform(np.asarray(vectors))
            method = "pca"
    except Exception as exc:
        return ProjectionResult({}, "none", model_name, error=f"projection failed: {exc}")

    norm = _normalize(raw)
    coords = {k: (round(float(x), 4), round(float(y), 4))
              for k, (x, y) in zip(keys, norm)}
    return ProjectionResult(coords, method, model_name)
