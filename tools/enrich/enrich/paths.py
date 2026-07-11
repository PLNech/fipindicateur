"""Resolve the fipindicateur data directory, honoring XDG_DATA_HOME."""

from __future__ import annotations

import os
from pathlib import Path


def data_dir(override: str | None = None) -> Path:
    """Return the fipindicateur data directory.

    Precedence: explicit override, then $XDG_DATA_HOME/fipindicateur, then
    ~/.local/share/fipindicateur (the XDG default).
    """
    if override:
        return Path(override).expanduser()
    xdg = os.environ.get("XDG_DATA_HOME")
    if xdg:
        return Path(xdg).expanduser() / "fipindicateur"
    return Path.home() / ".local" / "share" / "fipindicateur"


def history_path(base: Path) -> Path:
    return base / "history.jsonl"


def prefs_path(base: Path) -> Path:
    return base / "prefs.jsonl"


def cache_dir(base: Path) -> Path:
    return base / "enrich-cache"


def output_path(base: Path) -> Path:
    return base / "enriched.json"
