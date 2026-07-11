// subset-fonts.mjs
//
// Re-subsets the Fontsource latin WOFF2 faces (Bricolage Grotesque 400/700/800,
// Literata 400/400i/700) down to the exact glyph set the report needs: Latin
// with full French accents plus the typographic punctuation the design uses
// (guillemets, middot, curly quotes, ellipsis, en dash, arrows). Output WOFF2
// lands in web/fonts/ and is committed; build.mjs inlines it as data URIs.
//
// Pure Node (subset-font wraps harfbuzz-wasm), so `npm run fonts` needs no
// Python. Run it only when bumping the font versions; the committed .tmpl and
// web/fonts/*.woff2 mean `make build` never touches Node.

import subsetFont from "subset-font";
import { readFile, writeFile, copyFile, mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const NM = join(here, "node_modules", "@fontsource");
const OUT = join(here, "fonts");

// Glyph inventory. Printable ASCII, the French accented alphabet (both cases),
// and the punctuation the design actually renders. Kept explicit so the subset
// is auditable and stable across font-version bumps.
const ASCII = Array.from({ length: 0x7e - 0x20 + 1 }, (_, i) =>
  String.fromCodePoint(0x20 + i)
).join("");
const FRENCH = "àâäæçéèêëîïôöœùûüÿÀÂÄÆÇÉÈÊËÎÏÔÖŒÙÛÜŸ";
const PUNCT = [
  0x00a0, // no-break space
  0x202f, // narrow no-break space (French spacing before ; : ! ?)
  0x00ab,
  0x00bb, // guillemets
  0x00b7, // middot
  0x00b0, // degree
  0x00e9, // (already in FRENCH, harmless)
  0x2018,
  0x2019, // curly single quotes
  0x201c,
  0x201d, // curly double quotes
  0x2013, // en dash (allowed; em dash is banned)
  0x2026, // ellipsis
  0x2022, // bullet
  0x2192,
  0x2190,
  0x2194, // arrows (safety; charts draw their own)
  0x00d7, // multiplication sign (x in "hours x weekdays" prose)
  0x2212, // minus sign
]
  .map((c) => String.fromCodePoint(c))
  .join("");

const CHARSET = ASCII + FRENCH + PUNCT;

const FACES = [
  ["bricolage-grotesque", "bricolage-grotesque-latin-400-normal", "bricolage-400.woff2"],
  ["bricolage-grotesque", "bricolage-grotesque-latin-700-normal", "bricolage-700.woff2"],
  ["bricolage-grotesque", "bricolage-grotesque-latin-800-normal", "bricolage-800.woff2"],
  ["literata", "literata-latin-400-normal", "literata-400.woff2"],
  ["literata", "literata-latin-400-italic", "literata-400i.woff2"],
  ["literata", "literata-latin-700-normal", "literata-700.woff2"],
];

async function main() {
  await mkdir(OUT, { recursive: true });
  let before = 0;
  let after = 0;
  for (const [pkg, src, dst] of FACES) {
    const buf = await readFile(join(NM, pkg, "files", `${src}.woff2`));
    const out = await subsetFont(buf, CHARSET, { targetFormat: "woff2" });
    await writeFile(join(OUT, dst), out);
    before += buf.length;
    after += out.length;
    console.log(
      `${dst.padEnd(20)} ${String(buf.length).padStart(6)} -> ${String(out.length).padStart(6)} B`
    );
  }
  // Commit the OFL licences alongside the fonts (SIL OFL 1.1 requires the
  // licence travel with the font software).
  await copyFile(join(NM, "bricolage-grotesque", "LICENSE"), join(OUT, "OFL-Bricolage-Grotesque.txt"));
  await copyFile(join(NM, "literata", "LICENSE"), join(OUT, "OFL-Literata.txt"));
  console.log(`\ntotal ${before} -> ${after} B (${((1 - after / before) * 100).toFixed(0)}% smaller)`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
