// 8. Palmares: achievements as drawn SVG insignia keyed by ID. The emoji field
// is ignored entirely (zero emoji on the page). Locked medals show progress.

import { el } from "../lib/dom.js";
import { plural } from "../lib/format.js";
import { finding, caption } from "../lib/section.js";
import { insigne } from "../insignia.js";

export function palmares(data, mk) {
  const ach = data.achievements || [];
  if (ach.length === 0) return null;
  const { sec, body } = mk({ tc: "04:30", title: "Palmarès", id: "palmares" });

  const unlocked = ach.filter((a) => a.unlocked).length;
  body.appendChild(finding(`<span class="you">${unlocked}</span> ${plural(unlocked, "insigne décroché", "insignes décrochés")} sur ${ach.length}.`));

  body.appendChild(el("div", { class: "insignia" }, ach.map((a) => {
    const p = a.target > 0 ? Math.min(100, (a.current / a.target) * 100) : 0;
    const cur = Number.isInteger(a.current) ? a.current : a.current.toFixed(1);
    const tgt = Number.isInteger(a.target) ? a.target : a.target.toFixed(1);
    return el("div", { class: `insigne ${a.unlocked ? "unlocked" : "locked"}` }, [
      el("div", { class: "medal" }, [insigne(a.id)]),
      el("div", { style: { flex: "1", minWidth: "0" } }, [
        el("div", { class: "nm", text: a.name }),
        el("div", { class: "ds", text: a.desc }),
        a.unlocked ? null : el("div", { class: "prog" }, [el("i", { style: { width: `${p}%` } })]),
        el("div", { class: "pt", text: a.unlocked ? "Débloqué" : `${cur} / ${tgt}` }),
      ]),
    ]);
  })));

  body.appendChild(caption("Les insignes verrouillés montrent ta progression.", null));
  return sec;
}
