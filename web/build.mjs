// build.mjs -- assembles the one self-contained report template.
//
//   src/main.js  --esbuild-->  minified IIFE JS
//   src/css/report.css + fonts/*.woff2  -->  @font-face data URIs + minified CSS
//   src/index.html (shell with __FIP_DATA__)  -->  everything inlined
//   =>  ../internal/stats/report.html.tmpl   (committed; Go embeds it verbatim)
//
// The literal __FIP_DATA__ placeholder is preserved exactly once: the Go binary
// replaces it with json.Marshal(Report) at render time. Zero CDN, zero runtime
// network. `make build` never runs this: the generated .tmpl is committed.

import { build } from "esbuild";
import { readFile, writeFile, readdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { dirname, join, basename } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const OUT = join(here, "..", "internal", "stats", "report.html.tmpl");
const DATA_PLACEHOLDER = "__FIP_DATA__";

// Map subsetted WOFF2 files to their @font-face descriptors.
const FACES = [
  ["bricolage-400.woff2", "Bricolage Grotesque", 400, "normal"],
  ["bricolage-700.woff2", "Bricolage Grotesque", 700, "normal"],
  ["bricolage-800.woff2", "Bricolage Grotesque", 800, "normal"],
  ["literata-400.woff2", "Literata", 400, "normal"],
  ["literata-400i.woff2", "Literata", 400, "italic"],
  ["literata-700.woff2", "Literata", 700, "normal"],
];

async function fontFaces() {
  const blocks = [];
  for (const [file, family, weight, style] of FACES) {
    const buf = await readFile(join(here, "fonts", file));
    const b64 = buf.toString("base64");
    blocks.push(
      `@font-face{font-family:"${family}";font-style:${style};font-weight:${weight};font-display:swap;` +
        `src:url(data:font/woff2;base64,${b64}) format("woff2")}`
    );
  }
  return blocks.join("");
}

async function minifyJS() {
  const res = await build({
    entryPoints: [join(here, "src", "main.js")],
    bundle: true,
    minify: true,
    format: "iife",
    target: ["chrome100", "firefox100", "safari15"],
    legalComments: "none",
    write: false,
    charset: "utf8",
  });
  return res.outputFiles[0].text;
}

async function minifyCSS(css) {
  const res = await build({
    stdin: { contents: css, loader: "css" },
    minify: true,
    write: false,
    charset: "utf8",
  });
  return res.outputFiles[0].text;
}

// The em-dash ban (make lint) greps the whole tree, including this generated
// file. Assert here too so a stray U+2014 fails the build early with context.
const EM_DASH = String.fromCharCode(0x2014);
function assertNoEmDash(name, s) {
  const i = s.indexOf(EM_DASH);
  if (i >= 0) {
    throw new Error(`em dash (U+2014) in ${name} near: ${JSON.stringify(s.slice(Math.max(0, i - 30), i + 30))}`);
  }
}

async function main() {
  const [faces, js, cssRaw, shell] = await Promise.all([
    fontFaces(),
    minifyJS(),
    readFile(join(here, "src", "css", "report.css"), "utf8"),
    readFile(join(here, "src", "index.html"), "utf8"),
  ]);
  const css = faces + (await minifyCSS(cssRaw));

  assertNoEmDash("JS bundle", js);
  assertNoEmDash("CSS", css);

  let html = shell
    .replace("/*__CSS__*/", () => css)
    .replace("/*__JS__*/", () => js);

  // The data placeholder must survive verbatim, exactly once.
  const count = html.split(DATA_PLACEHOLDER).length - 1;
  if (count !== 1) throw new Error(`expected __FIP_DATA__ exactly once, found ${count}`);
  assertNoEmDash("HTML shell", html.replace(DATA_PLACEHOLDER, ""));

  await writeFile(OUT, html);
  const kb = (Buffer.byteLength(html) / 1024).toFixed(1);
  console.log(`wrote ${OUT} (${kb} kB, JS ${(js.length / 1024).toFixed(1)} kB, CSS+fonts ${(css.length / 1024).toFixed(1)} kB)`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
