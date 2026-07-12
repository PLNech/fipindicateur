// Drawn SVG insignia, keyed by achievement ID. No emoji anywhere (the retired
// offender). Every medal shares a dial frame with graduation ticks; the centre
// glyph is a dial / needle / wave motif chosen per achievement. Colour comes
// from currentColor (rose when unlocked, hairline when locked) set in CSS, so
// the same drawing reads correctly in both states and both themes.

import { svg } from "./lib/dom.js";

const C = 32; // centre
const R = 29; // frame radius

function frame() {
  const parts = [
    svg("circle", { cx: C, cy: C, r: R, fill: "none", stroke: "currentColor", "stroke-width": 1.5, opacity: 0.55 }),
  ];
  // dial graduations
  for (let i = 0; i < 24; i++) {
    const a = (i / 24) * 2 * Math.PI - Math.PI / 2;
    const long = i % 6 === 0;
    const r0 = R - (long ? 4.5 : 2.5);
    parts.push(
      svg("line", {
        x1: C + R * Math.cos(a), y1: C + R * Math.sin(a),
        x2: C + r0 * Math.cos(a), y2: C + r0 * Math.sin(a),
        stroke: "currentColor", "stroke-width": long ? 1.4 : 0.8, opacity: 0.45,
      })
    );
  }
  return parts;
}

const S = { fill: "none", stroke: "currentColor", "stroke-width": 2.2, "stroke-linecap": "round", "stroke-linejoin": "round" };

// wave path: n crests across width w centred on cy.
function wave(cx, cy, w, amp, n) {
  const pts = [];
  const steps = 40;
  for (let i = 0; i <= steps; i++) {
    const t = i / steps;
    const x = cx - w / 2 + t * w;
    const y = cy - Math.sin(t * n * Math.PI * 2) * amp;
    pts.push(`${x.toFixed(1)},${y.toFixed(1)}`);
  }
  return svg("polyline", { ...S, points: pts.join(" ") });
}

