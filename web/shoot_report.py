#!/usr/bin/env python3
"""Render the listening report from FICTIONAL fixture data and screenshot it
(dark theme, desktop) for docs/. Never uses real user data: the only input is
web/fixtures/report-enriched.json. Dev-only, not part of the build.

  1. inject the fixture JSON into internal/stats/report.html.tmpl's __FIP_DATA__;
  2. serve the result over 127.0.0.1 (ephemeral port);
  3. capture dark-theme desktop screenshots (full page + a representative crop).

Run: python3 web/shoot_report.py
"""
import functools
import http.server
import pathlib
import tempfile
import threading
from PIL import Image
from playwright.sync_api import sync_playwright

ROOT = pathlib.Path(__file__).resolve().parent.parent
TMPL = ROOT / "internal" / "stats" / "report.html.tmpl"
FIXTURE = ROOT / "web" / "fixtures" / "report-enriched.json"
SERVE_DIR = pathlib.Path(tempfile.mkdtemp(prefix="fip-report-"))
PAGE = SERVE_DIR / "report-fixture.html"
DOCS = ROOT / "docs"
# The README shot spans the top of the report through the signature Markov
# "zapping" figure; the next segment ("Les epoques") begins here (logical px).
CROP_TO = 3029
OUT_WIDTH = 1180  # 1x logical width, downscaled from the 2x supersample


def inject():
    html = TMPL.read_text(encoding="utf-8")
    data = FIXTURE.read_text(encoding="utf-8")
    PAGE.write_text(html.replace("__FIP_DATA__", data, 1), encoding="utf-8")


def serve():
    handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=str(SERVE_DIR))
    httpd = http.server.HTTPServer(("127.0.0.1", 0), handler)
    port = httpd.server_address[1]
    threading.Thread(target=httpd.serve_forever, daemon=True).start()
    return httpd, port


def main():
    inject()
    httpd, port = serve()
    url = f"http://127.0.0.1:{port}/report-fixture.html"
    with sync_playwright() as p:
        b = p.chromium.launch()
        ctx = b.new_context(
            viewport={"width": 1180, "height": 1000},
            device_scale_factor=2,
            color_scheme="dark",
            reduced_motion="reduce",  # settle instantly, content fully visible
        )
        pg = ctx.new_page()
        errs = []
        pg.on("console", lambda m: errs.append(m.text) if m.type == "error" else None)
        pg.on("pageerror", lambda e: errs.append(str(e)))
        pg.goto(url, wait_until="networkidle")
        pg.wait_for_timeout(700)
        sections = pg.eval_on_selector("#report", "e => e.children.length")
        height = pg.evaluate("document.documentElement.scrollHeight")
        is_light = pg.evaluate("document.documentElement.classList.contains('light')")
        print(f"sections={sections} scrollHeight={height} light={is_light} errors={errs or 'none'}")
        full = SERVE_DIR / "report-full-dark.png"
        pg.screenshot(path=str(full), full_page=True)
        ctx.close()
        b.close()
    httpd.shutdown()

    # Crop top-through-zapping and downscale to the README asset.
    img = Image.open(full).convert("RGB")
    scale = img.width / 1180  # 2x device scale factor
    crop = img.crop((0, 0, img.width, int(CROP_TO * scale)))
    h = int(crop.height * OUT_WIDTH / crop.width)
    out = DOCS / "stats-report.png"
    crop.resize((OUT_WIDTH, h), Image.LANCZOS).save(out, optimize=True)
    print(f"wrote {out} ({OUT_WIDTH}x{h}, {out.stat().st_size // 1024} KB)")


if __name__ == "__main__":
    main()
