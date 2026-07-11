// 7. A ton gout (conditional: data.tastes). Explicit verdicts (like / dislike)
// drawn as rising / falling wave glyphs, then implicit signals stated plainly
// AS hints, never as verdicts (a zap-out or an early pause is a clue, not a
// judgement).

import { el, svg } from "../lib/dom.js";
import { fmtDay, num, plural } from "../lib/format.js";
import { finding, caption } from "../lib/section.js";

function waveGlyph(up) {
  const d = up ? "M2 15 C6 15 6 5 10 5 C14 5 14 12 18 12" : "M2 5 C6 5 6 15 10 15 C14 15 14 8 18 8";
  return svg("svg", { viewBox: "0 0 20 20", "aria-hidden": "true" }, [
    svg("path", { d, class: up ? "glyph-up" : "glyph-down", fill: "none", "stroke-width": 2, "stroke-linecap": "round", "stroke-linejoin": "round" }),
  ]);
}

export function gout(data, mk) {
  const t = data.tastes;
  if (!t || ((!Array.isArray(t.items) || t.items.length === 0) && !t.implicit)) return null;
  const { sec, body } = mk({ tc: "03:50", title: "À ton goût", id: "gout" });

  const items = Array.isArray(t.items) ? t.items : [];
  const likes = t.likes != null ? t.likes : items.filter((i) => i.verdict === "like").length;
  const dislikes = t.dislikes != null ? t.dislikes : items.filter((i) => i.verdict === "dislike").length;

  body.appendChild(finding(likes + dislikes > 0
    ? `Tu as tranché <span class="you">${likes + dislikes} ${plural(likes + dislikes, "fois", "fois")}</span> : ${likes} pour, ${dislikes} contre.`
    : `Tu laisses surtout parler tes gestes.`));

  if (items.length > 0) {
    body.appendChild(el("div", { class: "figure plain" }, [
      el("div", { class: "verdicts" }, items.slice(0, 12).map((it) => {
        const up = it.verdict === "like";
        return el("div", { class: "verdict" }, [
          waveGlyph(up),
          el("span", { class: "nm", html: `${it.artist}${it.title ? ` <span class="ti">· ${it.title}</span>` : ""}` }),
          el("span", { class: "when", text: it.ts ? fmtDay(it.ts) : "" }),
        ]);
      })),
    ]));
  }

  const im = t.implicit;
  if (im && (im.zapOuts || im.earlyPauses)) {
    body.appendChild(el("div", { class: "hint-box" }, [
      el("span", { class: "tag", text: "Indices, pas des verdicts" }),
      el("p", { class: "prose", style: { margin: "8px 0 0", fontSize: "0.95rem" }, html:
        `Tu as quitté une radio dans la foulée <span class="stat">${num(im.zapOuts || 0)}</span> ${plural(im.zapOuts || 0, "fois", "fois")}, ` +
        `et mis en pause dans les premières secondes <span class="stat">${num(im.earlyPauses || 0)}</span> ${plural(im.earlyPauses || 0, "fois", "fois")}. ` +
        `Ce sont des signaux faibles : peut-être un titre qui ne passait pas, peut-être juste le téléphone qui sonne.` }),
    ]));
  }

  body.appendChild(caption(
    `${items.length} ${plural(items.length, "verdict explicite", "verdicts explicites")}${im ? `, ${im.n || 0} signaux implicites` : ""}.`,
    items.length, { threshold: 10, unit: "verdicts" }
  ));
  return sec;
}
