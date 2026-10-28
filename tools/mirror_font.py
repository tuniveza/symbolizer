#!/usr/bin/env python3
"""Make "Symbolizer Mirror": the mode icons flipped left to right.

A terminal can't flip a letter, so the flipped icons live in a small font of
their own. The mode block on the left of the top panel shows them, so the two
icons face each other across the title. Each icon X is flipped into
mirrored(X) (the same sum as in modes.go):

    X below U+10000  ->  U+100000 + X              (e.g. U+F040  -> U+10F040)
    X from U+F0000   ->  U+100000 + (X - U+F0000)  (e.g. U+F03D8 -> U+1003D8)

    python3 tools/mirror_font.py    -> fonts/SymbolizerMirror.ttf, copied into ~/.local/share/fonts
"""
import os, shutil, subprocess

from fontTools.pens.recordingPen import DecomposingRecordingPen
from fontTools.pens.reverseContourPen import ReverseContourPen
from fontTools.pens.transformPen import TransformPen
from fontTools.pens.ttGlyphPen import TTGlyphPen
from fontTools.ttLib import TTFont
from fontTools import subset

SOURCE = "/usr/share/fonts/TTF/AgaveNerdFont-Regular.ttf"
HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(HERE, "fonts", "SymbolizerMirror.ttf")
INSTALL = os.path.expanduser("~/.local/share/fonts/SymbolizerMirror.ttf")

# the mode icons (modes.go); keep the two lists the same
ICONS = [0xF040, 0xF03D8, 0xF0428, 0xF02D, 0xF05DA, 0xF0F49]


def mirrored(cp):
    return 0x100000 + cp if cp < 0x10000 else 0x100000 + (cp - 0xF0000)


def main():
    src = TTFont(SOURCE)
    cmap = src.getBestCmap()
    names = {cp: cmap[cp] for cp in ICONS}
    gs = src.getGlyphSet()

    # flipped outlines, drawn before the font is cut down
    flipped = {}
    for cp, name in names.items():
        width = src["hmtx"][name][0]
        rec = DecomposingRecordingPen(gs)
        gs[name].draw(rec)
        pen = TTGlyphPen(None)
        rec.replay(TransformPen(ReverseContourPen(pen), (-1, 0, 0, 1, width, 0)))
        flipped[name] = pen.glyph()

    opts = subset.Options()
    opts.layout_features, opts.name_IDs, opts.notdef_outline = [], ["*"], True
    sub = subset.Subsetter(opts)
    sub.populate(unicodes=ICONS)
    sub.subset(src)

    for name, glyph in flipped.items():
        src["glyf"][name] = glyph
    for table in src["cmap"].tables:
        table.cmap = {}
    for t in list(src["cmap"].tables):
        if not (t.platformID == 3 and t.platEncID == 10):
            src["cmap"].tables.remove(t)
    if not src["cmap"].tables:
        from fontTools.ttLib.tables._c_m_a_p import cmap_format_12
        t = cmap_format_12(12)
        t.platformID, t.platEncID, t.format, t.reserved, t.length, t.language = 3, 10, 12, 0, 0, 0
        src["cmap"].tables.append(t)
    src["cmap"].tables[0].cmap = {mirrored(cp): name for cp, name in names.items()}

    for rec in src["name"].names:
        if rec.nameID in (1, 4, 16, 18, 21):
            rec.string = "Symbolizer Mirror"
        elif rec.nameID == 6:
            rec.string = "SymbolizerMirror-Regular"
        elif rec.nameID == 3:
            rec.string = "Symbolizer Mirror; made from Agave Nerd Font"
    src["OS/2"].ulUnicodeRange4 |= 1 << 26  # Supplementary Private Use Area
    src.save(OUT)
    os.makedirs(os.path.dirname(INSTALL), exist_ok=True)
    shutil.copy(OUT, INSTALL)
    subprocess.run(["fc-cache", "-f", os.path.dirname(INSTALL)], check=False)
    print(OUT, "->", INSTALL)
    for cp in ICONS:
        print(f"  U+{cp:X} -> U+{mirrored(cp):X}")


if __name__ == "__main__":
    main()
