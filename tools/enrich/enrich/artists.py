"""Extract and clean distinct artists from the listening history.

Mirrors internal/metadata/artist.go: a histlog artist field may hold a
multi-artist credit string. We cut at the earliest separator to get the
primary artist for resolution, but keep the raw string as the join key so
the enriched output maps back onto exactly what the report already stores.
"""

from __future__ import annotations

import json
from pathlib import Path

# Separators mark the boundary after which a credit string stops being the
# primary artist ("A, B", "A / B", "A feat B", "A & C", "A; B"). The
# leading-space forms avoid cutting names that legitimately contain the
# characters (e.g. "AC/DC" has no spaces around its slash).
SEPARATORS = [",", " / ", " feat", " Feat", " FEAT", " & ", ";"]


def clean_artist(s: str) -> str:
    """Reduce a multi-artist credit string to its first artist."""
    s = s.strip()
    cut = len(s)
    for sep in SEPARATORS:
        i = s.find(sep)
        if 0 <= i < cut:
            cut = i
    return s[:cut].strip()


def _iter_artists(path: Path):
    if not path.exists():
        return
    with path.open(encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            artist = obj.get("artist")
            if isinstance(artist, str) and artist.strip():
                yield artist


def distinct_artists(history: Path, prefs: Path | None = None) -> dict[str, str]:
    """Return an ordered mapping {raw credit string -> primary artist}.

    The raw string is the join key used by the report. Insertion order follows
    first appearance in the history, then any prefs-only artists.
    """
    mapping: dict[str, str] = {}
    for raw in _iter_artists(history):
        if raw not in mapping:
            mapping[raw] = clean_artist(raw)
    if prefs is not None:
        for raw in _iter_artists(prefs):
            if raw not in mapping:
                mapping[raw] = clean_artist(raw)
    return mapping
