"""Resolver tests with a fully mocked client. No network is ever touched."""

import pytest

from enrich.wikidata import Resolver


class FakeClient:
    """Serves canned wbsearchentities / wbgetentities responses."""

    def __init__(self):
        self.calls = 0
        self.searches = {
            ("Air", "fr"): [{"id": "Q189598", "label": "Air"}],
            ("Ambiguous", "fr"): [
                {"id": "Q100", "label": "Ambiguous"},   # not musical
                {"id": "Q200", "label": "Ambiguous"},   # musical (index 1)
            ],
            ("NothingMusical", "fr"): [{"id": "Q300", "label": "x"}],
            # "Void" absent in fr and en -> unmatched
        }
        self.entities = {
            "Q189598": _band("Q189598", "Air", "groupe francais",
                             genres=["Q9778"], country="Q142", inception="+1995-01-01T00:00:00Z"),
            "Q100": _human("Q100", "Ambiguous", occupation="Q901"),  # non-music job
            "Q200": _human("Q200", "Ambiguous the singer", occupation="Q177220",
                           country="Q30", birth="+1970-00-00T00:00:00Z"),
            "Q300": _human("Q300", "Not musical", occupation="Q901"),
            "Q9778": _label_only("Q9778", "musique electronique"),
            "Q142": _country("Q142", "France", "FR"),
            "Q30": _country("Q30", "Etats-Unis", "US"),
        }

    def get_json(self, url, params):
        self.calls += 1
        action = params["action"]
        if action == "wbsearchentities":
            key = (params["search"], params["language"])
            return {"search": self.searches.get(key, [])}
        if action == "wbgetentities":
            ids = params["ids"].split("|")
            return {"entities": {i: self.entities.get(i, {}) for i in ids}}
        raise AssertionError(f"unexpected action {action}")


class FakeCache:
    """In-memory cache; always misses on first read."""

    def __init__(self):
        self.force_refresh = False
        self.hits = 0
        self._search = {}
        self._entity = {}

    def get_search(self, q, lang):
        return self._search.get((q, lang))

    def set_search(self, q, lang, data):
        self._search[(q, lang)] = data

    def get_entity(self, qid):
        return self._entity.get(qid)

    def set_entity(self, qid, data):
        self._entity[qid] = data


def _band(qid, label, desc, genres=None, country=None, inception=None):
    claims = {"P31": [_item("Q215380")]}  # musical group
    if genres:
        claims["P136"] = [_item(g) for g in genres]
    if country:
        claims["P495"] = [_item(country)]
    if inception:
        claims["P571"] = [_time(inception)]
    return {
        "id": qid,
        "labels": {"fr": {"value": label}},
        "descriptions": {"fr": {"value": desc}},
        "claims": claims,
        "sitelinks": {"frwiki": {"url": f"https://fr.wikipedia.org/wiki/{label}"}},
    }


def _human(qid, label, occupation=None, country=None, birth=None):
    claims = {"P31": [_item("Q5")]}  # human
    if occupation:
        claims["P106"] = [_item(occupation)]
    if country:
        claims["P27"] = [_item(country)]
    if birth:
        claims["P569"] = [_time(birth)]
    return {
        "id": qid,
        "labels": {"en": {"value": label}},
        "descriptions": {"en": {"value": "person"}},
        "claims": claims,
        "sitelinks": {},
    }


def _label_only(qid, label):
    return {"id": qid, "labels": {"fr": {"value": label}}, "claims": {}}


def _country(qid, label, iso):
    return {"id": qid, "labels": {"fr": {"value": label}},
            "claims": {"P297": [_string(iso)]}}


def _item(qid):
    return {"mainsnak": {"datavalue": {"value": {"id": qid}}}}


def _string(s):
    return {"mainsnak": {"datavalue": {"value": s}}}


def _time(t):
    return {"mainsnak": {"datavalue": {"value": {"time": t}}}}


@pytest.fixture
def resolver():
    return Resolver(FakeClient(), FakeCache())


def test_resolves_band_high_confidence(resolver):
    r = resolver.resolve("Air")
    assert r.qid == "Q189598"
    assert r.label == "Air"
    assert r.confidence == 0.95
    assert r.low_confidence is False
    assert r.genre_qids == ["Q9778"]
    assert r.country_qid == "Q142"
    assert r.year == 1995
    assert r.wikipedia.startswith("https://fr.wikipedia.org/")


def test_prefers_musical_candidate_over_top_hit(resolver):
    r = resolver.resolve("Ambiguous")
    assert r.qid == "Q200"          # the singer, not the top non-music hit
    assert r.confidence == 0.85     # first music hit at index 1
    assert r.year == 1970
    assert r.country_qid == "Q30"


def test_low_confidence_when_no_musical_candidate(resolver):
    r = resolver.resolve("NothingMusical")
    assert r.qid == "Q300"
    assert r.low_confidence is True
    assert r.confidence == 0.4


def test_unmatched_when_no_hits(resolver):
    r = resolver.resolve("Void")
    assert r.qid is None
    assert r.confidence == 0.0


def test_resolve_labels_returns_genre_and_country_iso(resolver):
    labels, countries = resolver.resolve_labels(["Q9778", "Q142", "Q30"])
    assert labels["Q9778"] == "musique electronique"
    assert countries["Q142"] == ("France", "FR")
    assert countries["Q30"] == ("Etats-Unis", "US")


def test_caching_avoids_second_network_call():
    client = FakeClient()
    r = Resolver(client, FakeCache())
    r.resolve("Air")
    first = client.calls
    r.resolve("Air")
    # Second resolve is fully served from cache: no new client calls.
    assert client.calls == first
