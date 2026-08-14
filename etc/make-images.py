#!/usr/bin/env python3
"""Generate the site's raster images from the same design as logo.svg.

SVG covers the favicon for every browser that matters, but three consumers
still want raster: link-preview crawlers (og:image), iOS home screens
(apple-touch-icon), and anything old enough to ask for /favicon.ico. Rather
than hand-maintain four files, generate them.

    python etc/make-images.py

Writes web/og-image.png, web/apple-touch-icon.png and web/favicon.ico.
Re-run after changing logo.svg or the palette; the outputs are committed, so
the site build needs neither Python nor a rasteriser.
"""

import os
import sys

from PIL import Image, ImageDraw, ImageFont

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
OUT = os.path.join(ROOT, "web")

ACCENT = (0x6F, 0x42, 0xC1)      # --accent, matching logo.svg
WHITE = (0xFF, 0xFF, 0xFF)
DIM = (0xD8, 0xC8, 0xF4)         # the accent lightened, for secondary text

# Serif for the Þ (logo.svg asks for Georgia), sans for the wordmark, mono for
# the code line. Each is the first of its candidates that exists on this box.
FONTS = {
    "serif": ["georgia.ttf", "Georgia.ttf",
              "/usr/share/fonts/truetype/dejavu/DejaVuSerif-Bold.ttf",
              "/System/Library/Fonts/Supplemental/Georgia.ttf"],
    "sans": ["segoeui.ttf", "arial.ttf",
             "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
             "/System/Library/Fonts/Supplemental/Arial.ttf"],
    "mono": ["consola.ttf", "cour.ttf",
             "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
             "/System/Library/Fonts/Menlo.ttc"],
}


def font(kind, size):
    for name in FONTS[kind]:
        for path in (name, os.path.join(r"C:\Windows\Fonts", name)):
            try:
                return ImageFont.truetype(path, size)
            except OSError:
                continue
    sys.exit("no %s font found — add one to FONTS in %s" % (kind, __file__))


def rounded_mark(size):
    """The logo: a rounded accent square with a white thorn on it."""
    scale = 4  # supersample, then downscale, for smooth corners and glyph
    img = Image.new("RGBA", (size * scale, size * scale), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)
    draw.rounded_rectangle([0, 0, size * scale - 1, size * scale - 1],
                           radius=int(size * scale * 12 / 64), fill=ACCENT)
    glyph = font("serif", int(size * scale * 44 / 64))
    draw.text((size * scale / 2, size * scale * 46 / 64), "\u00de",
              font=glyph, fill=WHITE, anchor="ms")
    return img.resize((size, size), Image.LANCZOS)


def og_image():
    """1200x630, the size every link-preview crawler expects."""
    w, h = 1200, 630
    img = Image.new("RGB", (w, h), (0x14, 0x16, 0x1A))  # --bg, dark
    draw = ImageDraw.Draw(img)

    mark = rounded_mark(180)
    img.paste(mark, (90, 110), mark)

    draw.text((310, 150), "Thunky", font=font("sans", 108), fill=WHITE)
    draw.text((316, 285), "a toy language, lazily evaluated",
              font=font("sans", 44), fill=DIM)

    code = font("mono", 34)
    draw.text((316, 380), "primes = upFrom 2 > filter isPrime", font=code, fill=DIM)
    draw.text((316, 428), "fibs   = prepend [1;1]", font=code, fill=DIM)
    draw.text((316, 476), "           (zipWith add fibs (tail fibs))",
              font=code, fill=DIM)

    draw.line([(90, 560), (1110, 560)], fill=ACCENT, width=3)
    draw.text((90, 578), "reference \u00b7 tutorial \u00b7 playground",
              font=font("sans", 30), fill=DIM)
    return img


def main():
    og_image().save(os.path.join(OUT, "og-image.png"), optimize=True)

    # iOS composites onto its own background, so this one is opaque.
    touch = Image.new("RGB", (180, 180), ACCENT)
    mark = rounded_mark(180)
    touch.paste(mark, (0, 0), mark)
    touch.save(os.path.join(OUT, "apple-touch-icon.png"), optimize=True)

    rounded_mark(64).save(os.path.join(OUT, "favicon.ico"),
                          sizes=[(16, 16), (32, 32), (48, 48)])
    print("wrote og-image.png, apple-touch-icon.png, favicon.ico into web/")


if __name__ == "__main__":
    main()
