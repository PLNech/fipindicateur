"""Build and validate the enriched.json output (schema v1).

Kept dependency-free: a small hand-rolled validator, since the schema is tiny
and stable and the tool avoids pulling extra modules.
"""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

SCHEMA_VERSION = 1


def build(
    resolutions: dict[str, Any],
    genre_labels: dict[str, str],
    country_labels: dict[str, tuple[str, str | None]],
    coords: dict[str, tuple[float, float]] | None,
) -> dict[str, Any]:
    """Assemble the output document from per-artist Resolution objects.

    `resolutions` maps raw artist string -> Resolution. `coords` is None when
    embedding was skipped or failed, in which case no artist gets a coords key.
    """
    artists: dict[str, Any] = {}
    matched = 0
    for raw, res in resolutions.items():
        genres = [genre_labels.get(q, q) for q in res.genre_qids]
        country = None
        country_code = None
        if res.country_qid:
            label, iso = country_labels.get(res.country_qid, (None, None))
            country = label
            country_code = iso
        entry: dict[str, Any] = {
            "qid": res.qid,
            "label": res.label,
            "confidence": res.confidence,
            "description": res.description,
            "genres": genres,
            "country": country,
            "country_code": country_code,
            "year": res.year,
            "wikipedia": res.wikipedia,
        }
        if res.low_confidence:
            entry["low_confidence"] = True
        if coords is not None and raw in coords:
            entry["coords"] = list(coords[raw])
        if res.qid:
            matched += 1
        artists[raw] = entry

    n = len(resolutions)
    return {
        "v": SCHEMA_VERSION,
        "generated_at": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "n_artists": n,
        "n_matched": matched,
        "match_rate": round(matched / n, 4) if n else 0.0,
        "artists": artists,
    }


def validate(doc: dict[str, Any]) -> list[str]:
    """Return a list of schema violations (empty means valid)."""
    errors: list[str] = []

    def err(msg: str) -> None:
        errors.append(msg)

    if doc.get("v") != SCHEMA_VERSION:
        err(f"v must be {SCHEMA_VERSION}, got {doc.get('v')!r}")
    for key, typ in (("generated_at", str), ("n_artists", int),
                     ("n_matched", int), ("match_rate", (int, float))):
        if not isinstance(doc.get(key), typ):
            err(f"{key} missing or wrong type")
    artists = doc.get("artists")
    if not isinstance(artists, dict):
        err("artists must be an object")
        return errors

    if isinstance(doc.get("n_artists"), int) and len(artists) != doc["n_artists"]:
        err(f"n_artists ({doc['n_artists']}) != len(artists) ({len(artists)})")

    counted = 0
    for raw, e in artists.items():
        if not isinstance(e, dict):
            err(f"artist {raw!r}: not an object")
            continue
        qid = e.get("qid")
        if qid is not None and not (isinstance(qid, str) and qid.startswith("Q")):
            err(f"artist {raw!r}: qid must be null or a Q-id")
        if qid is not None:
            counted += 1
        if not isinstance(e.get("genres"), list):
            err(f"artist {raw!r}: genres must be a list")
        conf = e.get("confidence")
        if not isinstance(conf, (int, float)) or not (0.0 <= conf <= 1.0):
            err(f"artist {raw!r}: confidence must be in [0,1]")
        coords = e.get("coords")
        if coords is not None:
            if (not isinstance(coords, list) or len(coords) != 2
                    or not all(isinstance(c, (int, float)) and 0.0 <= c <= 1.0
                               for c in coords)):
                err(f"artist {raw!r}: coords must be two floats in [0,1]")
        for opt in ("label", "description", "country", "country_code", "wikipedia"):
            if opt in e and e[opt] is not None and not isinstance(e[opt], str):
                err(f"artist {raw!r}: {opt} must be string or null")
        if e.get("year") is not None and not isinstance(e["year"], int):
            err(f"artist {raw!r}: year must be int or null")

    if isinstance(doc.get("n_matched"), int) and counted != doc["n_matched"]:
        err(f"n_matched ({doc['n_matched']}) != counted with qid ({counted})")
    return errors
