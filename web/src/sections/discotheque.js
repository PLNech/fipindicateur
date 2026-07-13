// La discotheque (conditional: data.discotheque). The searchable archive of
// everything you heard: one row per track line, reverse-chronological, with a
// fulltext search (diacritic-insensitive) and facet chips (genre, decennie,
// station, emission) that AND with the query. Genres and country come from the
// artist-level enrichment; the section states that honestly, and captions its
// own staleness. Rendered in capped batches so a long history never freezes.

import { el } from "../lib/dom.js";
import { num, plural } from "../lib/format.js";
import { finding, caption } from "../lib/section.js";

const BATCH = 100; // rows painted per "Afficher plus" step

// norm folds a string for search: NFD decompose, strip combining marks, lower.
// So "Beyoncé" matches "beyonce" and "Sigur Rós" matches "sigur ros".
function norm(s) {
  return (s || "").normalize("NFD").replace(/[\u0300-\u036f]/g, "").toLowerCase();
}

// decadeOf turns a release year into a decade label ("1970s"), or "" when the
// year is unknown. Matches Les epoques' "${dec}s" grammar for consistency.
function decadeOf(year) {
  if (!year || year <= 0) return "";
  return `${Math.floor(year / 10) * 10}s`;
}

// fmtStamp renders a timestamp as "mar. 8 juil. · 23h16" (French, no year).
function fmtStamp(iso) {
  const d = new Date(iso);
  if (isNaN(d)) return "";
  const day = d.toLocaleDateString("fr-FR", { weekday: "short", day: "numeric", month: "short" });
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  return `${day} · ${hh}h${mm}`;
}

// daysSince returns whole days between two ISO timestamps (>= 0), or null when
// either is unparseable.
function daysSince(fromISO, toISO) {
  const a = new Date(fromISO), b = new Date(toISO);
  if (isNaN(a) || isNaN(b)) return null;
  return Math.max(0, Math.floor((b - a) / 86400000));
}

// facetValues collects the distinct chip values for one accessor over the rows,
// ordered by frequency desc then label, capped so the chip bar stays legible.
function facetValues(rows, accessor, cap) {
  const count = new Map();
  for (const r of rows) {
    for (const v of accessor(r)) {
      if (!v) continue;
      count.set(v, (count.get(v) || 0) + 1);
    }
  }
  const vals = [...count.keys()].sort((a, b) => {
    const d = count.get(b) - count.get(a);
    return d !== 0 ? d : a.localeCompare(b, "fr");
  });
  return cap ? vals.slice(0, cap) : vals;
}

