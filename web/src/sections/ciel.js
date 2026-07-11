// 5. La carte du ciel (conditional: data.enriched.constellation). The signature
// visual: a full-bleed night sky. Each artist is a star; magnitude (radius +
// glow) encodes play count; position is the precomputed 2D affinity projection.
// The rose star is the most played. Brightest N are labelled; the rest reveal
// their label on hover or keyboard focus. Every star is tabbable with an
// aria-label, so the value never lives in the tooltip alone.

import { el, svg } from "../lib/dom.js";
import { num, plural } from "../lib/format.js";
import { finding, caption } from "../lib/section.js";
import { bindTip } from "../lib/tooltip.js";
import { max, extent } from "d3-array";
import { scaleSqrt, scaleLinear } from "d3-scale";

export function ciel(data, mk) {
  const enr = data.enriched;
  const stars = enr && Array.isArray(enr.constellation) ? enr.constellation.slice() : [];
  if (stars.length === 0) return null;
  const { sec, body } = mk({ tc: "02:30", title: "La carte du ciel", id: "ciel", cls: "sky-section" });

  stars.sort((a, b) => b.plays - a.plays);
  const topStar = stars[0];
  const brightCount = Math.min(6, stars.length);

  const W = 1000, H = 520, pad = 70;
  const xs = extent(stars, (d) => d.x), ys = extent(stars, (d) => d.y);
  const sx = scaleLinear().domain(xs[0] === xs[1] ? [xs[0] - 1, xs[0] + 1] : xs).range([pad, W - pad]);
  const sy = scaleLinear().domain(ys[0] === ys[1] ? [ys[0] - 1, ys[0] + 1] : ys).range([H - pad, pad]);
  const sr = scaleSqrt().domain([0, max(stars, (d) => d.plays) || 1]).range([1.6, 15]);

  // faint background dust
  const dust = [];
  let seed = 7;
  const rnd = () => (seed = (seed * 9301 + 49297) % 233280) / 233280;
  for (let i = 0; i < 60; i++) {
    dust.push(svg("circle", { cx: (rnd() * W).toFixed(1), cy: (rnd() * H).toFixed(1), r: (rnd() * 0.9 + 0.3).toFixed(2), fill: "var(--star)", opacity: (rnd() * 0.25 + 0.05).toFixed(2) }));
  }

  const nodes = stars.map((d, i) => {
    const x = sx(d.x), y = sy(d.y), r = sr(d.plays);
    const bright = i < brightCount;
    const isTop = d === topStar;
    const g = svg("g", {
      class: `star${isTop ? " top" : ""}`, tabindex: "0", role: "img",
      "aria-label": `${d.name}, ${d.plays} ${plural(d.plays, "écoute", "écoutes")}${d.genres && d.genres.length ? ", " + d.genres.join(", ") : ""}`,
      transform: `translate(${x.toFixed(1)} ${y.toFixed(1)})`,
      style: { "--tw": (2.4 + rnd() * 2.6).toFixed(2) + "s", "--twd": (rnd() * 3).toFixed(2) + "s" },
    }, [
      svg("circle", { class: "star-focus", r: (r + 7).toFixed(1) }),
      svg("circle", { class: "star-glow", r: (r * 2.6).toFixed(1) }),
      svg("circle", { class: "star-core", r: r.toFixed(1) }),
      svg("text", { class: `star-label ${bright ? "" : "hidden-label"}`.trim(), x: 0, y: -(r + 8), "text-anchor": "middle" }, [d.name]),
      bright ? svg("text", { class: "star-sub", x: 0, y: -(r + 8) + 13, "text-anchor": "middle" }, [`${d.plays}`]) : null,
    ]);
    bindTip(g, () => `<b>${d.name}</b><br>${d.plays} ${plural(d.plays, "écoute", "écoutes")}${d.genres && d.genres.length ? "<br>" + d.genres.join(" · ") : ""}`);
    return g;
  });

  const map = svg("svg", { viewBox: `0 0 ${W} ${H}`, role: "group", "aria-label": "Constellation des artistes écoutés" }, [...dust, ...nodes]);

  body.appendChild(finding(`Ton étoile polaire : <span class="you">${topStar.name}</span>.`));

  const sky = el("div", { class: "sky" }, [el("div", { class: "page" }, [map])]);
  sec.appendChild(sky);

  const nArtists = enr.nArtists || stars.length;
  sec.appendChild(el("div", { class: "page" }, [
    caption(
      `${stars.length} ${plural(stars.length, "artiste placé", "artistes placés")} selon leur affinité (projection 2D). Taille de l’étoile : nombre d’écoutes. ${enr.matchRate != null ? Math.round(enr.matchRate * 100) + " % des titres ont pu être enrichis." : ""}`,
      nArtists, { threshold: 25, unit: "artistes" }
    ),
  ]));
  return sec;
}
