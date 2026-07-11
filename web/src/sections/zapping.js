// 3. Le zapping: the station transition graph (Markov edges), plus the folded
// "Vos radios" station shares and the session shape stated inline in prose.
// Nodes ring the circle sized by listening share; brand colours pass through
// legible(); edges curve so A->B and B->A never overlap; labels are direct.

import { el, svg } from "../lib/dom.js";
import { fmtDur, num, pct, plural } from "../lib/format.js";
import { finding, caption } from "../lib/section.js";
import { legible } from "../lib/color.js";
import { bindTip } from "../lib/tooltip.js";
import { max } from "d3-array";

function graph(trans, stations) {
  const shareByKey = {}, colorByKey = {}, dispByKey = {};
  (stations || []).forEach((s) => { shareByKey[s.key] = s.share; colorByKey[s.key] = s.color; dispByKey[s.key] = s.display; });
  const keys = [];
  const seen = {};
  trans.forEach((t) => [t.from, t.to].forEach((k) => { if (!seen[k]) { seen[k] = 1; keys.push(k); } }));
  trans.forEach((t) => { dispByKey[t.from] = t.fromDisplay; dispByKey[t.to] = t.toDisplay; });

  const W = 760, H = Math.max(420, 150 + keys.length * 26), c = { x: W / 2, y: H / 2 };
  const R = Math.min(W, H) / 2 - 92;
  const pos = {};
  keys.forEach((k, i) => {
    const a = (i / keys.length) * 2 * Math.PI - Math.PI / 2;
    pos[k] = { x: c.x + R * Math.cos(a), y: c.y + R * Math.sin(a), a };
  });
  const mxCount = max(trans, (t) => t.count) || 1;

  const kids = [];
  const top = trans[0];

  // edges
  trans.forEach((t) => {
    const a = pos[t.from], b = pos[t.to];
    if (!a || !b) return;
    const mx = (a.x + b.x) / 2, my = (a.y + b.y) / 2;
    const dx = b.x - a.x, dy = b.y - a.y, len = Math.hypot(dx, dy) || 1;
    const off = 30;
    const qx = mx + (-dy / len) * off, qy = my + (dx / len) * off;
    const nr = 22;
    const d1 = Math.hypot(qx - a.x, qy - a.y) || 1;
    const d2 = Math.hypot(qx - b.x, qy - b.y) || 1;
    const sx = a.x + ((qx - a.x) / d1) * nr, sy = a.y + ((qy - a.y) / d1) * nr;
    const ex = b.x + ((qx - b.x) / d2) * nr, ey = b.y + ((qy - b.y) / d2) * nr;
    const isTop = t === top;
    const op = (0.28 + t.prob * 0.62).toFixed(2);
    const p = svg("path", {
      d: `M${sx.toFixed(1)} ${sy.toFixed(1)} Q${qx.toFixed(1)} ${qy.toFixed(1)} ${ex.toFixed(1)} ${ey.toFixed(1)}`,
      class: `edge${isTop ? " hot" : ""}`,
      "stroke-width": (1.4 + (t.count / mxCount) * 7).toFixed(1),
      opacity: op,
      tabindex: "0",
      role: "img",
      "aria-label": `${t.fromDisplay} vers ${t.toDisplay}, ${t.count} fois`,
    });
    bindTip(p, () => `<b>${t.fromDisplay} → ${t.toDisplay}</b><br>${t.count} ${plural(t.count, "fois", "fois")} (${Math.round(t.prob * 100)} %)`);
    // Fixed-size arrowhead, oriented along the quadratic's end tangent
    // (direction end - control), so it never scales with stroke width.
    const tdx = ex - qx, tdy = ey - qy, tl = Math.hypot(tdx, tdy) || 1;
    const ux = tdx / tl, uy = tdy / tl, ah = 8, aw = 4.5;
    const bx = ex - ux * ah, by = ey - uy * ah; // base centre
    const px = -uy, py = ux; // perpendicular
    const head = svg("polygon", {
      points: `${ex.toFixed(1)},${ey.toFixed(1)} ${(bx + px * aw).toFixed(1)},${(by + py * aw).toFixed(1)} ${(bx - px * aw).toFixed(1)},${(by - py * aw).toFixed(1)}`,
      fill: "var(--fip)", opacity: op, "pointer-events": "none",
    });
    kids.push(p, head);
  });

  // nodes
  keys.forEach((k) => {
    const pt = pos[k];
    const r = 15 + (shareByKey[k] || 0) * 26;
    const dot = svg("circle", { cx: pt.x, cy: pt.y, r: r.toFixed(1), fill: "var(--surface-2)", stroke: colorByKey[k] ? legible(colorByKey[k]) : "var(--fip)", class: "node-dot" });
    const ly = pt.y < c.y ? pt.y - r - 9 : pt.y + r + 17;
    const label = svg("text", { x: pt.x, y: ly, class: "node-label", "text-anchor": "middle", fill: "var(--ink)" }, [dispByKey[k] || k]);
    kids.push(dot, label);
  });

  return svg("svg", { viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": "Graphe des transitions entre radios" }, kids);
}

function stationBars(stations) {
  const mx = max(stations, (s) => s.listeningSec) || 1;
  return el("div", { class: "stations" }, stations.map((s, i) => {
    const col = s.color ? legible(s.color) : "var(--fip)";
    return el("div", { class: "st-row" }, [
      el("span", { class: "lbl", title: s.display, text: s.display }),
      el("div", { class: "track" }, [el("div", { class: "fill", style: { width: `${Math.max(2, (s.listeningSec / mx) * 100)}%`, background: i === 0 ? "var(--fip)" : col } })]),
      el("span", { class: "v", html: `${fmtDur(s.listeningSec)} <small>${pct(s.share)}</small>` }),
    ]);
  }));
}

export function zapping(data, mk) {
  const trans = data.transitions || [];
  const stations = data.stations || [];
  const s = data.sessions || {};
  const { sec, body } = mk({ tc: "01:00", title: "Le zapping", id: "zapping" });

  if (trans.length > 0) {
    const top = trans[0];
    body.appendChild(finding(`Ton saut réflexe : <span class="you">${top.fromDisplay} → ${top.toDisplay}</span>.`));
    body.appendChild(el("div", { class: "figure zap" }, [graph(trans, stations)]));
    body.appendChild(caption(
      `Chaque flèche est une transition observée; son épaisseur suit sa fréquence. La plus fréquente : ${top.fromDisplay} vers ${top.toDisplay} (${top.count} ${plural(top.count, "fois", "fois")}). ${data.totals.zaps} ${plural(data.totals.zaps, "changement de radio", "changements de radio")} en tout.`,
      data.totals.zaps, { threshold: 8 }
    ));
  } else {
    body.appendChild(el("p", { class: "empty", text: "Change de radio quelques fois pour voir apparaître ta carte de zapping." }));
  }

  // Vos radios (folded in)
  if (stations.length > 0) {
    body.appendChild(finding(`Sur ${stations.length} ${plural(stations.length, "radio écoutée", "radios écoutées")}, <span class="you">${stations[0].display}</span> mène.`));
    body.appendChild(el("div", { class: "figure" }, [stationBars(stations)]));
    body.appendChild(caption(`Temps d’écoute par webradio, part du total à droite.`, data.calibration && data.calibration.sessions));
  }

  // Sessions, stated inline as prose (not a tile wall)
  if (s.count > 0) {
    const p = el("p", { class: "prose", style: { marginTop: "32px" } });
    p.innerHTML =
      `Tes sessions durent en médiane <span class="stat you">${fmtDur(s.medianSec)}</span>, ` +
      `en moyenne <span class="stat">${fmtDur(s.meanSec)}</span>, ` +
      `et la plus longue a tenu <span class="stat">${fmtDur(s.maxSec)}</span>. ` +
      `<span class="muted">(${s.count} ${plural(s.count, "session", "sessions")})</span>`;
    body.appendChild(p);
  }
  return sec;
}
