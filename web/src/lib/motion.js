// Motion is enhancement only: content is visible by default, animation adds a
// single opening choreography plus gentle scroll reveals. prefers-reduced-motion
// short-circuits everything to its settled state.

export const reduced = () =>
  !!(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches);

// Reveal on scroll. If the observer never fires (hidden tab, no support), the
// .reveal default keeps content fully visible, so this only ever adds.
export function observeReveals(root = document) {
  const items = root.querySelectorAll(".reveal");
  if (reduced() || !("IntersectionObserver" in window)) {
    items.forEach((n) => n.classList.add("in"));
    return;
  }
  const io = new IntersectionObserver(
    (entries, obs) => {
      for (const e of entries) {
        if (e.isIntersecting) {
          e.target.classList.add("in");
          obs.unobserve(e.target);
        }
      }
    },
    { rootMargin: "0px 0px -8% 0px", threshold: 0.08 }
  );
  items.forEach((n) => io.observe(n));
}

// Count a numeral up once. fmt maps the raw number to display text.
export function countUp(node, to, fmt, ms = 800) {
  if (reduced()) {
    node.textContent = fmt(to);
    return;
  }
  const t0 = performance.now();
  const ease = (t) => 1 - Math.pow(1 - t, 4); // ease-out-quart
  function frame(now) {
    const p = Math.min(1, (now - t0) / ms);
    node.textContent = fmt(to * ease(p));
    if (p < 1) requestAnimationFrame(frame);
  }
  requestAnimationFrame(frame);
}
