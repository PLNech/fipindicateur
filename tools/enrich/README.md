# enrich

Local companion tool for fipindicateur. It reads your listening history,
resolves each artist against public Wikidata/Wikipedia APIs, and writes an
enriched metadata file (`enriched.json`) that the stats report can consume. It
also builds a lightweight 2D "artist map" from local text embeddings.

The Go binary stays stdlib-only; this enrichment lives here on purpose, as a
separate opt-in Python tool. Nothing leaves your machine except polite,
read-only API calls to Wikidata and Wikipedia.

## What it produces

`~/.local/share/fipindicateur/enriched.json` (schema v1):

```json
{
  "v": 1,
  "generated_at": "2026-07-11T12:00:00+00:00",
  "n_artists": 283,
  "n_matched": 250,
  "match_rate": 0.883,
  "artists": {
    "<raw artist string from history>": {
      "qid": "Q189598", "label": "Air", "confidence": 0.95,
      "description": "groupe francais de musique electronique",
      "genres": ["musique electronique"], "country": "France", "country_code": "FR",
      "year": 1995, "wikipedia": "https://fr.wikipedia.org/wiki/Air_(groupe)",
      "coords": [0.42, 0.77]
    }
  }
}
```

Unmatched artists still appear, with `qid: null`, so the consumer can count
matches honestly. Best guesses with weak evidence carry `"low_confidence": true`.

The raw artist string (exactly as stored in `history.jsonl`) is the map key, so
the output joins straight back onto what the report already has.

## Install

Requires Python 3.11+ and [poetry](https://python-poetry.org/).

```
cd tools/enrich
poetry install
```

## Run

```
poetry run enrich                 # enrich, embed, project, write enriched.json
poetry run enrich --no-embed      # skip embeddings; omit coords
poetry run enrich --force-refresh # ignore the cache; refetch everything
poetry run enrich --verbose       # progress logging (aggregates only, no PII)
poetry run enrich --data-dir DIR  # use a different fipindicateur data dir
```

Expect a few minutes on first run: requests to Wikidata are paced to about one
per second out of politeness, and the embedding model downloads once.

## How it works

1. **Distinct artists.** The history `artist` field can hold a multi-artist
   credit string. We split on the same separators as the Go client
   (`internal/metadata/artist.go`: `,`, ` / `, ` feat`, ` & `, `;`) and resolve
   the primary artist, but keep the raw string as the join key.
2. **Wikidata resolution.** `wbsearchentities` (French, then English) finds
   candidates; `wbgetentities` fetches their claims. We pick the first
   candidate that looks musical (P31 a band type, or a human P106 with a music
   occupation). If none looks musical we keep the top hit but flag it low
   confidence. From the chosen entity we read P136 genres, P495/P27 country
   (with its ISO code from P297), P571/P569 year, the frwiki/enwiki link, and a
   French-preferred description.
3. **Incremental cache.** Every raw API response is cached under
   `~/.local/share/fipindicateur/enrich-cache/`. Reruns hit the network only for
   artists (and referenced entities) not seen before.
4. **Embeddings + projection.** For each matched artist we build a short text
   (description + genres + country), embed it with a static multilingual
   [model2vec](https://github.com/MinishLab/model2vec) model (numpy only, no
   torch), then project to 2D with t-SNE (PCA fallback for small sets) and
   normalize to `[0, 1]`.

## Graceful degradation

- Network down: the tool emits output from the cache with an honest
  `match_rate` and a note. Nothing crashes.
- `--no-embed`, or a missing embedding dependency, or a failed model download:
  the tool skips step 4 and omits `coords` (still writes everything else).

## Privacy

- Read-only. It reads `history.jsonl` (and `prefs.jsonl` if present) and writes
  `enriched.json` plus the cache. It never modifies your history.
- No telemetry. The only outbound traffic is to the public Wikidata/Wikipedia
  APIs, identified by a descriptive User-Agent.
- The tool prints aggregates only (counts, rates, cardinalities), never your
  artist or title strings.

## Tests

```
poetry run pytest
```

The network is fully mocked; tests never reach Wikidata.
