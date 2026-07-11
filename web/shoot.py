#!/usr/bin/env python3
"""Screenshot the report in both themes at desktop + mobile widths, for visual
QA. Not part of the build; dev-only. Outputs to web/screenshots/ (gitignored)."""
import sys, pathlib
from playwright.sync_api import sync_playwright

OUT = pathlib.Path(__file__).parent / "screenshots"
OUT.mkdir(exist_ok=True)

TARGETS = [
    ("real", "file:///tmp/claude-1000/report-real.html"),
    ("fixture", "file:///tmp/claude-1000/report-fixture.html"),
]
VIEWPORTS = [("desktop", 1280), ("mobile", 390)]
THEMES = ["dark", "light"]


def run():
    results = []
    with sync_playwright() as p:
        b = p.chromium.launch()
        for name, url in TARGETS:
            for vp, w in VIEWPORTS:
                for theme in THEMES:
                    ctx = b.new_context(
                        viewport={"width": w, "height": 900},
                        device_scale_factor=2,
                        color_scheme=theme,
                        reduced_motion="reduce",  # settle instantly for a stable capture
                    )
                    pg = ctx.new_page()
                    errs = []
                    pg.on("console", lambda m: errs.append(m.text) if m.type == "error" else None)
                    pg.on("pageerror", lambda e: errs.append(str(e)))
                    pg.goto(url, wait_until="networkidle")
                    pg.wait_for_timeout(400)
                    # sanity: report populated, theme class correct
                    kids = pg.eval_on_selector("#report", "e => e.children.length")
                    is_light = pg.evaluate("document.documentElement.classList.contains('light')")
                    f = OUT / f"{name}-{vp}-{theme}.png"
                    pg.screenshot(path=str(f), full_page=True)
                    results.append((f.name, kids, is_light, errs))
                    ctx.close()
        b.close()
    print(f"{'file':32} sections light? errors")
    for name, kids, light, errs in results:
        print(f"{name:32} {kids:8} {str(light):6} {errs if errs else 'none'}")


run()
