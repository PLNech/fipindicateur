#!/usr/bin/env python3
"""Build the social-preview hero image.

Pipeline (dev-only, not part of `make build`):
  1. inline the committed WOFF2 subsets (web/fonts/) into hero.template.html
     as base64 data URIs, producing the standalone web/hero/hero.html;
  2. render hero.html with headless chromium at 1280x640, deviceScaleFactor 2
     (a 2560x1280 supersample);
  3. downscale to EXACTLY 1280x640 (GitHub social preview 2:1) with Lanczos,
     writing docs/social-preview.png.

Run: python3 web/hero/build.py   (needs playwright + Pillow).
Fonts are subsetted latin + full French (see web/subset-fonts.mjs), so the
wordmark and tagline render without any serif fallback.
"""
import base64
import pathlib
from playwright.sync_api import sync_playwright
from PIL import Image

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parent.parent
FONTS = ROOT / "web" / "fonts"
TEMPLATE = HERE / "hero.template.html"
STANDALONE = HERE / "hero.html"
OUT = ROOT / "docs" / "social-preview.png"
SUPERSAMPLE = HERE / "_hero-2x.png"  # transient

FONT_SLOTS = {
    "__BRICOLAGE_400__": "bricolage-400.woff2",
    "__BRICOLAGE_800__": "bricolage-800.woff2",
    "__LITERATA_400I__": "literata-400i.woff2",
}


def inline_fonts() -> str:
    html = TEMPLATE.read_text(encoding="utf-8")
    for slot, name in FONT_SLOTS.items():
        b64 = base64.b64encode((FONTS / name).read_bytes()).decode("ascii")
        html = html.replace(slot, b64)
    STANDALONE.write_text(html, encoding="utf-8")
    return html


def render():
    with sync_playwright() as p:
        b = p.chromium.launch()
        ctx = b.new_context(
            viewport={"width": 1280, "height": 640},
            device_scale_factor=2,
            reduced_motion="reduce",
        )
        pg = ctx.new_page()
        errs = []
        pg.on("console", lambda m: errs.append(m.text) if m.type == "error" else None)
        pg.on("pageerror", lambda e: errs.append(str(e)))
        pg.goto(STANDALONE.as_uri(), wait_until="networkidle")
        pg.wait_for_timeout(300)
        pg.screenshot(path=str(SUPERSAMPLE), clip={"x": 0, "y": 0, "width": 1280, "height": 640})
        ctx.close()
        b.close()
        if errs:
            print("PAGE ERRORS:", errs)


def downscale():
    img = Image.open(SUPERSAMPLE).convert("RGB")
    img = img.resize((1280, 640), Image.LANCZOS)
    img.save(OUT, optimize=True)
    SUPERSAMPLE.unlink(missing_ok=True)
    print(f"wrote {OUT} ({OUT.stat().st_size // 1024} KB) at {img.size}")


if __name__ == "__main__":
    inline_fonts()
    render()
    downscale()
