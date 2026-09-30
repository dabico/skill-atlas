# /// script
# requires-python = ">=3.10"
# dependencies = ["playwright>=1.50"]
# ///
"""Screenshot and record a skill-atlas HTML report.

Usage: uv run html.py <report.html> <out-dir> [filter-query]
"""

import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

from playwright.sync_api import sync_playwright

DESKTOP = {"width": 1280, "height": 800}
PHONE = {"width": 390, "height": 844}


def launch(p):
    # Installed Chrome avoids a Playwright browser download; DEMO_BROWSER_CHANNEL= uses bundled Chromium.
    channel = os.environ.get("DEMO_BROWSER_CHANNEL", "chrome") or None
    try:
        return p.chromium.launch(channel=channel)
    except Exception as e:
        sys.exit(f"html.py: can't start browser ({e}).\nInstall Chrome, or run: uvx playwright install chromium; then DEMO_BROWSER_CHANNEL= demo.sh html")


def filter_to(page, query):
    page.keyboard.press("/")
    page.keyboard.type(query, delay=120)
    page.wait_for_selector("#fcount:not([hidden])")


def shots(browser, url, out, query):
    for name, opts in [
        ("html-light.png", {"viewport": DESKTOP, "color_scheme": "light"}),
        ("html-dark.png", {"viewport": DESKTOP, "color_scheme": "dark"}),
        ("html-phone.png", {"viewport": PHONE, "device_scale_factor": 2, "is_mobile": True}),
    ]:
        ctx = browser.new_context(**opts)
        page = ctx.new_page()
        page.goto(url)
        page.screenshot(path=out / name)
        ctx.close()

    ctx = browser.new_context(viewport=DESKTOP)
    page = ctx.new_page()
    page.goto(url)
    filter_to(page, query)
    page.screenshot(path=out / "html-filter.png")
    # Jump to the first match's section.
    page.locator("#toc li:visible a").first.click()
    page.wait_for_timeout(300)
    page.screenshot(path=out / "html-skill.png")
    ctx.close()


def video(browser, url, out, query):
    # Playwright records with its own ffmpeg build; no-op once it's in ~/Library/Caches/ms-playwright.
    subprocess.run([sys.executable, "-m", "playwright", "install", "ffmpeg"], check=True, stdout=subprocess.DEVNULL)
    tmp = Path(tempfile.mkdtemp())
    try:
        ctx = browser.new_context(viewport=DESKTOP, record_video_dir=tmp, record_video_size=DESKTOP)
        page = ctx.new_page()
        page.goto(url)
        page.wait_for_timeout(1200)
        for _ in range(4):
            page.mouse.wheel(0, 500)
            page.wait_for_timeout(400)
        page.keyboard.press("Home")
        page.wait_for_timeout(800)
        filter_to(page, query)
        page.wait_for_timeout(1200)
        page.locator("#toc li:visible a").first.click()
        page.wait_for_timeout(2000)
        page.keyboard.press("Home")
        page.keyboard.press("Escape")
        page.wait_for_timeout(1200)
        webm = page.video.path()
        ctx.close()  # flushes the video

        ff = ["ffmpeg", "-y", "-loglevel", "error", "-i", str(webm)]
        subprocess.run(ff + ["-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", str(out / "html-tour.mp4")], check=True)
        gif = "fps=12,scale=960:-1:flags=lanczos,split[a][b];[a]palettegen[p];[b][p]paletteuse"
        subprocess.run(ff + ["-vf", gif, str(out / "html-tour.gif")], check=True)
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def main():
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    report = Path(sys.argv[1]).resolve()
    out = Path(sys.argv[2]).resolve()
    query = sys.argv[3] if len(sys.argv) > 3 else "laconic"
    out.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as p:
        browser = launch(p)
        shots(browser, report.as_uri(), out, query)
        video(browser, report.as_uri(), out, query)
        browser.close()


if __name__ == "__main__":
    main()
