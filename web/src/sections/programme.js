// La regle des 48 heures (conditional: data.programme). FIP claims it never
// plays the same title twice within 48h on a given antenna, and its programmers
// build three-hour music blocks by hand. We hold the claim up against what the
// listener actually heard, keeping the night loop (an automated rerun) apart
// from a genuine daytime catch. Absent history collapses the whole section.

import { el } from "../lib/dom.js";
import { fmtDur, num, plural } from "../lib/format.js";
import { finding, caption } from "../lib/section.js";

export function programme(data, mk) {
  const p = data.programme;
  if (!p || !p.rule48h) return null;
  const { sec, body } = mk({ tc: "01:20", title: "La règle des 48 heures", id: "programme" });

  const r = p.rule48h;
  const c = p.conducteurs || { curatedSec: 0, nightSec: 0, blocksEstimate: 0 };
  const strict = r.strictRepeats || 0;
  const nights = r.nightRepeats || 0;
  const observed = r.observedSec || 0;
  const antenne = p.sameStation ? "une même antenne" : "toutes stations confondues";

  // Finding: the verdict from your own data.
  body.appendChild(finding(strict > 0
    ? `Ton oreille a surpris <span class="you">${num(strict)} ${plural(strict, "fois", "fois")}</span> le même titre repassé en moins de 48 h.`
    : `Sur <span class="you">${fmtDur(observed)}</span> d’écoute, aucun titre n’est repassé en moins de 48 h.`));

  // The claim, stated as a claim.
  body.appendChild(el("div", { class: "prose" }, [
    el("p", { html:
      `FIP revendique ne jamais diffuser deux fois le même titre en moins de 48 heures sur ${antenne}. ` +
      (strict > 0
        ? `Voici les fois où tes oreilles ont pris la programmation en défaut.`
        : `Promesse tenue, pour autant que tes oreilles aient pu en juger.`) }),
  ]));

  // Caught strict repeats: a small direct-labelled table (artist, title, gap).
  if (strict > 0 && Array.isArray(r.caught) && r.caught.length > 0) {
    body.appendChild(el("div", { class: "figure" }, [
      el("div", { class: "repeats-list" }, r.caught.map((it) =>
        el("div", { class: "repeat-row" }, [
          el("span", { class: "nm", html: `${it.artist}${it.title ? ` <span class="ti">· ${it.title}</span>` : ""}` }),
          el("span", { class: "st", text: it.stationName || "" }),
          el("span", { class: "gap", text: fmtDur(it.gapSec) }),
        ])
      )),
    ]));
  }

  // Night repeats: expected, not a violation. Explained apart.
  if (nights > 0) {
    body.appendChild(el("div", { class: "hint-box" }, [
      el("span", { class: "tag", text: "La boucle de nuit" }),
      el("p", { class: "prose", style: { margin: "8px 0 0", fontSize: "0.95rem" }, html:
        `De 22 h à 7 h, l’antenne rediffuse la journée en automatique. ` +
        `<span class="stat">${num(nights)}</span> ${plural(nights, "rediffusion s’est glissée", "rediffusions se sont glissées")} dans ces heures : ` +
        `c’est la boucle de nuit, une rediffusion attendue, pas une entorse à la règle.` }),
    ]));
  }

  // Conducteurs: the hand-crafted three-hour blocks, with the sourced quote.
  body.appendChild(el("div", { class: "prose", style: { marginTop: "var(--s4)" } }, [
    el("p", { html:
      `Tu as traversé environ <span class="stat you">${num(c.blocksEstimate || 0)}</span> ` +
      `${plural(c.blocksEstimate || 0, "enchaînement", "enchaînements")} de trois heures, façonnés à la main : ` +
      `${fmtDur(c.curatedSec || 0)} de programmation de jour, ${fmtDur(c.nightSec || 0)} dans la boucle de nuit.` }),
  ]));
  body.appendChild(el("blockquote", { class: "pullquote" }, [
    el("p", { text: "On fabrique artisanalement des enchaînements de trois heures de musique." }),
    el("cite", { text: "Luc Frelon, programmateur FIP" }),
  ]));

  body.appendChild(caption(
    `${num(r.tracksChecked || 0)} ${plural(r.tracksChecked || 0, "titre examiné", "titres examinés")} sur ${fmtDur(observed)} d’écoute observées. ` +
    `Tu n’entends l’antenne que lorsque tu écoutes : l’absence de rediffusion prouve peu de chose.`,
    r.tracksChecked || 0, { threshold: 200, unit: "titres" }
  ));
  return sec;
}
