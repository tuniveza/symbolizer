"""One-off generator for font.py: bakes "brick letters" from Noto Sans Regular.

Run: python3 tools/make_font.py   (needs PIL; brick itself never imports PIL)

Source font: Noto Sans Regular (/usr/share/fonts/noto/NotoSans-Regular.ttf),
Copyright 2022 The Noto Project Authors, SIL Open Font License 1.1 (embedding allowed;
the baked result is renamed "brick letters" as the licence asks for modified versions).

Each baked size B (em size in px) becomes:
  header  8 bytes: B (u16), line height (u16), ascent (u16), 0 (u16)
  96 records of 8 bytes, for characters 32..126 then the box for anything unknown:
          advance (u8), x offset (i8), y offset from the line top (i8), width (u8), height (u8), 0, bitmap offset (u16)
  bitmaps: 4-bit coverage (0..15), two pixels a byte (first pixel in the low half), rows one after
          another, each glyph starting on a byte.
"""
import base64
import os
import zlib

from PIL import Image, ImageDraw, ImageFont

SOURCE = "/usr/share/fonts/noto/NotoSans-Regular.ttf"
SIZES = [12, 14, 16, 18, 20, 24, 28, 32, 40]
HERE = os.path.dirname(os.path.abspath(__file__))


def bake(size):
    font = ImageFont.truetype(SOURCE, size)
    ascent, descent = font.getmetrics()
    records, bitmaps = bytearray(), bytearray()

    def add(advance, x, y, w, h, levels):
        records.extend(bytes([advance, x & 255, y & 255, w, h, 0]) + len(bitmaps).to_bytes(2, "little"))
        packed = bytearray((len(levels) + 1) // 2)
        for i, level in enumerate(levels):
            packed[i // 2] |= level << (4 * (i & 1))
        bitmaps.extend(packed)

    for code in range(32, 127):
        ch = chr(code)
        advance = round(font.getlength(ch))
        x0, y0, x1, y1 = font.getbbox(ch)
        w, h = max(0, x1 - x0), max(0, y1 - y0)
        if code == 32 or w == 0 or h == 0:
            add(advance, 0, 0, 0, 0, [])
            continue
        image = Image.new("L", (w, h), 0)
        ImageDraw.Draw(image).text((-x0, -y0), ch, font=font, fill=255)
        levels = [(v * 15 + 127) // 255 for v in image.tobytes()]
        add(advance, x0, y0, w, h, levels)
    # the box for anything unknown: an outline as tall as a capital letter
    cap = font.getbbox("H")
    stroke = max(1, round(size / 16))
    w, h = max(3, round(size * 0.5)), cap[3] - cap[1]
    levels = [15 if (x < stroke or x >= w - stroke or y < stroke or y >= h - stroke) else 0
              for y in range(h) for x in range(w)]
    add(w + 2 * max(1, round(size / 12)), max(1, round(size / 12)), cap[1], w, h, levels)
    header = size.to_bytes(2, "little") + (ascent + descent).to_bytes(2, "little") + ascent.to_bytes(2, "little") + bytes(2)
    return header + records + bitmaps


def main():
    lines = []
    for size in SIZES:
        raw = bake(size)
        packed = base64.b64encode(zlib.compress(raw, 9)).decode()
        chunks = [packed[i:i + 100] for i in range(0, len(packed), 100)]
        lines.append(f"    {size}: (  # {len(raw)} bytes in a program\n" + "".join(f'        "{c}"\n' for c in chunks) + "    ),")
    out = TEMPLATE.replace("@DATA@", "\n".join(lines))
    with open(os.path.join(HERE, "..", "font.py"), "w") as f:
        f.write(out)
    print("wrote font.py:", ", ".join(f"{s}px" for s in SIZES))


TEMPLATE = '''"""brick letters: the font paint_text draws with.

Made by tools/make_font.py (run once, with PIL) from Noto Sans Regular,
Copyright 2022 The Noto Project Authors, licensed under the SIL Open Font License 1.1
(https://openfontlicense.org), which allows embedding it in programs. The baked, reduced result is
named "brick letters", not Noto. brick itself only uses the standard library: the letters below are
literal data (zlib + base64), unpacked when brick compiles a program.

Each baked size B (the em size in px) is: an 8-byte header (B, line height, ascent, 0 as u16), 96 records
of 8 bytes for characters 32..126 then a box for anything unknown (advance u8, x offset i8, y offset from the
line top i8, width u8, height u8, 0, bitmap offset u16), then 4-bit coverage bitmaps, two pixels a byte
(first pixel in the low half), rows one after another, each glyph starting on a byte.

Programs get only the sizes they need: add(a, sizes) adds the baked sizes nearest to the sizes asked for
(DEFAULT_SIZES, about 31 KB, when the sizes are not known), and the routine paint_font_pick that chooses
between them while the program runs. paint_text scales the chosen size to the size asked for.
"""
import base64
import zlib

from asm import RAX, RCX, RDX, LESS

PACKED = {
@DATA@
}
SIZES = sorted(PACKED)
DEFAULT_SIZES = [12, 16, 20, 28]
_cache = {}


def baked(size):
    """The bytes of one baked size."""
    if size not in _cache:
        _cache[size] = zlib.decompress(base64.b64decode("".join(PACKED[size])))
    return _cache[size]


def score(s, b):
    """How far size s is from baked size b (smaller is better): the same sum paint_font_pick does."""
    return abs(s * 256 // b - 256)


def pick(s, sizes):
    """The baked size used for text of s px, from the sorted list sizes (the first best one wins)."""
    s = min(max(s, 1), 1024)
    best = sizes[0]
    for b in sizes[1:]:
        if score(s, b) < score(s, best):
            best = b
    return best


def sizes_for(wanted):
    """The baked sizes a program needs for these text sizes (None = not known)."""
    if wanted is None:
        return list(DEFAULT_SIZES)
    return sorted({pick(s if s > 0 else 16, SIZES) for s in wanted}) or [16]


def add(a, wanted=None):
    """Add the letters for these text sizes and paint_font_pick (rax = size px -> rax = baked font address)."""
    sizes = sizes_for(wanted)
    for b in sizes:
        a.data.setdefault(f"font_{b}", baked(b))
    # rax = s, clamped to 1..1024; rcx = best score so far, rdx = best address
    a.label("paint_font_pick")
    a.set(RCX, 1)
    a.op("cmp", RAX, RCX)
    a.move_if(LESS, RAX, RCX)
    a.set(RCX, 1024)
    a.op("cmp", RAX, RCX)
    a.move_if(0xF, RAX, RCX)            # GREATER
    a.push(RAX)
    a.address(RDX, f"font_{sizes[0]}")
    a.push(RDX)                          # [rsp] = best address, [rsp+8] = s
    first = True
    for b in sizes:
        # score = |s*256/b - 256|
        a.load(RAX, 4, 8)                # rax = s  (4 = RSP)
        a.shift_left(RAX, 8)
        a.set(RCX, b)
        a.set(RDX, 0)
        a.divide(RCX)
        a.op_n("sub", RAX, 256)
        a.mov(RCX, RAX)
        a.negate(RCX)
        a.op("cmp", RAX, RCX)
        a.move_if(LESS, RAX, RCX)        # rax = |rax|
        if first:
            a.mov(8, RAX)                # r8 = best score
            first = False
        else:
            skip = a.new_label("pick_skip")
            a.op("cmp", RAX, 8)
            a.jump_if(0xD, skip)         # NOT_LESS: keep the earlier one
            a.mov(8, RAX)
            a.address(RDX, f"font_{b}")
            a.store(4, 0, RDX)
            a.label(skip)
    a.pop(RAX)
    a.pop(RCX)
    a.ret()
'''

if __name__ == "__main__":
    main()
