#!/usr/bin/env python3
"""Focused element captures for QA (hero dial, zapping graph)."""
import pathlib
from playwright.sync_api import sync_playwright

OUT = pathlib.Path(__file__).parent / "screenshots"
CASES = [
    ("fixture", "file:///tmp/claude-1000/report-fixture.html", "dark"),
    ("real", "file:///tmp/claude-1000/report-real.html", "dark"),
]
SELECTORS = [("hero", "header.hero"), ("zap", "#zapping .zap"), ("grille", "#grille .figure")]

with sync_playwright() as p:
    b = p.chromium.launch()
    for name, url, theme in CASES:
        ctx = b.new_context(viewport={"width": 1280, "height": 900}, device_scale_factor=2,
                            color_scheme=theme, reduced_motion="reduce")
        pg = ctx.new_page()
        pg.goto(url, wait_until="networkidle")
        pg.wait_for_timeout(300)
        for sname, sel in SELECTORS:
            el = pg.query_selector(sel)
            if el:
                el.screenshot(path=str(OUT / f"zoom-{name}-{sname}.png"))
                print("wrote", f"zoom-{name}-{sname}.png")
        ctx.close()
    b.close()
