// Station brand colours are not a tuned chart palette: some melt into the
// surface. legible() keeps the hue but mixes it toward the theme extreme just
// until the fill clears 3:1 non-text contrast against the figure surface.
// Computed against the live resolved colour, never eyeballed. Identity always
// stays on the text label; colour is only a secondary cue.

function cssVar(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

// Resolve any CSS colour (incl. oklch(...)) to [r,g,b] via a scratch canvas.
let ctx;
function resolveRGB(color) {
  if (!ctx) ctx = document.createElement("canvas").getContext("2d");
  ctx.fillStyle = "#000";
  ctx.fillStyle = color;
  const s = ctx.fillStyle; // normalised: #rrggbb or rgba(...)
  if (s.startsWith("#")) {
    const n = parseInt(s.slice(1), 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
  }
  const m = s.match(/\d+(\.\d+)?/g);
  return m ? m.slice(0, 3).map(Number) : [128, 128, 128];
}

function relLum([r, g, b]) {
  const f = (v) => {
    v /= 255;
    return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b);
}

function contrast(a, b) {
  const la = relLum(a), lb = relLum(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

const mix = (a, b, t) => a.map((v, i) => Math.round(v + (b[i] - v) * t));

export function isLight() {
  return document.documentElement.classList.contains("light");
}

export function legible(hex, surfaceVar = "--surface-2") {
  const track = resolveRGB(cssVar(surfaceVar) || (isLight() ? "#fff" : "#000"));
  const toward = isLight() ? [0, 0, 0] : [255, 255, 255];
  let c = resolveRGB(hex || "#888");
  for (let i = 0; i < 14 && contrast(c, track) < 3; i++) c = mix(c, toward, 0.08);
  return `rgb(${c.join(",")})`;
}
