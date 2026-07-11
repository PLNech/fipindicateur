// Tiny DOM/SVG builders. No framework: the page is a static render of one data
// blob, so a couple of element helpers beat any runtime.

const SVGNS = "http://www.w3.org/2000/svg";

export function el(tag, attrs = {}, children = []) {
  const n = document.createElement(tag);
  apply(n, attrs);
  append(n, children);
  return n;
}

export function svg(tag, attrs = {}, children = []) {
  const n = document.createElementNS(SVGNS, tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v == null) continue;
    n.setAttribute(k, v);
  }
  append(n, children);
  return n;
}

function apply(n, attrs) {
  for (const [k, v] of Object.entries(attrs)) {
    if (v == null) continue;
    if (k === "class") n.className = v;
    else if (k === "html") n.innerHTML = v;
    else if (k === "text") n.textContent = v;
    else if (k === "style" && typeof v === "object") Object.assign(n.style, v);
    else if (k.startsWith("on") && typeof v === "function") n.addEventListener(k.slice(2), v);
    else if (k === "dataset") Object.assign(n.dataset, v);
    else n.setAttribute(k, v);
  }
}

function append(n, children) {
  const list = Array.isArray(children) ? children : [children];
  for (const c of list) {
    if (c == null || c === false) continue;
    n.appendChild(typeof c === "string" || typeof c === "number" ? document.createTextNode(String(c)) : c);
  }
}

export const byId = (id) => document.getElementById(id);
