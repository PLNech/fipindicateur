"""Command-line entrypoint: read history, resolve, embed, write enriched.json.

Privacy: this reads the user's real listening history. Nothing is printed
except aggregates (counts, rates, cardinalities). Artist and title strings
never reach stdout.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from . import __version__, paths
from .artists import distinct_artists
from .cache import Cache
from .embed import build_text, project
from .httpclient import HttpClient, NetworkError
from .schema import build, validate
from .wikidata import Resolver


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    p = argparse.ArgumentParser(
        prog="enrich",
        description="Enrich fipindicateur listening history with Wikidata "
                    "metadata and a 2D artist map (local, read-only APIs).",
    )
    p.add_argument("--data-dir", help="fipindicateur data directory "
                   "(default: $XDG_DATA_HOME/fipindicateur or ~/.local/share/fipindicateur)")
    p.add_argument("--no-embed", action="store_true",
                   help="skip embeddings and omit coords")
    p.add_argument("--force-refresh", action="store_true",
                   help="ignore the cache and refetch every artist")
    p.add_argument("--verbose", action="store_true", help="progress logging (no PII)")
    p.add_argument("--version", action="version", version=f"enrich {__version__}")
    return p.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    base = paths.data_dir(args.data_dir)
    hist = paths.history_path(base)
    if not hist.exists():
        print(f"error: history not found at {hist}", file=sys.stderr)
        return 2

    mapping = distinct_artists(hist, paths.prefs_path(base))
    n = len(mapping)
    if args.verbose:
        print(f"distinct raw artist strings: {n}")

    client = HttpClient(verbose=args.verbose)
    cache = Cache(paths.cache_dir(base), force_refresh=args.force_refresh)
    resolver = Resolver(client, cache, verbose=args.verbose)

    resolutions: dict[str, object] = {}
    network_down = False
    for i, (raw, primary) in enumerate(mapping.items(), 1):
        query = primary or raw
        try:
            res = resolver.resolve(query)
        except NetworkError as exc:
            network_down = True
            if args.verbose:
                print(f"  [{i}/{n}] network error, emitting from cache only: {exc}")
            # Best effort: a cached resolution may still exist for reruns.
            from .wikidata import Resolution
            res = Resolution(query=query)
        res.query = raw  # keep the raw join key
        resolutions[raw] = res
        if args.verbose and i % 25 == 0:
            done = sum(1 for r in resolutions.values() if getattr(r, "qid", None))
            print(f"  [{i}/{n}] matched so far: {done}")

    # Second pass: resolve genre and country QID labels/ISO codes.
    ref_qids: list[str] = []
    for res in resolutions.values():
        ref_qids.extend(res.genre_qids)
        if res.country_qid:
            ref_qids.append(res.country_qid)
    genre_labels: dict[str, str] = {}
    country_labels: dict[str, tuple[str, str | None]] = {}
    if ref_qids:
        try:
            genre_labels, country_labels = resolver.resolve_labels(ref_qids)
        except NetworkError as exc:
            network_down = True
            if args.verbose:
                print(f"  label pass network error, labels may be partial: {exc}")

    # Embedding + projection.
    coords = None
    proj = None
    if not args.no_embed:
        keys, texts = [], []
        for raw, res in resolutions.items():
            if not res.qid:
                continue
            genres = [genre_labels.get(q, q) for q in res.genre_qids]
            country = None
            if res.country_qid:
                country = country_labels.get(res.country_qid, (None, None))[0]
            keys.append(raw)
            texts.append(build_text(res.label or "", res.description, genres, country))
        proj = project(keys, texts)
        if proj.method != "none":
            coords = proj.coords

    doc = build(resolutions, genre_labels, country_labels, coords)
    errors = validate(doc)
    if errors:
        print("schema validation FAILED:", file=sys.stderr)
        for e in errors[:20]:
            print(f"  - {e}", file=sys.stderr)
        return 1

    out = paths.output_path(base)
    out.write_text(json.dumps(doc, ensure_ascii=False, indent=2), encoding="utf-8")

    _report(doc, coords, proj, cache, client, network_down)
    return 0


def _report(doc, coords, proj, cache, client, network_down) -> None:
    """Print privacy-safe aggregates only."""
    arts = doc["artists"].values()
    genre_counts = [len(a["genres"]) for a in arts if a["qid"]]
    with_country = sum(1 for a in arts if a["qid"] and a.get("country"))
    with_code = sum(1 for a in arts if a["qid"] and a.get("country_code"))
    with_year = sum(1 for a in arts if a["qid"] and a.get("year") is not None)
    with_wiki = sum(1 for a in arts if a["qid"] and a.get("wikipedia"))
    low_conf = sum(1 for a in arts if a.get("low_confidence"))
    distinct_genres = {g for a in arts if a["qid"] for g in a["genres"]}
    distinct_countries = {a["country_code"] for a in arts
                          if a.get("country_code")}
    mean_genres = (sum(genre_counts) / len(genre_counts)) if genre_counts else 0.0
    matched = doc["n_matched"]

    print("=" * 56)
    print("enrichment complete (aggregates only, no PII)")
    print(f"  artists (raw strings):   {doc['n_artists']}")
    print(f"  matched to Wikidata:     {matched} "
          f"({doc['match_rate'] * 100:.1f}%)")
    print(f"  low-confidence guesses:  {low_conf}")
    print(f"  mean genres / matched:   {mean_genres:.2f}")
    print(f"  distinct genres:         {len(distinct_genres)}")
    print(f"  with country:            {with_country}  "
          f"(ISO code: {with_code}, distinct countries: {len(distinct_countries)})")
    print(f"  with inception/birth yr: {with_year}")
    print(f"  with wikipedia link:     {with_wiki}")
    if proj is not None:
        if proj.method == "none":
            print(f"  projection:              skipped ({proj.error})")
        else:
            print(f"  projection:              {proj.method} on "
                  f"{len(coords)} points via {proj.model}")
    else:
        print("  projection:              disabled (--no-embed)")
    print(f"  cache hits:              {cache.hits}")
    print(f"  network requests:        {client.calls}")
    if network_down:
        print("  NOTE: network errors occurred; output built from cache "
              "where possible (match_rate is honest).")
    print("=" * 56)


if __name__ == "__main__":
    raise SystemExit(main())
