// The conducteur rundown line: a tabular timecode, a thin rule, the segment
// title. This is the page's only kicker grammar (DESIGN.md). Returns the
// <section> and the body node to fill.

import { el, svg } from "./dom.js";

export function section({ tc, title, id, cls = "" }) {
  const body = el("div", { class: "section-body" });
  const sec = el("section", { class: `section reveal ${cls}`.trim(), id }, [
    el("div", { class: "page" }, [
      el("div", { class: "rundown" }, [
        el("span", { class: "tc", text: tc }),
        el("span", { class: "dot", text: "·" }),
        el("h2", { text: title }),
        el("span", { class: "rule", "aria-hidden": "true" }),
      ]),
      body,
    ]),
  ]);
  return { sec, body };
}

// A finding-as-title line. `you` marks the rose-highlighted span.
export function finding(html) {
  return el("p", { class: "finding", html });
}

export function caption(text, n, { threshold = 12, unit = "sessions" } = {}) {
  const c = el("p", { class: "caption" });
  c.append(text);
  if (n != null && n < threshold) {
    c.append(" ");
    c.appendChild(el("span", { class: "warn", text: "Indicatif, pas significatif." }));
  }
  return c;
}
