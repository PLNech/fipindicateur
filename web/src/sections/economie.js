// 6. L'economie du disque (conditional: data.enriched.labels). Where the music
// comes from: independents versus majors, and the labels you lean on. The split
// bar runs the full 0-100 width (no truncated baseline); labels are direct.

import { el } from "../lib/dom.js";
import { fmtDur, pct, plural } from "../lib/format.js";
import { finding, caption } from "../lib/section.js";

export function economie(data, mk) {
  const enr = data.enriched;
  const labels = enr && Array.isArray(enr.labels) ? enr.labels.slice() : [];
  if (labels.length === 0) return null;
  const { sec, body } = mk({ tc: "03:15", title: "L’économie du disque", id: "economie" });

  labels.sort((a, b) => b.seconds - a.seconds);
  const total = labels.reduce((a, b) => a + b.seconds, 0) || 1;
  const indieSec = labels.filter((l) => l.indie).reduce((a, b) => a + b.seconds, 0);
  const indieShare = indieSec / total;

  body.appendChild(finding(indieShare >= 0.5
    ? `Tu roules surtout <span class="you">indépendant</span> : ${Math.round(indieShare * 100)} % du temps.`
    : `Les majors pèsent ${Math.round((1 - indieShare) * 100)} % de ton écoute.`));

  body.appendChild(el("div", { class: "figure" }, [
    el("div", { class: "split-bar", role: "img", "aria-label": `Indépendants ${Math.round(indieShare * 100)} pour cent, majors ${Math.round((1 - indieShare) * 100)} pour cent` }, [
      el("div", { class: "indie", style: { width: `${(indieShare * 100).toFixed(1)}%` } }),
      el("div", { class: "major", style: { width: `${((1 - indieShare) * 100).toFixed(1)}%` } }),
    ]),
    el("div", { class: "split-legend" }, [
      el("span", { html: `<b class="stat you">${Math.round(indieShare * 100)} %</b> indépendants` }),
      el("span", { html: `<b class="stat">${Math.round((1 - indieShare) * 100)} %</b> majors` }),
    ]),
    el("div", { class: "labels-list" }, labels.slice(0, 8).map((l) =>
      el("div", { class: "lbl-row" }, [
        el("span", {}, [el("span", { class: "nm", text: l.name }), l.indie ? el("span", { class: "tag", text: "indé" }) : null]),
        el("span", { class: "v", text: `${fmtDur(l.seconds)} · ${pct(l.seconds / total)}` }),
      ])
    )),
  ]));

  body.appendChild(caption(
    `${labels.length} ${plural(labels.length, "label identifié", "labels identifiés")}. Un label est compté indépendant hors des trois majors.`,
    labels.length, { threshold: 10, unit: "labels" }
  ));
  return sec;
}