export function discotheque(data, mk) {
  const d = data.discotheque;
  if (!d || !Array.isArray(d.rows) || d.rows.length === 0) return null;
  const { sec, body } = mk({ tc: "04:00", title: "La discothèque", id: "discotheque" });

  const rows = d.rows;

  // Precompute the search haystack and the decade for each row once.
  const prepared = rows.map((r) => ({
    r,
    decade: decadeOf(r.year),
    hay: norm([r.artist, r.title, r.album, r.label, r.show, (r.genres || []).join(" ")].join(" ")),
  }));

  body.appendChild(finding(
    `<span class="you">${num(d.plays)}</span> ${plural(d.plays, "écoute", "écoutes")}, ` +
    `<span class="you">${num(d.distinct)}</span> ${plural(d.distinct, "morceau distinct", "morceaux distincts")}. ` +
    `Cherche dedans.`));

  // Facet definitions: id, label, and the distinct chip values (frequency-ranked
  // for the open-ended facets, chronological for decades).
  const facets = [
    { id: "genre", label: "Genre", vals: facetValues(rows, (r) => r.genres || [], 24) },
    { id: "decennie", label: "Décennie", vals: [...new Set(prepared.map((p) => p.decade).filter(Boolean))].sort() },
    { id: "station", label: "Station", vals: facetValues(rows, (r) => (r.station ? [r.station] : []), 0) },
    { id: "emission", label: "Émission", vals: facetValues(rows, (r) => (r.show ? [r.show] : []), 24) },
  ];
  // Active selections per facet id (a Set of chip values).
  const active = { genre: new Set(), decennie: new Set(), station: new Set(), emission: new Set() };
  let query = "";

  // Controls: search box + chip groups.
  const input = el("input", {
    type: "search", class: "disco-search", id: "disco-q",
    placeholder: "Artiste, titre, album, label, émission, genre...",
    "aria-label": "Rechercher dans la discothèque", autocomplete: "off", spellcheck: "false",
  });
  const controls = el("div", { class: "disco-controls" }, [
    el("label", { class: "disco-search-wrap" }, [input]),
  ]);

  for (const f of facets) {
    if (f.vals.length === 0) continue;
    const group = el("div", { class: "disco-chips", role: "group", "aria-label": f.label });
    group.appendChild(el("span", { class: "disco-facet-lbl", text: f.label }));
    for (const v of f.vals) {
      const chip = el("button", { type: "button", class: "disco-chip", "aria-pressed": "false", text: v });
      chip.addEventListener("click", () => {
        if (active[f.id].has(v)) { active[f.id].delete(v); chip.setAttribute("aria-pressed", "false"); }
        else { active[f.id].add(v); chip.setAttribute("aria-pressed", "true"); }
        render();
      });
      group.appendChild(chip);
    }
    controls.appendChild(group);
  }
  body.appendChild(controls);

  // Results header + list + "show more".
  const head = el("p", { class: "disco-head", "aria-live": "polite" });
  const list = el("div", { class: "disco-list" });
  const more = el("button", { type: "button", class: "disco-more", text: "Afficher plus" });
  const moreWrap = el("div", { class: "disco-more-wrap" }, [more]);
  body.appendChild(head);
  body.appendChild(el("div", { class: "figure plain" }, [list, moreWrap]));

  let filtered = prepared;
  let shown = 0;

  function matches(p) {
    if (query && !p.hay.includes(query)) return false;
    // Each facet ANDs: a row passes a facet with active chips only if it holds
    // one of them (OR within a facet). No active chips = that facet is open.
    if (active.genre.size && !(p.r.genres || []).some((g) => active.genre.has(g))) return false;
    if (active.decennie.size && !active.decennie.has(p.decade)) return false;
    if (active.station.size && !active.station.has(p.r.station)) return false;
    if (active.emission.size && !active.emission.has(p.r.show)) return false;
    return true;
  }

  function rowNode(r) {
    const cover = r.cover
      ? el("img", { class: "disco-cover", src: r.cover, alt: "", loading: "lazy", decoding: "async",
          onerror: (e) => { e.target.style.display = "none"; } })
      : el("span", { class: "disco-cover disco-cover-empty", "aria-hidden": "true" });

    const albumYear = [r.album, r.year ? String(r.year) : ""].filter(Boolean).join(" · ");
    const place = [r.station, r.show].filter(Boolean).join(" · ");

    const titleLine = el("div", { class: "disco-title" }, [
      el("span", { class: "t", text: r.title || "(sans titre)" }),
      r.liked ? el("span", { class: "disco-like", title: "Aimé", "aria-label": "Aimé", text: "♥" }) : null,
      r.disliked ? el("span", { class: "disco-dislike", title: "Pas aimé", "aria-label": "Pas aimé", text: "♡" }) : null,
    ]);

    const metaBits = [el("span", { class: "ar", text: r.artist || "Artiste inconnu" })];
    if (albumYear) metaBits.push(el("span", { class: "al", text: albumYear }));
    if (r.label) metaBits.push(el("span", { class: "la", text: r.label }));
    if (r.genres && r.genres.length) metaBits.push(el("span", { class: "ge", text: r.genres.slice(0, 3).join(", ") }));

    const foot = el("div", { class: "disco-foot" }, [
      place ? el("span", { class: "pl", text: place }) : null,
      el("span", { class: "ts", text: fmtStamp(r.ts) }),
      r.link ? el("a", { class: "disco-listen", href: r.link, target: "_blank", rel: "noopener noreferrer", text: "Écouter" }) : null,
    ]);

    return el("div", { class: "disco-row" }, [
      cover,
      el("div", { class: "disco-body" }, [
        titleLine,
        el("div", { class: "disco-meta" }, metaBits),
        foot,
      ]),
    ]);
  }

  // paint appends the next batch to the list without clearing it.
  function paint() {
    const end = Math.min(shown + BATCH, filtered.length);
    const frag = document.createDocumentFragment();
    for (let i = shown; i < end; i++) frag.appendChild(rowNode(filtered[i].r));
    list.appendChild(frag);
    shown = end;
    more.style.display = shown < filtered.length ? "" : "none";
  }

  function render() {
    query = norm(input.value);
    filtered = prepared.filter(matches);
    list.textContent = "";
    shown = 0;
    if (filtered.length === 0) {
      list.appendChild(el("p", { class: "empty", text: "Rien ne correspond. Enlève un filtre ou change ta recherche." }));
      more.style.display = "none";
    } else {
      paint();
    }
    const distinct = new Set(filtered.map((p) => `${p.r.artist} ${p.r.title}`)).size;
    head.textContent = `${num(filtered.length)} ${plural(filtered.length, "écoute", "écoutes")} · ${num(distinct)} ${plural(distinct, "morceau distinct", "morceaux distincts")}`;
  }

  input.addEventListener("input", render);
  more.addEventListener("click", paint);
  render();

  // Honesty captions: genre grain + enrichment staleness.
  const capBits = ["Les genres sont à l’échelle de l’artiste, pas du morceau."];
  if (d.generatedAt) {
    const days = daysSince(d.generatedAt, data.generatedAt);
    const age = days == null ? "" : `enrichi il y a ${num(days)} ${plural(days, "jour", "jours")}`;
    const gaps = `${num(d.unenriched)} ${plural(d.unenriched, "écoute sans genre", "écoutes sans genre")}`;
    capBits.push([age, gaps].filter(Boolean).join(" · ") + ".");
  } else {
    capBits.push(`${num(d.unenriched)} ${plural(d.unenriched, "écoute sans genre", "écoutes sans genre")} (aucun enrichissement).`);
  }
  body.appendChild(caption(capBits.join(" ")));

  return sec;
}
