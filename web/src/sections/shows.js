// Tes rendez-vous (conditional: data.shows). The programmes ("émissions") you
// kept company with, aggregated by their stable conceptUuid so the same show
// heard across several nights counts as one standing appointment. Ranked by
// listening time; each row states its own duration and the number of evenings.
// Only the main antenna carries shows, so an events-only, webradio-only, or
// pre-show-tag history collapses the whole section.

import { el } from "../lib/dom.js";
import { fmtDur, num, pct, plural } from "../lib/format.js";
import { finding, caption } from "../lib/section.js";

export function shows(data, mk) {
  const s = data.shows;
  if (!s || !Array.isArray(s.shows) || s.shows.length === 0) return null;
  const { sec, body } = mk({ tc: "01:32", title: "Tes rendez-vous", id: "shows" });

  const list = s.shows;
  const top = list[0];
  const share = s.inShowShare || 0;

  // Finding: how much of the night is spent in curated shows, and with whom.
  body.appendChild(finding(list.length === 1
    ? `<span class="you">${top.name}</span> t’a accompagné ${num(top.evenings)} ${plural(top.evenings, "soirée", "soirées")}.`
    : `<span class="you">${Math.round(share * 100)} %</span> de ton écoute se passe en émission, surtout avec <span class="you">${top.name}</span>.`));

  body.appendChild(el("div", { class: "prose" }, [
    el("p", { html:
      `Les émissions sont les rendez-vous fabriqués de l’antenne, entre deux pans de rotation libre. ` +
      `Voici ceux que tu as le plus suivis, une même émission comptée d’une nuit à l’autre.` }),
  ]));

  const maxSec = list.reduce((m, x) => Math.max(m, x.listeningSec), 0) || 1;
  body.appendChild(el("div", { class: "figure plain" }, [
    el("div", { class: "shows-list" }, list.slice(0, 10).map((x) => {
      const w = Math.max(2, (x.listeningSec / maxSec) * 100);
      const meta = `${num(x.evenings)} ${plural(x.evenings, "soirée", "soirées")} · ${num(x.tracks)} ${plural(x.tracks, "titre", "titres")}`;
      return el("div", { class: "show-row", role: "img", "aria-label": `${x.name}, ${fmtDur(x.listeningSec)}, ${meta}` }, [
        el("div", { class: "hd" }, [
          el("span", { class: "nm", text: x.name }),
          el("span", { class: "v", html: `<b class="stat">${fmtDur(x.listeningSec)}</b> · ${pct(x.share)}` }),
        ]),
        el("div", { class: "bar" }, [el("i", { style: { width: `${w.toFixed(1)}%` } })]),
        el("div", { class: "meta", text: meta }),
      ]);
    })),
  ]));

  body.appendChild(caption(
    `${num(s.distinct)} ${plural(s.distinct, "émission suivie", "émissions suivies")} sur ${fmtDur(s.inShowSec)} d’écoute en émission, ${fmtDur(s.totalSec)} d’écoute en tout. Les barres sont proportionnelles à la plus écoutée.`,
    s.distinct, { threshold: 3, unit: "émissions" }
  ));
  return sec;
}
