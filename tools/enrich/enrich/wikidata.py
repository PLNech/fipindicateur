"""Wikidata resolution: artist -> QID + claims, with plausibility scoring.

Flow per artist:
  1. wbsearchentities (fr, then en fallback) to get candidate items.
  2. wbgetentities for the top candidates' claims.
  3. Pick the first candidate that looks musical (P31 a band type, or a human
     P106 with a music occupation). If none looks musical we keep the top hit
     but flag low confidence, so the consumer can count honest best guesses.

All entity fetches use one rich props set, so a single cached blob per QID
serves artists, genres and countries alike.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import Any

from .cache import Cache
from .httpclient import HttpClient, NetworkError

API = "https://www.wikidata.org/w/api.php"
ENTITY_PROPS = "labels|descriptions|claims|sitelinks/urls"
LANGS = "fr|en"

# Instance-of (P31) values that mark a musical group / ensemble.
MUSIC_GROUP_QIDS = {
    "Q215380",   # musical group (band)
    "Q2088357",  # musical ensemble
    "Q9212979",  # musical duo
    "Q281643",   # boy band
}

# Occupation (P106) values that mark a music-maker.
MUSIC_OCCUPATION_QIDS = {
    "Q639669",    # musician
    "Q177220",    # singer
    "Q488205",    # singer-songwriter
    "Q753110",    # songwriter
    "Q36834",     # composer
    "Q158852",    # conductor
    "Q855091",    # guitarist
    "Q386854",    # drummer
    "Q1259917",   # bassist
    "Q486748",    # pianist
    "Q183945",    # record producer
    "Q806349",    # bandleader
    "Q2252262",   # rapper
    "Q130857",    # disc jockey
    "Q1470478",   # keyboardist
    "Q12800682",  # multi-instrumentalist
    "Q3455803",   # instrumentalist
}

_YEAR = re.compile(r"^[+]?(\d{1,4})")


@dataclass
class Resolution:
    query: str
    qid: str | None = None
    label: str | None = None
    confidence: float = 0.0
    low_confidence: bool = False
    description: str | None = None
    genre_qids: list[str] = field(default_factory=list)
    country_qid: str | None = None
    year: int | None = None
    wikipedia: str | None = None


def _claim_qids(claims: dict[str, Any], prop: str) -> list[str]:
    out: list[str] = []
    for st in claims.get(prop, []):
        val = st.get("mainsnak", {}).get("datavalue", {}).get("value")
        if isinstance(val, dict) and "id" in val:
            out.append(val["id"])
    return out


def _claim_strings(claims: dict[str, Any], prop: str) -> list[str]:
    out: list[str] = []
    for st in claims.get(prop, []):
        val = st.get("mainsnak", {}).get("datavalue", {}).get("value")
        if isinstance(val, str):
            out.append(val)
    return out


def _claim_year(claims: dict[str, Any], prop: str) -> int | None:
    for st in claims.get(prop, []):
        val = st.get("mainsnak", {}).get("datavalue", {}).get("value")
        if isinstance(val, dict) and "time" in val:
            m = _YEAR.match(val["time"])
            if m:
                y = int(m.group(1))
                if 0 < y <= 2100:
                    return y
    return None


def _pref_label(entity: dict[str, Any], kind: str = "labels") -> str | None:
    node = entity.get(kind, {})
    for lang in ("fr", "en"):
        v = node.get(lang, {}).get("value")
        if v:
            return v
    # last resort: any label present
    for v in node.values():
        if isinstance(v, dict) and v.get("value"):
            return v["value"]
    return None


class Resolver:
    def __init__(self, client: HttpClient, cache: Cache, verbose: bool = False) -> None:
        self.client = client
        self.cache = cache
        self.verbose = verbose

    # -- primitives ---------------------------------------------------------

    def _search(self, query: str, lang: str) -> list[dict[str, Any]]:
        cached = self.cache.get_search(query, lang)
        if cached is None:
            cached = self.client.get_json(
                API,
                {
                    "action": "wbsearchentities",
                    "search": query,
                    "language": lang,
                    "uselang": lang,
                    "type": "item",
                    "limit": 5,
                    "format": "json",
                },
            )
            self.cache.set_search(query, lang, cached)
        return cached.get("search", []) or []

    def get_entities(self, qids: list[str]) -> dict[str, dict[str, Any]]:
        """Fetch entities by QID, one cached blob each, batching network misses."""
        result: dict[str, dict[str, Any]] = {}
        missing: list[str] = []
        for qid in qids:
            blob = self.cache.get_entity(qid)
            if blob is not None:
                result[qid] = blob
            else:
                missing.append(qid)
        for i in range(0, len(missing), 50):
            batch = missing[i : i + 50]
            data = self.client.get_json(
                API,
                {
                    "action": "wbgetentities",
                    "ids": "|".join(batch),
                    "props": ENTITY_PROPS,
                    "languages": LANGS,
                    "format": "json",
                },
            )
            entities = data.get("entities", {})
            for qid in batch:
                ent = entities.get(qid, {})
                self.cache.set_entity(qid, ent)
                result[qid] = ent
        return result

    # -- resolution ---------------------------------------------------------

    @staticmethod
    def _is_music(entity: dict[str, Any]) -> bool:
        claims = entity.get("claims", {})
        p31 = set(_claim_qids(claims, "P31"))
        p106 = set(_claim_qids(claims, "P106"))
        if p31 & MUSIC_GROUP_QIDS:
            return True
        if p106 & MUSIC_OCCUPATION_QIDS:
            return True
        return False

    def resolve(self, query: str) -> Resolution:
        hits = self._search(query, "fr")
        if not hits:
            hits = self._search(query, "en")
        if not hits:
            return Resolution(query=query)

        top = hits[:3]
        entities = self.get_entities([h["id"] for h in top])

        chosen: dict[str, Any] | None = None
        confidence = 0.0
        low = False
        for i, hit in enumerate(top):
            ent = entities.get(hit["id"])
            if ent and self._is_music(ent):
                chosen = ent
                confidence = round(max(0.6, 0.95 - 0.1 * i), 2)
                break
        if chosen is None:
            chosen = entities.get(top[0]["id"]) or {}
            confidence = 0.4
            low = True

        claims = chosen.get("claims", {})
        country_qids = _claim_qids(claims, "P495") or _claim_qids(claims, "P27")
        sitelinks = chosen.get("sitelinks", {})
        wiki = None
        for site in ("frwiki", "enwiki"):
            if site in sitelinks and sitelinks[site].get("url"):
                wiki = sitelinks[site]["url"]
                break
        return Resolution(
            query=query,
            qid=chosen.get("id"),
            label=_pref_label(chosen, "labels"),
            confidence=confidence,
            low_confidence=low,
            description=_pref_label(chosen, "descriptions"),
            genre_qids=_claim_qids(claims, "P136"),
            country_qid=country_qids[0] if country_qids else None,
            year=_claim_year(claims, "P571") or _claim_year(claims, "P569"),
            wikipedia=wiki,
        )

    # -- label resolution for referenced entities ---------------------------

    def resolve_labels(
        self, qids: list[str]
    ) -> tuple[dict[str, str], dict[str, tuple[str, str | None]]]:
        """Return ({qid: label} for genres/any, {qid: (label, iso2)} for countries).

        Both are read from the same rich entity blobs, so callers get genre
        labels and country ISO codes from one fetch.
        """
        uniq = sorted(set(q for q in qids if q))
        entities = self.get_entities(uniq)
        labels: dict[str, str] = {}
        countries: dict[str, tuple[str, str | None]] = {}
        for qid, ent in entities.items():
            label = _pref_label(ent, "labels") or qid
            labels[qid] = label
            iso = _claim_strings(ent.get("claims", {}), "P297")
            countries[qid] = (label, iso[0] if iso else None)
        return labels, countries
