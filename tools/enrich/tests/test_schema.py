from enrich.schema import build, validate
from enrich.wikidata import Resolution


def _matched():
    return Resolution(
        query="Air",
        qid="Q189598",
        label="Air",
        confidence=0.95,
        description="groupe francais",
        genre_qids=["Q1"],
        country_qid="Q142",
        year=1995,
        wikipedia="https://fr.wikipedia.org/wiki/Air_(groupe)",
    )


def test_build_matched_and_unmatched():
    resolutions = {
        "Air": _matched(),
        "Unknown Xyz": Resolution(query="Unknown Xyz"),  # no qid
    }
    genre_labels = {"Q1": "electronic"}
    country_labels = {"Q142": ("France", "FR")}
    coords = {"Air": (0.42, 0.77)}

    doc = build(resolutions, genre_labels, country_labels, coords)

    assert doc["v"] == 1
    assert doc["n_artists"] == 2
    assert doc["n_matched"] == 1
    assert doc["match_rate"] == 0.5
    air = doc["artists"]["Air"]
    assert air["genres"] == ["electronic"]
    assert air["country"] == "France"
    assert air["country_code"] == "FR"
    assert air["coords"] == [0.42, 0.77]
    unk = doc["artists"]["Unknown Xyz"]
    assert unk["qid"] is None
    assert "coords" not in unk
    assert validate(doc) == []


def test_low_confidence_flag_and_no_coords_when_disabled():
    r = Resolution(query="Foo", qid="Q9", label="Foo", confidence=0.4,
                   low_confidence=True)
    doc = build({"Foo": r}, {}, {}, None)  # coords disabled
    assert doc["artists"]["Foo"]["low_confidence"] is True
    assert "coords" not in doc["artists"]["Foo"]
    assert validate(doc) == []


def test_validate_catches_bad_qid_and_counts():
    doc = build({"Air": _matched()}, {"Q1": "electronic"},
                {"Q142": ("France", "FR")}, None)
    doc["artists"]["Air"]["qid"] = "not-a-qid"
    errors = validate(doc)
    assert any("qid" in e for e in errors)


def test_validate_catches_count_mismatch():
    doc = build({"Air": _matched()}, {}, {}, None)
    doc["n_matched"] = 5
    assert any("n_matched" in e for e in validate(doc))


def test_validate_catches_bad_coords():
    doc = build({"Air": _matched()}, {}, {}, {"Air": (1.5, 0.2)})
    assert any("coords" in e for e in validate(doc))
