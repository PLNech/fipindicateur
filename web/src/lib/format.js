// French formatting helpers. Numbers use a thin no-break space thousands
// separator; durations read "1 h 05" / "42 min" / "18 s"; dates in long French.

const NBSP = " "; // narrow no-break space (French thousands + before ; : )

export function fmtDur(sec) {
  if (!isFinite(sec) || sec < 0) sec = 0;
  sec = Math.round(sec);
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (h > 0) return `${h}${NBSP}h${NBSP}${String(m).padStart(2, "0")}`;
  if (m > 0) return `${m}${NBSP}min`;
  return `${sec}${NBSP}s`;
}

// Splits a duration into a value + unit so the hero can size them separately.
export function durParts(sec) {
  if (!isFinite(sec) || sec < 0) sec = 0;
  sec = Math.round(sec);
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (h > 0) return { v: `${h}`, u: "h", v2: String(m).padStart(2, "0"), u2: "min" };
  if (m > 0) return { v: `${m}`, u: "min" };
  return { v: `${sec}`, u: "s" };
}

export function num(n) {
  return Math.round(n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, NBSP);
}

export function fmtDate(iso) {
  const d = new Date(iso);
  if (isNaN(d)) return "";
  return d.toLocaleDateString("fr-FR", { day: "numeric", month: "long", year: "numeric" });
}

export function fmtDay(iso) {
  const d = new Date(iso);
  if (isNaN(d)) return "";
  return d.toLocaleDateString("fr-FR", { day: "numeric", month: "short" });
}

export function pct(x) {
  return `${Math.round(x * 100)}${NBSP}%`;
}

export function plural(n, one, many) {
  return n <= 1 ? one : many;
}
