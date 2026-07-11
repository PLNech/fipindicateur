// One shared tooltip. It enriches marks but never carries the sole access to a
// value (every mark also has a direct label or aria-label).

import { el } from "./dom.js";

let tip;

function ensure() {
  if (!tip) {
    tip = el("div", { class: "tip", role: "status" });
    document.body.appendChild(tip);
  }
  return tip;
}

export function showTip(evt, html) {
  const t = ensure();
  t.innerHTML = html;
  t.style.opacity = "1";
  const x = evt.clientX ?? window.innerWidth / 2;
  const y = evt.clientY ?? window.innerHeight / 2;
  const w = t.offsetWidth;
  t.style.left = Math.min(x + 12, window.innerWidth - w - 8) + "px";
  t.style.top = y + 14 + "px";
}

export function hideTip() {
  if (tip) tip.style.opacity = "0";
}

// Attach tooltip + keyboard/hover parity to a mark.
export function bindTip(node, htmlFn) {
  node.addEventListener("mousemove", (e) => showTip(e, htmlFn()));
  node.addEventListener("mouseleave", hideTip);
  node.addEventListener("focus", (e) => {
    const r = node.getBoundingClientRect();
    showTip({ clientX: r.left + r.width / 2, clientY: r.top }, htmlFn());
  });
  node.addEventListener("blur", hideTip);
}
