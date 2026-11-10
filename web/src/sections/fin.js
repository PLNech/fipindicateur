// 9. Fin d'emission: the privacy statement. 100% local, on by default, the
// exact file paths, and how to erase or switch off. Same meaning as the
// retired footer, in the night-voice.

import { el } from "../lib/dom.js";

export function fin(data, mk) {
  const { sec, body } = mk({ tc: "05:00", title: "Fin d’émission", id: "fin", cls: "fin" });
  const p = el("p", { class: "prose" });
  p.innerHTML =
    `<b>100 % local, activé par défaut.</b> Ces statistiques sont calculées sur ta machine, ` +
    `à partir de <span class="path">~/.local/share/fipindicateur/history.jsonl</span> (les titres écoutés) ` +
    `et <span class="path">events.jsonl</span> (les actions : lecture, pause, zaps, volume). ` +
    `Rien n’est envoyé sur le réseau, jamais. ` +
    `Un seul réglage les pilote : la case <b>Historique d’écoute (local)</b> des Réglages. ` +
    `Pour tout effacer, menu Réglages · Statistiques · Effacer l’historique ` +
    `(ou supprime simplement les fichiers). Tes verdicts de goût ` +
    `(<span class="path">prefs.jsonl</span>) vivent dans un journal séparé, ` +
    `effacés par une entrée distincte.`;
  body.appendChild(p);
  body.appendChild(el("p", { class: "prose muted", style: { fontSize: "0.85rem", marginTop: "24px" }, html:
    `Bonne nuit. <span style="color:var(--fip-ink)">·</span> Rendez-vous à la prochaine antenne.` }));
  return sec;
}
