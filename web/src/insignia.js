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
