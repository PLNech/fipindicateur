// Fin d'emission -- report entrypoint. Reads the injected data blob, builds the
// conducteur rundown section by section, wires the theme toggle and the scroll
// reveals. Content is rendered by JS but never gated behind animation: the
// reveal default keeps everything visible if motion is off or an observer never
// fires.

import { byId } from "./lib/dom.js";
import { section } from "./lib/section.js";
import { observeReveals, reduced } from "./lib/motion.js";
import { ouverture } from "./sections/ouverture.js";
import { grille } from "./sections/grille.js";
import { zapping } from "./sections/zapping.js";
import { programme } from "./sections/programme.js";
import { shows } from "./sections/shows.js";
import { epochs } from "./sections/epochs.js";
import { ciel } from "./sections/ciel.js";
import { economie } from "./sections/economie.js";
import { gout } from "./sections/gout.js";
import { palmares } from "./sections/palmares.js";
import { fin } from "./sections/fin.js";

const DATA = JSON.parse(byId("fip-data").textContent);

// mk is the section-shell factory handed to every renderer.
const mk = (opts) => section(opts);

// Conditional sections (programme, shows, epochs, ciel, economie, gout) return
// null when their data is absent, and the rundown simply skips them.
const SECTIONS = [grille, zapping, programme, shows, epochs, ciel, economie, gout, palmares, fin];

function build() {
  const root = byId("report");
  root.textContent = "";
  root.appendChild(ouverture(DATA));
  for (const render of SECTIONS) {
    const node = render(DATA, mk);
    if (node) root.appendChild(node);
  }
  wireTheme();
}

function wireTheme() {
  const btn = byId("themeBtn");
  if (!btn) return;
  const light = document.documentElement.classList.contains("light");
  btn.textContent = light ? "Antenne de nuit" : "Édition papier";
  btn.addEventListener("click", () => {
    document.documentElement.classList.toggle("light");
    build(); // legible() and theme-derived chart colours recompute
    revealAll();
  });
}

// After the first paint, animate reveals; after a theme rebuild, show at once.
let firstPaint = true;
function revealAll() {
  if (firstPaint && !reduced()) {
    observeReveals(byId("report"));
  } else {
    byId("report").querySelectorAll(".reveal").forEach((n) => n.classList.add("in"));
  }
  firstPaint = false;
}

build();
revealAll();
