// 4. Les epoques (conditional: data.epochs). Release-year timeline of what you
// listened to. Column per year with a 0 baseline; the dominant decade carries
// the rose. Absent data collapses the whole section.

import { el, svg } from "../lib/dom.js";
import { fmtDur, num, plural } from "../lib/format.js";
import { finding, caption } from "../lib/section.js";
import { bindTip } from "../lib/tooltip.js";
import { max } from "d3-array";

export function epochs(data, mk) {
  const e = data.epochs;
  if (!e || !Array.isArray(e.byYear) || e.byYear.length === 0) return null;
  const { sec, body } = mk({ tc: "01:45", title: "Les époques", id: "epochs" });

  const years = e.byYear.slice().sort((a, b) => a.year - b.year);
  const y0 = years[0].year, y1 = years[years.length - 1].year;
  const mxY = max(years, (d) => d.seconds) || 1;

  // dominant decade
  const decades = (e.byDecade || []).slice().sort((a, b) => b.seconds - a.seconds);
  const topDecade = decades[0];

  const W = 1000, H = 300, padB = 34, padL = 8, padT = 12;
  const span = Math.max(1, y1 - y0);
  const bw = (W - padL * 2) / (span + 1);
  const bars = [];
  years.forEach((d) => {
    const x = padL + ((d.year - y0) / (span + 1)) * (W - padL * 2);
    const h = (d.seconds / mxY) * (H - padB - padT);
    const inTop = topDecade && Math.floor(d.year / 10) * 10 === topDecade.decade;
    const r = svg("rect", {
      x: x.toFixed(1), y: (H - padB - h).toFixed(1), width: Math.max(2, bw - 2).toFixed(1), height: Math.max(1, h).toFixed(1),
      class: `bar${inTop ? " peak" : ""}`, tabindex: "0", role: "img", "aria-label": `${d.year}: ${fmtDur(d.seconds)}`,
    });
    bindTip(r, () => `<b>${d.year}</b> ${fmtDur(d.seconds)}`);
    bars.push(r);
  });
  // decade axis labels
  const labels = [];
  for (let dec = Math.floor(y0 / 10) * 10; dec <= y1; dec += 10) {
    const x = padL + ((dec - y0) / (span + 1)) * (W - padL * 2);
    if (x < 0 || x > W) continue;
    labels.push(svg("text", { x: x.toFixed(1), y: H - 12, class: "decade-lbl", "text-anchor": "start" }, [`${dec}s`]));
  }
  const axis = svg("line", { x1: padL, y1: H - padB, x2: W - padL, y2: H - padB, class: "grad", "stroke-width": 1 });

  body.appendChild(finding(topDecade
    ? `Ton oreille penche vers les <span class="you">années ${topDecade.decade}</span>.`
    : `Tu écoutes à travers les décennies.`));
  body.appendChild(el("div", { class: "figure epochs" }, [svg("svg", { viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": "Écoute par année de sortie" }, [axis, ...bars, ...labels])]));
  body.appendChild(caption(
    `Réparti sur ${e.n} ${plural(e.n, "titre daté", "titres datés")}, de ${y0} à ${y1}.`,
    e.n, { threshold: 30, unit: "titres" }
  ));
  return sec;
}
