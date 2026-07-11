"""Incremental on-disk cache for raw API responses.

Two namespaces under enrich-cache/:
  search/<lang>-<slug>.json   one wbsearchentities response per (query, lang)
  entity/<QID>.json           one wbgetentities response per entity

Entity responses are shared across artists (countries and genres recur), so a
rerun only reaches the network for artists (or referenced entities) not seen
before. --force-refresh makes reads miss so everything is refetched and
rewritten.
"""

from __future__ import annotations

import hashlib
import json
import re
from pathlib import Path
from typing import Any

_SLUG = re.compile(r"[^a-z0-9]+")


def _slug(text: str) -> str:
    base = _SLUG.sub("-", text.lower()).strip("-")[:60]
    digest = hashlib.sha1(text.encode("utf-8")).hexdigest()[:10]
    return f"{base}-{digest}" if base else digest


class Cache:
    def __init__(self, root: Path, force_refresh: bool = False) -> None:
        self.root = root
        self.force_refresh = force_refresh
        self.search_dir = root / "search"
        self.entity_dir = root / "entity"
        self.search_dir.mkdir(parents=True, exist_ok=True)
        self.entity_dir.mkdir(parents=True, exist_ok=True)
        self.hits = 0
        self.misses = 0

    def _read(self, path: Path) -> dict[str, Any] | None:
        if self.force_refresh or not path.exists():
            return None
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
            self.hits += 1
            return data
        except (json.JSONDecodeError, OSError):
            return None

    @staticmethod
    def _write(path: Path, data: dict[str, Any]) -> None:
        path.write_text(json.dumps(data, ensure_ascii=False), encoding="utf-8")

    def get_search(self, query: str, lang: str) -> dict[str, Any] | None:
        return self._read(self.search_dir / f"{lang}-{_slug(query)}.json")

    def set_search(self, query: str, lang: str, data: dict[str, Any]) -> None:
        self._write(self.search_dir / f"{lang}-{_slug(query)}.json", data)

    def get_entity(self, qid: str) -> dict[str, Any] | None:
        return self._read(self.entity_dir / f"{qid}.json")

    def set_entity(self, qid: str, data: dict[str, Any]) -> None:
        self._write(self.entity_dir / f"{qid}.json", data)
