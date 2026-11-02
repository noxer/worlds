"""Draws assets/tiles/worlds.png, the project's own tiles (CC0), and writes
its tileset assets/tilesets/worlds.tsj:
    python3 tools/art/worlds_sheet.py assets/tiles/tinybattle.png assets/tiles/worlds.png assets/tilesets/worlds.tsj
Row 0: 0 bicycle, 1 scooter (placeholders), 2 lamp post, 3 its lantern (the
tile above).
Row 1 (16-31): rails, by which sides they connect to: tile 16+mask with
N=1, E=2, S=4, W=8 (named "rail <mask>"). Straights, curves, junctions and
buffer stops; transparent around the track, so they go on the deco layer
over any ground (a rail over a path is a level crossing).
Row 2 (32-47): a wooden pier, by which sides continue the pier (named
"pier <mask>"), outlined on the open sides; it covers the water.
Deep water lives in water.png."""
import json, math, sys
from PIL import Image, ImageDraw
battle, out, tsj = sys.argv[1], sys.argv[2], sys.argv[3]
src = Image.open(battle).convert("RGBA")
def tile(i): return src.crop((i%18*16, i//18*16, i%18*16+16, i//18*16+16))
sheet = Image.new("RGBA", (16*16, 16*3), (0, 0, 0, 0))
INK = (43, 29, 35, 255)
def bike(ox, frame, accent, deck=False):
    d = ImageDraw.Draw(sheet)
    # wheels (outlined rings) at the bottom of the tile
    for cx in (ox+4, ox+12):
        d.ellipse((cx-3, 9, cx+3, 15), outline=INK, fill=(0,0,0,0), width=1)
        d.point((cx, 12), fill=INK)
    if deck:   # scooter: low deck and a tall stem
        d.line((ox+4, 12, ox+12, 12), fill=INK, width=2)
        d.line((ox+12, 12, ox+12, 4), fill=accent, width=2)
        d.line((ox+10, 4, ox+14, 4), fill=INK, width=1)
    else:      # bicycle: diamond frame, saddle and handlebar
        d.line((ox+4, 12, ox+8, 8), fill=accent, width=1)
        d.line((ox+8, 8, ox+12, 12), fill=accent, width=1)
        d.line((ox+8, 8, ox+11, 8), fill=accent, width=1)
        d.line((ox+11, 8, ox+12, 12), fill=accent, width=1)
        d.line((ox+8, 8, ox+7, 6), fill=INK, width=1)
        d.line((ox+6, 6, ox+9, 6), fill=INK, width=1)
        d.line((ox+11, 8, ox+12, 5), fill=INK, width=1)
        d.line((ox+12, 5, ox+14, 5), fill=INK, width=1)
bike(0, 0, (200, 90, 60, 255))
bike(16, 0, (70, 140, 200, 255), deck=True)

# Lamp post, two tiles high, in the Kenney style: dark outline, flat colours.
IRON, IRON_HI = (74, 72, 92, 255), (112, 110, 132, 255)
GLASS, GLASS_HI = (255, 206, 92, 255), (255, 244, 184, 255)
def rect(x0, y0, x1, y1, c):
    ImageDraw.Draw(sheet).rectangle((x0, y0, x1, y1), fill=c)
def lamp(ox):
    # post (bottom tile): pole with a wide foot
    rect(ox+6, 0, ox+9, 15, INK)
    rect(ox+7, 0, ox+8, 12, IRON)
    rect(ox+7, 0, ox+7, 12, IRON_HI)
    rect(ox+4, 11, ox+11, 15, INK)
    rect(ox+5, 12, ox+10, 14, IRON)
    rect(ox+5, 12, ox+10, 12, IRON_HI)
def lantern(ox):
    # lantern (top tile): roof, glass, and the pole below it
    rect(ox+6, 13, ox+9, 15, INK)
    rect(ox+7, 13, ox+8, 15, IRON)
    rect(ox+7, 0, ox+8, 1, INK)            # finial
    rect(ox+5, 2, ox+10, 2, INK)           # roof
    rect(ox+3, 3, ox+12, 5, INK)
    rect(ox+5, 3, ox+10, 4, IRON)
    rect(ox+4, 5, ox+11, 13, INK)          # body
    rect(ox+5, 6, ox+10, 11, GLASS)
    rect(ox+6, 7, ox+7, 9, GLASS_HI)
    rect(ox+5, 12, ox+10, 12, IRON)
lamp(32)
lantern(48)

N, E, S, W = 1, 2, 4, 8
SLEEPER, SLEEPER_HI = (120, 78, 46, 255), (156, 104, 62, 255)
STEEL, STEEL_HI = (150, 152, 168, 255), (214, 216, 228, 255)

def put(img, x, y, c):
    if 0 <= x < 16 and 0 <= y < 16:
        img.putpixel((x, y), c)

def rail_tile(mask):
    """Draws a 16x16 rail tile: a 'track space' where the two rails sit 4 and
    11 pixels from the left of a vertical track, with sleepers every 4 px."""
    t = Image.new("RGBA", (16, 16), (0, 0, 0, 0))
    def vertical(y0, y1):  # sleepers and rails of a vertical track, rows y0..y1
        for y in range(y0, y1 + 1):
            if y % 4 in (1, 2):
                for x in range(2, 14):
                    put(t, x, y, INK if x in (2, 13) or y % 4 == 2 else SLEEPER_HI if y % 4 == 1 else SLEEPER)
        for y in range(y0, y1 + 1):
            for rx in (4, 11):
                put(t, rx - 1, y, INK); put(t, rx, y, STEEL_HI); put(t, rx + 1, y, INK)
    def rot(img, k):  # rotate by k*90 degrees counter-clockwise
        return img.rotate(90 * k)
    arms = [m for m in (N, E, S, W) if mask & m]
    n_s, e_w = bool(mask & N) and bool(mask & S), bool(mask & E) and bool(mask & W)
    if len(arms) == 2 and not n_s and not e_w:
        # A curve: arcs round the corner between the two sides.
        cx = 16 if mask & E else 0
        cy = 0 if mask & N else 16
        for y in range(16):
            for x in range(16):
                dx, dy = x + 0.5 - cx, y + 0.5 - cy
                r = math.hypot(dx, dy)
                ang = math.degrees(math.atan2(abs(dy), abs(dx)))  # 0..90
                if 1.5 <= r <= 14.5 and any(abs(ang - a) < 5.5 for a in (11, 34, 56, 79)):
                    put(t, x, y, SLEEPER)
        for y in range(16):
            for x in range(16):
                dx, dy = x + 0.5 - cx, y + 0.5 - cy
                r = math.hypot(dx, dy)
                for rr in (4.5, 11.5):
                    if abs(r - rr) < 0.5:
                        put(t, x, y, STEEL_HI)
                    elif abs(r - rr) < 1.5:
                        put(t, x, y, INK)
        return t
    # Straight pieces and arms from the centre.
    def piece(full, half):  # a vertical track: full height, or the top half
        p = Image.new("RGBA", (16, 16), (0, 0, 0, 0))
        nonlocal t
        keep = t
        t = p
        if full:
            vertical(0, 15)
        elif half:
            vertical(0, 8)
        t = keep
        return p
    layers = []
    if n_s:
        layers.append(piece(True, False))
    else:
        if mask & N: layers.append(piece(False, True))
        if mask & S: layers.append(rot(piece(False, True), 2))
    if e_w:
        layers.append(rot(piece(True, False), 1))
    else:
        if mask & E: layers.append(rot(piece(False, True), 3))
        if mask & W: layers.append(rot(piece(False, True), 1))
    if not layers:
        layers.append(rot(piece(True, False), 1))  # a lone piece: lying east-west
    for l in layers:
        t.alpha_composite(l)
    if len(arms) == 1:  # buffer stop at the open end
        bump = Image.new("RGBA", (16, 16), (0, 0, 0, 0))
        d = ImageDraw.Draw(bump)
        d.rectangle((2, 7, 13, 11), fill=INK)
        d.rectangle((3, 8, 12, 9), fill=(200, 60, 50, 255))
        k = {N: 0, W: 1, S: 2, E: 3}[arms[0]]
        t.alpha_composite(bump.rotate(90 * k))
    return t

PLANK, PLANK_HI, PLANK_DARK = (176, 122, 72, 255), (204, 150, 92, 255), (130, 84, 50, 255)

def pier_tile(mask):
    t = Image.new("RGBA", (16, 16), PLANK)
    across = not (mask & (E | W)) or (mask & (N | S))  # boards run east-west on a north-south pier
    for i in range(16):
        for j in range(16):
            seam = (j % 4 == 3) if across else (i % 4 == 3)
            hi = (j % 4 == 0) if across else (i % 4 == 0)
            if seam:
                t.putpixel((i, j), PLANK_DARK)
            elif hi:
                t.putpixel((i, j), PLANK_HI)
    d = ImageDraw.Draw(t)
    # Outline and shadow on the open sides, posts at their ends.
    if not mask & N: d.rectangle((0, 0, 15, 0), fill=INK)
    if not mask & S: d.rectangle((0, 14, 15, 15), fill=INK); d.rectangle((0, 14, 15, 14), fill=PLANK_DARK)
    if not mask & W: d.rectangle((0, 0, 0, 15), fill=INK)
    if not mask & E: d.rectangle((15, 0, 15, 15), fill=INK)
    for x, y, side in ((1, 13, S | W), (13, 13, S | E)):
        if not mask & S and (not mask & (side & (E | W))):
            d.rectangle((x, y, x + 1, y + 2), fill=INK)
    return t

for m in range(16):
    sheet.alpha_composite(rail_tile(m), (16 * m, 16))
    sheet.alpha_composite(pier_tile(m), (16 * m, 32))
sheet.save(out)

def prop(name, typ, value):
    return {"name": name, "type": typ, "value": value}
tiles = [
    {"id": 2, "properties": [prop("solid", "bool", True), prop("above", "int", 3), prop("name", "string", "lamp_post")]},
    {"id": 3, "properties": [prop("light", "float", 3.5)]},
]
tiles += [{"id": 16 + m, "properties": [prop("name", "string", f"rail {m}")]} for m in range(16)]
tiles += [{"id": 32 + m, "properties": [prop("name", "string", f"pier {m}")]} for m in range(16)]
ts = {"type": "tileset", "version": "1.10", "tiledversion": "1.11.0", "name": "worlds",
      "tilewidth": 16, "tileheight": 16, "tilecount": 48, "columns": 16,
      "image": "../tiles/worlds.png", "imagewidth": 256, "imageheight": 48,
      "margin": 0, "spacing": 0, "tiles": tiles}
with open(tsj, "w") as f:
    f.write(json.dumps(ts, indent=1) + "\n")
