// 2. La grille de tes nuits: when you listen. The data are two marginals
// (seconds per hour-of-day, seconds per weekday), not a full hour x weekday
// matrix, so we show two honest figures: a 24h radial dial (the tuner clock)
// and a weekday bar column. Night hours carry the rose; day hours the amber.

import { el, svg } from "../lib/dom.js";
import { fmtDur, num, plural } from "../lib/format.js";
import { finding, caption } from "../lib/section.js";
import { bindTip } from "../lib/tooltip.js";
import { max } from "d3-array";

const DAYS = ["Dim", "Lun", "Mar", "Mer", "Jeu", "Ven", "Sam"];
const DAYS_LONG = ["dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"];
const isNight = (h) => h >= 22 || h < 6;

function hoursDial(hourly) {
  const size = 340, c = size / 2;
  const rIn = 44, rOut = 150;
  const mx = max(hourly) || 1;
  const g = [];

  // hour graduations + cardinal labels
  for (let h = 0; h < 24; h++) {
    const a = (h / 24) * 2 * Math.PI - Math.PI / 2;
    const big = h % 6 === 0;
    g.push(svg("line", {
      x1: c + (rOut + 4) * Math.cos(a), y1: c + (rOut + 4) * Math.sin(a),
      x2: c + (rOut + (big ? 14 : 9)) * Math.cos(a), y2: c + (rOut + (big ? 14 : 9)) * Math.sin(a),
      class: "grad", "stroke-width": big ? 1.4 : 0.7,
    }));
    if (big) {
      g.push(svg("text", {
        x: c + (rOut + 28) * Math.cos(a), y: c + (rOut + 28) * Math.sin(a),
        class: "tick-label", "text-anchor": "middle", "dominant-baseline": "middle",
      }, [`${h}h`]));
    }
  }

  // radial bars, one per hour
  let peak = 0;
  hourly.forEach((v, h) => { if (v > hourly[peak]) peak = h; });
  hourly.forEach((v, h) => {
    const a0 = ((h - 0.5) / 24) * 2 * Math.PI - Math.PI / 2;
    const a1 = ((h + 0.5) / 24) * 2 * Math.PI - Math.PI / 2;
    const rr = rIn + (rOut - rIn) * (v / mx);
    const [x0, y0] = [c + rIn * Math.cos(a0), c + rIn * Math.sin(a0)];
    const [x1, y1] = [c + rr * Math.cos(a0), c + rr * Math.sin(a0)];
    const [x2, y2] = [c + rr * Math.cos(a1), c + rr * Math.sin(a1)];
    const [x3, y3] = [c + rIn * Math.cos(a1), c + rIn * Math.sin(a1)];
    const wedge = svg("path", {
      d: `M${x0} ${y0} L${x1} ${y1} A${rr} ${rr} 0 0 1 ${x2} ${y2} L${x3} ${y3} A${rIn} ${rIn} 0 0 0 ${x0} ${y0} Z`,
      fill: h === peak ? "var(--fip)" : isNight(h) ? "var(--fip-ink)" : "var(--amber)",
      "fill-opacity": v === 0 ? 0.12 : h === peak ? 1 : 0.82,
      tabindex: v > 0 ? "0" : null,
      role: v > 0 ? "img" : null,
      "aria-label": v > 0 ? `${h} heures: ${fmtDur(v)}` : null,
    });
    if (v > 0) bindTip(wedge, () => `<b>${String(h).padStart(2, "0")}h</b> ${fmtDur(v)}`);
    g.push(wedge);
  });

  g.push(svg("circle", { cx: c, cy: c, r: rIn - 4, fill: "none", class: "grad", "stroke-width": 1 }));
  return { svg: svg("svg", { viewBox: `0 0 ${size} ${size}`, role: "img", "aria-label": "Écoute par heure de la journée" }, g), peak };
}

function weekdayBars(weekday) {
  const order = [1, 2, 3, 4, 5, 6, 0]; // French week: Monday first
  const mx = max(weekday) || 1;
  let peak = 0;
  weekday.forEach((v, i) => { if (v > weekday[peak]) peak = i; });
  const rows = order.map((i) => {
    const v = weekday[i];
    return el("div", { class: "row" }, [
      el("span", { class: "lbl", text: DAYS[i] }),
      el("div", { class: "track" }, [
        el("div", { class: `fill ${i === peak ? "peak" : ""}`.trim(), style: { width: `${Math.max(2, (v / mx) * 100)}%` } }),
      ]),
      el("span", { class: "v", text: fmtDur(v) }),
    ]);
  });
  return { node: el("div", { class: "weekday" }, rows), peak };
}

export function grille(data, mk) {
  const hourly = data.hourly || [];
  const weekday = data.weekday || [];
  const total = hourly.reduce((a, b) => a + b, 0);
  const { sec, body } = mk({ tc: "00:24", title: "La grille de tes nuits", id: "grille" });
  if (total === 0) {
    body.appendChild(el("p", { class: "empty", text: "Pas encore assez d’écoute pour dessiner tes horaires." }));
    return sec;
  }

  const dial = hoursDial(hourly);
  const wd = weekdayBars(weekday);
  const nightSec = hourly.reduce((a, v, h) => a + (isNight(h) ? v : 0), 0);
  const nightShare = total > 0 ? nightSec / total : 0;

  const ph = String(dial.peak).padStart(2, "0");
  const f = nightShare >= 0.5
    ? `Tu écoutes surtout <span class="you">la nuit</span>, avec un pic vers ${ph}h.`
    : `Ton heure de pointe tombe vers <span class="you">${ph}h</span>.`;
  body.appendChild(finding(f));

  body.appendChild(el("div", { class: "figure grille" }, [
    el("div", { class: "dial-hours" }, [dial.svg]),
    el("div", {}, [
      el("p", { class: "muted", style: { margin: "0 0 12px", fontSize: "0.85rem", fontStyle: "italic" }, text: "Par jour de la semaine" }),
      wd.node,
    ]),
  ]));

  const cap = caption(
    `${Math.round(nightShare * 100)} % de ton temps d’écoute se joue entre 22h et 6h. Jour le plus actif : ${DAYS_LONG[wd.peak]}. Réparti sur ${num(total / 3600)} h cumulées.`,
    data.calibration && data.calibration.sessions
  );
  body.appendChild(cap);
  return sec;
}