function glyph(id) {
  switch (id) {
    case "night_owl": {
      // crescent moon + spark
      return [
        svg("path", { ...S, d: `M38 20 a13 13 0 1 0 0 24 a10 10 0 0 1 0 -24 z` }),
        svg("path", { ...S, "stroke-width": 1.6, d: "M23 22 l0 6 M20 25 l6 0" }),
      ];
    }
    case "early_bird": {
      // sunrise over a horizon
      const rays = [];
      for (let i = 0; i < 5; i++) {
        const a = -Math.PI + (i / 4) * Math.PI;
        rays.push(svg("line", { ...S, "stroke-width": 1.6, x1: C + 12 * Math.cos(a), y1: 36 + 12 * Math.sin(a), x2: C + 16 * Math.cos(a), y2: 36 + 16 * Math.sin(a) }));
      }
      return [
        svg("path", { ...S, d: "M22 36 a10 10 0 0 1 20 0" }),
        svg("line", { ...S, x1: 16, y1: 40, x2: 48, y2: 40 }),
        ...rays,
      ];
    }
    case "globe":
      return [
        svg("circle", { ...S, cx: C, cy: C, r: 13 }),
        svg("ellipse", { ...S, "stroke-width": 1.6, cx: C, cy: C, rx: 5.5, ry: 13 }),
        svg("line", { ...S, "stroke-width": 1.6, x1: 19, y1: C, x2: 45, y2: C }),
        svg("path", { ...S, "stroke-width": 1.4, d: "M21 25 h22 M21 39 h22" }),
      ];
    case "explorer": {
      // compass rose (needle motif)
      return [
        svg("polygon", { fill: "currentColor", stroke: "none", points: "32,17 36,32 32,30 28,32" }),
        svg("polygon", { ...S, "stroke-width": 1.8, points: "32,47 28,32 32,34 36,32" }),
        svg("circle", { fill: "currentColor", stroke: "none", cx: C, cy: C, r: 2 }),
      ];
    }
    case "zapper":
      return [svg("path", { ...S, d: "M35 16 L23 34 h8 l-3 14 L41 28 h-8 z" })];
    case "marathon": {
      // dial with a long sweeping hand (2h) + needle
      return [
        svg("circle", { ...S, "stroke-width": 1.6, cx: C, cy: C, r: 12 }),
        svg("line", { ...S, x1: C, y1: C, x2: C, y2: 22 }),
        svg("line", { ...S, "stroke-width": 1.8, x1: C, y1: C, x2: 41, y2: 37 }),
        svg("circle", { fill: "currentColor", stroke: "none", cx: C, cy: C, r: 2 }),
      ];
    }
    case "faithful": {
      // ring of 7 dots (7 consecutive days)
      const dots = [];
      for (let i = 0; i < 7; i++) {
        const a = (i / 7) * 2 * Math.PI - Math.PI / 2;
        dots.push(svg("circle", { fill: "currentColor", stroke: "none", cx: C + 12 * Math.cos(a), cy: C + 12 * Math.sin(a), r: i === 6 ? 3 : 2.2 }));
      }
      return dots;
    }
    case "purist":
      // bullseye (90% on one)
      return [
        svg("circle", { ...S, "stroke-width": 1.6, cx: C, cy: C, r: 13 }),
        svg("circle", { ...S, "stroke-width": 1.6, cx: C, cy: C, r: 7.5 }),
        svg("circle", { fill: "currentColor", stroke: "none", cx: C, cy: C, r: 3 }),
      ];
    case "melomane":
      // stacked waves (cumulative listening)
      return [wave(C, 26, 26, 3.2, 1.5), wave(C, 33, 26, 4.4, 1.5), wave(C, 40, 26, 3.2, 1.5)];
    // ---- Emission (programme) insignia. Families share a motif; paliers and
    // near-siblings reuse it so the wall reads as a coherent set. ----
    case "shows_premiere":
    case "shows_curieux":
    case "shows_habitue":
    case "shows_connaisseur": {
      // Broadcast tower: a mast with radiating arcs (the antenne).
      const arcs = [];
      for (let i = 1; i <= 2; i++) {
        const r = 6 + i * 5;
        arcs.push(svg("path", { ...S, "stroke-width": 1.6, d: `M${C - r} ${28} a${r} ${r} 0 0 1 ${2 * r} 0` }));
      }
      return [
        ...arcs,
        svg("line", { ...S, x1: C, y1: 28, x2: C, y2: 46 }),
        svg("circle", { fill: "currentColor", stroke: "none", cx: C, cy: 28, r: 2.4 }),
      ];
    }
    case "shows_traversee_bronze":
    case "shows_traversee_or": {
      // Two arrows crossing a boundary line (moving from show to show).
      return [
        svg("line", { ...S, "stroke-width": 1.4, x1: C, y1: 18, x2: C, y2: 46 }),
        svg("path", { ...S, d: "M20 27 h10 M26 23 l4 4 l-4 4" }),
        svg("path", { ...S, d: "M44 37 h-10 M38 33 l-4 4 l4 4" }),
      ];
    }
    case "shows_temps_bronze":
    case "shows_temps_argent":
    case "shows_temps_or": {
      // Microphone: capsule, stem and base (the studio voice).
      return [
        svg("rect", { ...S, "stroke-width": 1.8, x: 27, y: 16, width: 10, height: 17, rx: 5 }),
        svg("path", { ...S, "stroke-width": 1.6, d: "M23 30 a9 9 0 0 0 18 0" }),
        svg("line", { ...S, x1: C, y1: 39, x2: C, y2: 46 }),
        svg("line", { ...S, x1: 26, y1: 46, x2: 38, y2: 46 }),
      ];
    }
    case "shows_titres_bronze":
    case "shows_titres_argent":
    case "shows_titres_or": {
      // Musical note (titres heard within the show).
      return [
        svg("path", { ...S, "stroke-width": 1.8, d: "M28 40 V22 l12 -3 V37" }),
        svg("circle", { fill: "currentColor", stroke: "none", cx: 25, cy: 41, r: 3.2 }),
        svg("circle", { fill: "currentColor", stroke: "none", cx: 37, cy: 38, r: 3.2 }),
      ];
    }
    case "shows_fidele_bronze":
    case "shows_fidele_argent":
    case "shows_fidele_or":
    case "shows_assidu":
    case "shows_rituel": {
      // A ring of evenings returned to, with a heart-beat pulse across it.
      const dots = [];
      for (let i = 0; i < 8; i++) {
        const a = (i / 8) * 2 * Math.PI - Math.PI / 2;
        dots.push(svg("circle", { fill: "currentColor", stroke: "none", cx: C + 13 * Math.cos(a), cy: C + 13 * Math.sin(a), r: 2 }));
      }
      return [...dots, svg("path", { ...S, "stroke-width": 1.6, d: "M24 32 h5 l2 -5 l3 10 l2 -5 h4" })];
    }
    case "shows_part_quart":
    case "shows_part_moitie":
    case "shows_part_gros": {
      // A dial with a filled sector (share of the night in shows).
      return [
        svg("circle", { ...S, "stroke-width": 1.6, cx: C, cy: C, r: 12 }),
        svg("path", { fill: "currentColor", stroke: "none", d: `M${C} ${C} L${C} ${C - 12} A12 12 0 0 1 ${C + 12} ${C} Z` }),
      ];
    }
    case "shows_marathon":
    case "shows_veillee":
    case "shows_double_programme": {
      // Stacked programme blocks (a full evening of shows).
      return [
        svg("rect", { ...S, "stroke-width": 1.6, x: 20, y: 22, width: 24, height: 6, rx: 2 }),
        svg("rect", { ...S, "stroke-width": 1.6, x: 20, y: 30, width: 24, height: 6, rx: 2 }),
        svg("rect", { fill: "currentColor", stroke: "none", x: 20, y: 38, width: 15, height: 6, rx: 2 }),
      ];
    }
    case "shows_dimanche":
    case "shows_calendrier": {
      // Calendar: a sheet with a header bar and a marked day.
      return [
        svg("rect", { ...S, "stroke-width": 1.6, x: 19, y: 20, width: 26, height: 24, rx: 3 }),
        svg("line", { ...S, "stroke-width": 1.6, x1: 19, y1: 27, x2: 45, y2: 27 }),
        svg("line", { ...S, x1: 26, y1: 17, x2: 26, y2: 23 }),
        svg("line", { ...S, x1: 38, y1: 17, x2: 38, y2: 23 }),
        svg("circle", { fill: "currentColor", stroke: "none", cx: 32, cy: 36, r: 3 }),
      ];
    }
    case "shows_nocturne": {
      // Crescent moon with a small note (a programme in the deep night).
      return [
        svg("path", { ...S, d: "M40 20 a13 13 0 1 0 0 24 a10 10 0 0 1 0 -24 z" }),
        svg("circle", { fill: "currentColor", stroke: "none", cx: 24, cy: 40, r: 2.4 }),
        svg("path", { ...S, "stroke-width": 1.4, d: "M26 40 V30" }),
      ];
    }
    default:
      return [svg("circle", { fill: "currentColor", stroke: "none", cx: C, cy: C, r: 3 })];
  }
}

export function insigne(id) {
  return svg(
    "svg",
    { viewBox: "0 0 64 64", role: "img", "aria-hidden": "true" },
    [...frame(), ...glyph(id)]
  );
}
