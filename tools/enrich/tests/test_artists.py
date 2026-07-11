import json

from enrich.artists import clean_artist, distinct_artists


def test_clean_splits_common_separators():
    assert clean_artist("Daft Punk, Pharrell") == "Daft Punk"
    assert clean_artist("Air / Beck") == "Air"
    assert clean_artist("Gorillaz feat. De La Soul") == "Gorillaz"
    assert clean_artist("Simon & Garfunkel") == "Simon"
    assert clean_artist("Artist A; Artist B") == "Artist A"


def test_clean_preserves_names_without_spaced_separators():
    # No spaces around the slash, so AC/DC is not cut.
    assert clean_artist("AC/DC") == "AC/DC"
    # Trailing/leading whitespace trimmed.
    assert clean_artist("  Bjork  ") == "Bjork"


def test_clean_earliest_separator_wins():
    assert clean_artist("A & B, C") == "A"


def test_distinct_preserves_raw_key_and_order(tmp_path):
    lines = [
        {"v": 1, "artist": "Daft Punk, Pharrell", "title": "x"},
        {"v": 1, "artist": "Air / Beck", "title": "y"},
        {"v": 1, "artist": "Daft Punk, Pharrell", "title": "z"},  # dup raw
        {"v": 1, "title": "no artist"},  # skipped
    ]
    hist = tmp_path / "history.jsonl"
    hist.write_text("\n".join(json.dumps(o) for o in lines), encoding="utf-8")

    mapping = distinct_artists(hist, tmp_path / "missing-prefs.jsonl")
    assert list(mapping.keys()) == ["Daft Punk, Pharrell", "Air / Beck"]
    assert mapping["Daft Punk, Pharrell"] == "Daft Punk"
    assert mapping["Air / Beck"] == "Air"


def test_distinct_includes_prefs_only_artists(tmp_path):
    hist = tmp_path / "history.jsonl"
    hist.write_text(json.dumps({"v": 1, "artist": "Air"}), encoding="utf-8")
    prefs = tmp_path / "prefs.jsonl"
    prefs.write_text(
        "\n".join([
            json.dumps({"v": 1, "artist": "Air", "verdict": "up"}),  # already known
            json.dumps({"v": 1, "artist": "Phoenix", "verdict": "down"}),
        ]),
        encoding="utf-8",
    )
    mapping = distinct_artists(hist, prefs)
    assert set(mapping) == {"Air", "Phoenix"}


def test_distinct_tolerates_bad_lines(tmp_path):
    hist = tmp_path / "history.jsonl"
    hist.write_text("not json\n" + json.dumps({"v": 1, "artist": "Air"}) + "\n",
                    encoding="utf-8")
    mapping = distinct_artists(hist, None)
    assert list(mapping) == ["Air"]
