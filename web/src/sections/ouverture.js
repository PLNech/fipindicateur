// 1. Ouverture d'antenne: the hero. Totals, observed range, the calibration
// statement, and the one opening choreography (a tuner dial that draws its arc
// and sweeps its needle once, plus the headline duration counting up).

import { el, svg } from "../lib/dom.js";
import { fmtDur, fmtDate, num, plural } from "../lib/format.js";
import { countUp, reduced } from "../lib/motion.js";

// A 270-degree tuner gauge. The rose arc and needle settle at `frac` (0..1),
// the favourite station's share of listening time.
function dial(frac) {
  const size = 300, c = size / 2, r = 118;
  const a0 = Math.PI * 0.75, a1 = Math.PI * 2.25; // 270deg sweep, gap at bottom
  const ang = (t) => a0 + (a1 - a0) * t;
  const pt = (t, rad) => [c + rad * Math.cos(ang(t)), c + rad * Math.sin(ang(t))];
  const arcPath = (t0, t1, rad) => {
    const [x0, y0] = pt(t0, rad), [x1, y1] = pt(t1, rad);
    const large = ang(t1) - ang(t0) > Math.PI ? 1 : 0;
    return `M${x0.toFixed(1)} ${y0.toFixed(1)} A${rad} ${rad} 0 ${large} 1 ${x1.toFixed(1)} ${y1.toFixed(1)}`;
  };

  const ticks = [];
  for (let i = 0; i <= 20; i++) {
    const t = i / 20, big = i % 5 === 0;
    const [xa, ya] = pt(t, r + 8), [xb, yb] = pt(t, r + (big ? 18 : 13));
    ticks.push(svg("line", { x1: xa, y1: ya, x2: xb, y2: yb, class: "grad", "stroke-width": big ? 1.5 : 0.8 }));
  }

  const track = svg("path", { d: arcPath(0, 1, r), fill: "none", stroke: "var(--hairline)", "stroke-width": 6, "stroke-linecap": "round" });
  const fill = svg("path", { d: arcPath(0, Math.max(0.001, frac), r), fill: "none", stroke: "var(--fip)", "stroke-width": 6, "stroke-linecap": "round" });

  const [nx, ny] = pt(frac, r - 14);
  const needle = svg("line", { x1: c, y1: c, x2: nx, y2: ny, stroke: "var(--ink)", "stroke-width": 3, "stroke-linecap": "round", class: "needle" });
  const hub = svg("circle", { cx: c, cy: c, r: 6, fill: "var(--ink)" });

  const s = svg("svg", { viewBox: `0 0 ${size} ${size}`, role: "img", "aria-label": `Cadran: ${Math.round(frac * 100)} pour cent` }, [
    ...ticks, track, fill, needle, hub,
  ]);

  // Choreography: draw the fill arc and sweep the needle once, from the dial's
  // start to its settled reading. getTotalLength needs the node attached, so we
  // set the from-state now and flip to the to-state after two frames.
  if (!reduced()) {
    const startDeg = -((a1 - a0) * frac) * (180 / Math.PI);
    needle.style.transformOrigin = `${c}px ${c}px`;
    needle.style.transform = `rotate(${startDeg.toFixed(1)}deg)`;
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        const total = fill.getTotalLength();
        fill.style.strokeDasharray = String(total);
        fill.style.strokeDashoffset = String(total);
        // force reflow so the transition runs from the dashed state
        void fill.getBoundingClientRect();
        fill.style.transition = "stroke-dashoffset .6s cubic-bezier(.22,1,.36,1)";
        fill.style.strokeDashoffset = "0";
        needle.style.transition = "transform .6s cubic-bezier(.22,1,.36,1)";
        needle.style.transform = "rotate(0deg)";
      });
    });
  }
  return s;
}

export function ouverture(data) {
  const { totals: t, calibration: c, range, stations } = data;
  const hasData = c && c.events > 0 && t.listeningSec > 0;
  const fav = stations && stations[0];

  const head = el("header", { class: "hero page reveal in" }, [
    el("div", { class: "hero-top" }, [
      el("div", { class: "wordmark", html: 'Fin d’émission <span class="fip">· FIP</span>' }),
      el("button", { class: "theme-btn", id: "themeBtn", type: "button", "aria-label": "Changer de thème" }, ["Édition papier"]),
    ]),
  ]);

  if (!hasData) {
    head.appendChild(
      el("div", { class: "hero-grid" }, [
        el("div", {}, [
          el("h1", { class: "display", html: "Le studio est encore <span class=\"rose\">silencieux</span>." }),
          el("p", { class: "hero-range muted", text: "Active les statistiques d’écoute (local) dans le menu, puis reviens après quelques écoutes." }),
        ]),
      ])
    );
    return head;
  }

  const durNode = el("span", {});
  const grid = el("div", { class: "hero-grid" }, [
    el("div", {}, [
      el("p", { class: "wordmark", style: { marginBottom: "12px" }, text: "Ouverture d’antenne" }),
      el("h1", { class: "display" }, [durNode]),
      el("p", { class: "hero-range", html: `Du ${fmtDate(range.first)} au ${fmtDate(range.last)}.` }),
      el("ul", { class: "statline" }, [
        stat(t.sessions, "sessions"),
        stat(t.daysActive, plural(t.daysActive, "jour actif", "jours actifs")),
        stat(t.zaps, plural(t.zaps, "changement", "changements"), false),
      ]),
    ]),
    el("div", { class: "dial-wrap" }, [dial(fav ? fav.share : 0)]),
  ]);
  head.appendChild(grid);

  const cal = el("p", { class: "prose muted", style: { marginTop: "32px", fontSize: "0.95rem" } });
  let sentence = `${c.events} événements observés, ${c.sessions} ${plural(c.sessions, "session reconstruite", "sessions reconstruites")} sur ${c.daysActive} ${plural(c.daysActive, "jour", "jours")}.`;
  if (c.sessions > 0 && c.sessions < 12) sentence += " Échantillon réduit : ces chiffres sont indicatifs, pas encore significatifs.";
  else sentence += " De quoi lire quelques tendances honnêtes.";
  if (fav) sentence += ` Ta radio de chevet : ${fav.display} (${Math.round(fav.share * 100)} % du temps).`;
  cal.textContent = sentence;
  head.appendChild(el("div", { class: "page" }, [cal]));

  countUp(durNode, t.listeningSec, (v) => fmtDur(v), 900);
  return head;
}

function stat(v, label, you = true) {
  return el("li", {}, [
    el("span", { class: `v ${you ? "you" : ""}`.trim(), text: num(v) }),
    el("span", { class: "k", text: label }),
  ]);
}
