// 9. Fin d'emission: the privacy statement. 100% local, the exact file path,
// and how to erase. Same meaning as the retired footer, in the night-voice.

import { el } from "../lib/dom.js";

export function fin(data, mk) {
  const { sec, body } = mk({ tc: "05:00", title: "Fin d’émission", id: "fin", cls: "fin" });
  const p = el("p", { class: "prose" });
  p.innerHTML =
    `<b>100 % local, sur consentement.</b> Ces statistiques sont calculées sur ta machine, ` +
    `à partir de <span class="path">~/.local/share/fipindicateur/events.jsonl</span>. ` +
    `Rien n’est envoyé sur le réseau, jamais. ` +
    `Tu peux tout effacer depuis le menu (Réglages · Statistiques · Effacer), ` +
    `ou simplement supprimer ce fichier. Les titres écoutés ` +
    `(<span class="path">history.jsonl</span>) et tes verdicts de goût ` +
    `(<span class="path">prefs.jsonl</span>) vivent dans des journaux séparés, ` +
    `sous des consentements distincts.`;
  body.appendChild(p);
  body.appendChild(el("p", { class: "prose muted", style: { fontSize: "0.85rem", marginTop: "24px" }, html:
    `Bonne nuit. <span style="color:var(--fip-ink)">·</span> Rendez-vous à la prochaine antenne.` }));
  return sec;
}
