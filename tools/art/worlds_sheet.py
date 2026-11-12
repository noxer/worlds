"""Draws assets/tiles/worlds.png, the project's own tiles (CC0), and writes
its tileset assets/tilesets/worlds.tsj:
    python3 tools/art/worlds_sheet.py assets/tiles/tinybattle.png assets/tiles/worlds.png assets/tilesets/worlds.tsj
Row 0: 0 bicycle, 1 scooter (placeholders), 2 lamp post, 3 its lantern (the
tile above), 4 "bus stop" (the post, solid) and 5 its sign (the tile
above), 6-11 the bus seen from above, two tiles long: "bus front e", "bus
back e" (west: mirrored), "bus front s", "bus back s", "bus front n", "bus
back n".
Row 1 (16-31): rails, by which sides they connect to: tile 16+mask with
N=1, E=2, S=4, W=8 (named "rail <mask>"). Straights, curves, junctions and
buffer stops; transparent around the track, so they go on the deco layer
over any ground (a rail over a path is a level crossing).
Row 2 (32-47): a wooden pier, by which sides continue the pier (named
"pier <mask>"), outlined on the open sides; it covers the water.
Row 3 (48-58): trains. Outside, seen from above: "train car h", "train car v",
"train loco e", "train loco s", "train loco n" (west: "loco e" mirrored).
Inside a carriage: "train floor", "train wall", "train window" (glass left
transparent: the game shows the world passing behind it), "train seat",
"train door".
Row 4 (64-79): a railway fence by which sides it connects to ("fence <mask>",
N=1, E=2, S=4, W=8): posts and two rails, transparent round it (deco layer).
Row 3 also (58-63): the ferry outside, seen from above, two tiles long:
"ferry bow e", "ferry stern e" (west: mirrored), "ferry bow s", "ferry stern
s", "ferry bow n", "ferry stern n".
Row 5 (80): "ballast", the gravel bed of tracks in towns; 81-86 the taxi
seen from above, two tiles long: "taxi front e", "taxi back e" (west:
mirrored), "taxi front s", "taxi back s", "taxi front n", "taxi back n".
Row 6 (96-107): on the ferry's deck: "ferry deck", "ferry sea" (transparent:
the game shows the sea passing round the deck), "ferry bench", "ferry
gangway", and the railing round the deck on its outer sides ("ferry rail
<mask>", N=1, E=2, S=4, W=8: masks 1, 2, 4, 8, 9, 3, 12, 6), 96-107.
Row 7 (112-126): the ferry's hull shape: "ferry hull ne" and "ferry hull
se" (the bow's sides, deck below or above the diagonal, sea beyond),
"ferry hull e" (the tip of the bow), "ferry hull nw" and "ferry hull sw"
(the rounded stern), then the wheelhouse roof by its outer sides ("ferry
roof <mask>", masks 9, 1, 3, 8, 0, 2, 12, 4, 6) and "ferry funnel".
Rows 8-10 (128-161): the airliner seen from above, 3x3 tiles each way
("plane e <k>", "plane s <k>", "plane n <k>", k = row*3 + column; west:
"plane e" mirrored, columns swapped), the "departures" board (post, solid,
and the board above it), "runway", "runway mark" (the centre line),
"runway edge n", "runway edge s" (with lights) and "apron".
Deep water lives in water.png."""
import json, math, sys
from PIL import Image, ImageDraw
battle, out, tsj = sys.argv[1], sys.argv[2], sys.argv[3]
src = Image.open(battle).convert("RGBA")
def tile(i): return src.crop((i%18*16, i//18*16, i%18*16+16, i//18*16+16))
sheet = Image.new("RGBA", (16*16, 16*11), (0, 0, 0, 0))
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
    if len(arms) == 3:
        # A junction: the straight line, and curves from the third side
        # to both ends of it (a turnout either way).
        through = N | S if n_s else E | W
        arm = mask & ~through
        t = rail_tile(through)
        ends = (N, S) if through == N | S else (E, W)
        for end in ends:
            t.alpha_composite(rail_tile(arm | end))
        return t
    if len(arms) == 4:
        # A diamond crossing: sleepers both ways, the rails of each line
        # broken where the other's cross them (dark flangeways).
        t = rail_tile(N | S)
        t.alpha_composite(rail_tile(E | W))
        d = ImageDraw.Draw(t)
        d.rectangle((2, 2, 13, 13), fill=INK)          # the crossing plate
        d.rectangle((3, 3, 12, 12), fill=(70, 62, 60, 255))
        for k in (4, 11):                               # east-west rails run through
            d.line((0, k, 15, k), fill=STEEL_HI)
        for x in (4, 11):                               # north-south rails, broken
            for y in range(16):
                near = any(abs(y - k) == 1 for k in (4, 11))
                if near:
                    put(t, x, y, INK)                   # the flangeways
                elif y not in (4, 11):
                    put(t, x, y, STEEL_HI)
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
# --- trains ---------------------------------------------------------------
ROOF, ROOF_HI, ROOF_DARK = (196, 72, 56, 255), (230, 110, 84, 255), (150, 50, 44, 255)
LOCO, LOCO_HI = (58, 96, 140, 255), (92, 136, 186, 255)
GLASS_T = (150, 210, 236, 255)

def car_h(loco=False):
    t = Image.new("RGBA", (16, 16), (0, 0, 0, 0))
    d = ImageDraw.Draw(t)
    body, hi = (LOCO, LOCO_HI) if loco else (ROOF, ROOF_HI)
    right = 14 if loco else 15
    d.rectangle((0, 2, right, 13), fill=INK)
    d.rectangle((1, 3, right - 1, 12), fill=body)
    d.rectangle((1, 3, right - 1, 4), fill=hi)
    if loco:  # rounded nose and cab windows at the front (east)
        d.rectangle((right, 4, 15, 11), fill=INK)
        d.rectangle((right - 3, 5, right - 1, 10), fill=GLASS_T)
        d.rectangle((3, 6, 8, 9), fill=(40, 40, 52, 255))  # engine grille
    else:
        for x in (3, 7, 11):  # roof vents
            d.rectangle((x, 7, x + 1, 8), fill=ROOF_DARK)
        d.rectangle((0, 6, 0, 9), fill=(0, 0, 0, 0))  # coupling gaps
        d.rectangle((15, 6, 15, 9), fill=(0, 0, 0, 0))
    return t

ch, lh = car_h(), car_h(True)
sheet.alpha_composite(ch, (0, 48))
sheet.alpha_composite(ch.rotate(90), (16, 48))                 # vertical car
sheet.alpha_composite(lh, (32, 48))                            # loco facing east
sheet.alpha_composite(lh.rotate(-90), (48, 48))                # facing south
sheet.alpha_composite(lh.rotate(90), (64, 48))                 # facing north

FLOOR, FLOOR_HI = (164, 132, 100, 255), (184, 152, 116, 255)
WALL, WALL_HI = (112, 82, 76, 255), (140, 104, 94, 255)
SEAT, SEAT_HI = (64, 92, 150, 255), (96, 128, 186, 255)
def tile_at(x):
    return (x * 16, 48)
def floor():
    t = Image.new("RGBA", (16, 16), FLOOR)
    for y in range(0, 16, 4):
        ImageDraw.Draw(t).line((0, y, 15, y), fill=FLOOR_HI)
    return t
def wall():
    t = Image.new("RGBA", (16, 16), WALL)
    d = ImageDraw.Draw(t)
    d.rectangle((0, 0, 15, 2), fill=WALL_HI)
    d.rectangle((0, 14, 15, 15), fill=INK)
    return t
def window():
    t = wall()
    d = ImageDraw.Draw(t)
    d.rectangle((1, 3, 14, 12), fill=INK)
    d.rectangle((2, 4, 13, 11), fill=(0, 0, 0, 0))  # the glass: the world shows here
    return t
def seat():
    t = floor()
    d = ImageDraw.Draw(t)
    d.rectangle((2, 2, 13, 13), fill=INK)
    d.rectangle((3, 3, 12, 12), fill=SEAT)
    d.rectangle((3, 3, 12, 5), fill=SEAT_HI)
    return t
def door():
    t = Image.new("RGBA", (16, 16), (150, 152, 168, 255))
    d = ImageDraw.Draw(t)
    d.rectangle((0, 0, 15, 15), outline=INK)
    d.line((7, 1, 7, 14), fill=INK)
    d.rectangle((3, 3, 5, 9), fill=GLASS_T)
    d.rectangle((9, 3, 11, 9), fill=GLASS_T)
    return t
for k, img in enumerate((floor(), wall(), window(), seat(), door())):
    sheet.alpha_composite(img, (16 * (5 + k), 48))
# --- buses ------------------------------------------------------------------
def stop_post(ox):
    rect(ox+7, 0, ox+8, 15, INK)
    rect(ox+7, 0, ox+7, 13, IRON_HI)
    rect(ox+5, 13, ox+10, 15, INK)
    rect(ox+6, 14, ox+9, 14, IRON)
    # timetable panel on the post
    rect(ox+4, 4, ox+11, 10, INK)
    rect(ox+5, 5, ox+10, 9, (240, 240, 232, 255))
    for y in (6, 8):
        rect(ox+6, y, ox+9, y, (120, 120, 132, 255))
def stop_sign(ox):
    rect(ox+7, 9, ox+8, 15, INK)
    rect(ox+7, 9, ox+7, 15, IRON_HI)
    ImageDraw.Draw(sheet).ellipse((ox+2, 0, ox+13, 11), fill=INK)
    ImageDraw.Draw(sheet).ellipse((ox+3, 1, ox+12, 10), fill=(244, 196, 48, 255))
    ImageDraw.Draw(sheet).ellipse((ox+5, 3, ox+10, 8), fill=(46, 120, 74, 255))
    rect(ox+6, 5, ox+9, 6, (244, 196, 48, 255))  # a little "H"
    rect(ox+6, 4, ox+6, 7, (244, 196, 48, 255))
    rect(ox+9, 4, ox+9, 7, (244, 196, 48, 255))
stop_post(64)
stop_sign(80)
BUS, BUS_HI, BUS_SH = (52, 140, 92, 255), (92, 180, 128, 255), (34, 100, 64, 255)
BUS_ROOF = (226, 230, 222, 255)
def bus_e():
    """The bus heading east, 32x16: back tile, front tile."""
    t = Image.new("RGBA", (32, 16), (0, 0, 0, 0))
    d = ImageDraw.Draw(t)
    d.rounded_rectangle((0, 2, 31, 13), radius=3, fill=INK)
    d.rounded_rectangle((1, 3, 30, 12), radius=2, fill=BUS)
    d.line((2, 3, 29, 3), fill=BUS_HI)
    d.line((2, 12, 29, 12), fill=BUS_SH)
    d.rectangle((3, 4, 25, 11), fill=BUS_ROOF)            # the roof
    d.rectangle((3, 4, 25, 4), fill=(250, 250, 246, 255))
    for x in (8, 14, 20):                                 # roof hatches
        d.rectangle((x, 6, x + 2, 9), fill=(176, 182, 176, 255))
    d.rectangle((4, 6, 6, 9), fill=(120, 126, 120, 255))  # the air-con at the back
    d.rectangle((27, 4, 29, 11), fill=GLASS_T)            # the windscreen
    d.line((28, 5, 28, 10), fill=(200, 236, 248, 255))
    d.point((30, 4), fill=(255, 236, 150, 255))           # headlights
    d.point((30, 11), fill=(255, 236, 150, 255))
    return t
be = bus_e()
sheet.alpha_composite(be.crop((16, 0, 32, 16)), (16 * 6, 0))   # 6 front e
sheet.alpha_composite(be.crop((0, 0, 16, 16)), (16 * 7, 0))    # 7 back e
bs = be.rotate(-90, expand=True)  # heading south: back on top
sheet.alpha_composite(bs.crop((0, 16, 16, 32)), (16 * 8, 0))   # 8 front s
sheet.alpha_composite(bs.crop((0, 0, 16, 16)), (16 * 9, 0))    # 9 back s
bn = be.rotate(90, expand=True)   # heading north: front on top
sheet.alpha_composite(bn.crop((0, 0, 16, 16)), (16 * 10, 0))   # 10 front n
sheet.alpha_composite(bn.crop((0, 16, 16, 32)), (16 * 11, 0))  # 11 back n

TAXI, TAXI_HI, TAXI_SH = (244, 196, 48, 255), (255, 226, 120, 255), (200, 150, 30, 255)
def taxi_e():
    """The taxi heading east, 32x16: back tile, front tile."""
    t = Image.new("RGBA", (32, 16), (0, 0, 0, 0))
    d = ImageDraw.Draw(t)
    d.rounded_rectangle((3, 3, 29, 12), radius=4, fill=INK)
    d.rounded_rectangle((4, 4, 28, 11), radius=3, fill=TAXI)
    d.line((6, 4, 26, 4), fill=TAXI_HI)
    d.line((6, 11, 26, 11), fill=TAXI_SH)
    d.rectangle((19, 5, 22, 10), fill=GLASS_T)          # windscreen
    d.rectangle((8, 5, 10, 10), fill=GLASS_T)           # rear window
    d.rectangle((11, 5, 18, 10), fill=TAXI_HI)          # the roof
    d.rectangle((13, 6, 16, 9), fill=INK)               # the roof sign
    for k, (x, y) in enumerate(((14, 7), (15, 8), (13, 6), (16, 9))):
        d.point((x, y), fill=(250, 250, 246, 255))
    d.point((28, 5), fill=(255, 246, 200, 255))          # headlights
    d.point((28, 10), fill=(255, 246, 200, 255))
    d.point((4, 5), fill=(220, 60, 50, 255))             # tail lights
    d.point((4, 10), fill=(220, 60, 50, 255))
    return t
te = taxi_e()
sheet.alpha_composite(te.crop((16, 0, 32, 16)), (16 * 1, 80))  # 81 front e
sheet.alpha_composite(te.crop((0, 0, 16, 16)), (16 * 2, 80))   # 82 back e
ts_ = te.rotate(-90, expand=True)
sheet.alpha_composite(ts_.crop((0, 16, 16, 32)), (16 * 3, 80))  # 83 front s
sheet.alpha_composite(ts_.crop((0, 0, 16, 16)), (16 * 4, 80))   # 84 back s
tn = te.rotate(90, expand=True)
sheet.alpha_composite(tn.crop((0, 0, 16, 16)), (16 * 5, 80))    # 85 front n
sheet.alpha_composite(tn.crop((0, 16, 16, 32)), (16 * 6, 80))   # 86 back n

# --- the ferry ---------------------------------------------------------------
HULL, HULL_SH = (236, 238, 242, 255), (196, 200, 210, 255)
STRIPE = (58, 96, 140, 255)
CABIN, CABIN_HI = (210, 214, 224, 255), (244, 246, 250, 255)
FUNNEL = (196, 72, 56, 255)
def ferry_e():
    """The ferry heading east, 32x16: stern tile, bow tile."""
    t = Image.new("RGBA", (32, 16), (0, 0, 0, 0))
    d = ImageDraw.Draw(t)
    hull = [(1, 2), (24, 2), (31, 8), (24, 13), (1, 13), (0, 12), (0, 3)]
    d.polygon(hull, fill=INK)
    d.polygon([(2, 3), (24, 3), (29, 8), (24, 12), (2, 12), (1, 11), (1, 4)], fill=HULL)
    d.line((2, 3, 24, 3), fill=STRIPE)
    d.line((2, 12, 24, 12), fill=HULL_SH)
    d.line((24, 3, 29, 8), fill=STRIPE)
    d.rectangle((3, 5, 25, 10), fill=PLANK)     # the deck
    for x in range(3, 26, 3):
        d.line((x, 5, x, 10), fill=PLANK_DARK)
    d.rectangle((5, 4, 16, 11), fill=INK)        # the cabin roof
    d.rectangle((6, 5, 15, 10), fill=CABIN)
    d.rectangle((6, 5, 15, 6), fill=CABIN_HI)
    d.rectangle((9, 7, 11, 9), fill=INK)         # the funnel
    d.rectangle((10, 8, 10, 8), fill=FUNNEL)
    d.point((27, 8), fill=INK)                   # a bollard on the bow
    return t
fe = ferry_e()
sheet.alpha_composite(fe.crop((16, 0, 32, 16)), (16 * 10, 48))  # 58 bow e
sheet.alpha_composite(fe.crop((0, 0, 16, 16)), (16 * 11, 48))   # 59 stern e
fs = fe.rotate(-90, expand=True)  # 16x32, heading south: stern on top
sheet.alpha_composite(fs.crop((0, 16, 16, 32)), (16 * 12, 48))  # 60 bow s
sheet.alpha_composite(fs.crop((0, 0, 16, 16)), (16 * 13, 48))   # 61 stern s
fn = fe.rotate(90, expand=True)   # heading north: bow on top
sheet.alpha_composite(fn.crop((0, 0, 16, 16)), (16 * 14, 48))   # 62 bow n
sheet.alpha_composite(fn.crop((0, 16, 16, 32)), (16 * 15, 48))  # 63 stern n

DECK, DECK_HI, DECK_DARK = (196, 150, 98, 255), (220, 176, 120, 255), (150, 106, 64, 255)
RAIL, RAIL_HI = (236, 238, 242, 255), (255, 255, 255, 255)
def deck():
    t = Image.new("RGBA", (16, 16), DECK)
    d = ImageDraw.Draw(t)
    for y in range(0, 16, 4):
        d.line((0, y + 3, 15, y + 3), fill=DECK_DARK)
        d.line((0, y, 15, y), fill=DECK_HI)
    for y, x in ((0, 5), (4, 12), (8, 2), (12, 9)):  # board ends
        d.line((x, y, x, y + 2), fill=DECK_DARK)
    return t
def bench():
    t = deck()
    d = ImageDraw.Draw(t)
    d.rectangle((1, 3, 14, 12), fill=INK)
    d.rectangle((2, 4, 13, 11), fill=(132, 86, 52, 255))
    for y in (5, 8):
        d.line((2, y, 13, y), fill=(168, 114, 70, 255))
    d.line((2, 11, 13, 11), fill=(96, 62, 40, 255))
    return t
def gangway():
    t = deck()
    d = ImageDraw.Draw(t)
    d.rectangle((2, 0, 13, 15), fill=INK)
    d.rectangle((3, 0, 12, 15), fill=(150, 152, 168, 255))
    for y in range(1, 16, 3):
        d.line((3, y, 12, y), fill=(112, 114, 130, 255))
    d.rectangle((0, 13, 15, 15), fill=INK)          # the gate across it
    d.rectangle((1, 14, 14, 14), fill=(232, 192, 64, 255))
    return t
def rail(mask):
    """Railing at the deck's edge, on the sides in mask (N=1, E=2, S=4,
    W=8: the outer sides)."""
    t = deck()
    d = ImageDraw.Draw(t)
    def bar(x0, y0, x1, y1):
        d.rectangle((x0, y0, x1, y1), fill=INK)
        d.rectangle((x0 + 1, y0 + 1, x1 - 1, y1 - 1), fill=RAIL)
    if mask & N: bar(0, 0, 15, 3)
    if mask & S: bar(0, 12, 15, 15)
    if mask & W: bar(0, 0, 3, 15)
    if mask & E: bar(12, 0, 15, 15)
    # posts where the rails meet and at the middle
    for x, y, m in ((6, 0, N), (6, 12, S), (0, 6, W), (12, 6, E)):
        if mask & m:
            d.rectangle((x, y, x + 3, y + 3), fill=INK)
            d.rectangle((x + 1, y + 1, x + 2, y + 2), fill=RAIL_HI)
    return t
RAIL_MASKS = (N, E, S, W, N | W, N | E, S | W, S | E)
for k, img in enumerate([deck(), Image.new("RGBA", (16, 16), (0, 0, 0, 0)), bench(), gangway()] + [rail(m) for m in RAIL_MASKS]):
    sheet.alpha_composite(img, (16 * k, 96))
def masked_deck(inside):
    """Deck where inside(x, y), transparent (the sea) elsewhere, with the
    railing along the boundary."""
    base = deck()
    t = Image.new("RGBA", (16, 16), (0, 0, 0, 0))
    for y in range(16):
        for x in range(16):
            if inside(x + 0.5, y + 0.5):
                t.putpixel((x, y), base.getpixel((x, y)))
    # Railing: cells inside, within 3 px of an outside cell.
    def near_out(x, y, r):
        for dy in range(-r, r + 1):
            for dx in range(-r, r + 1):
                X, Y = x + dx, y + dy
                if 0 <= X < 16 and 0 <= Y < 16 and not inside(X + 0.5, Y + 0.5):
                    return True
        return False
    out = t.copy()
    for y in range(16):
        for x in range(16):
            if t.getpixel((x, y))[3] == 0:
                continue
            if near_out(x, y, 1):
                out.putpixel((x, y), INK)
            elif near_out(x, y, 3):
                out.putpixel((x, y), RAIL)
            elif near_out(x, y, 4):
                out.putpixel((x, y), INK)
    return out
hull = [
    masked_deck(lambda x, y: y >= x),                       # ne: below the falling diagonal
    masked_deck(lambda x, y: y <= 16 - x),                  # se: above the rising diagonal
    masked_deck(lambda x, y: y >= x and y <= 16 - x),       # e: the tip
    masked_deck(lambda x, y: (x - 16) ** 2 + (y - 16) ** 2 <= 256),  # nw: rounded corner
    masked_deck(lambda x, y: (x - 16) ** 2 + y ** 2 <= 256),         # sw
]
ROOFC, ROOF_HI2, ROOF_SH = (178, 188, 206, 255), (214, 222, 236, 255), (140, 150, 170, 255)
def roof(mask):
    t = Image.new("RGBA", (16, 16), ROOFC)
    d = ImageDraw.Draw(t)
    for y in range(1, 16, 4):
        d.line((0, y, 15, y), fill=ROOF_SH)
    if mask & N: d.rectangle((0, 0, 15, 1), fill=INK); d.line((0, 2, 15, 2), fill=ROOF_HI2)
    if mask & S: d.rectangle((0, 13, 15, 15), fill=INK); d.line((0, 12, 15, 12), fill=ROOF_SH)
    if mask & W: d.rectangle((0, 0, 1, 15), fill=INK)
    if mask & E: d.rectangle((14, 0, 15, 15), fill=INK)
    return t
ROOF_MASKS = (N | W, N, N | E, W, 0, E, S | W, S, S | E)
def funnel():
    t = roof(0)
    d = ImageDraw.Draw(t)
    d.ellipse((2, 2, 13, 13), fill=INK)
    d.ellipse((3, 3, 12, 12), fill=FUNNEL)
    d.ellipse((5, 5, 10, 10), fill=INK)
    d.ellipse((4, 3, 8, 6), fill=(230, 110, 84, 255))
    return t
for k, img in enumerate(hull + [roof(m) for m in ROOF_MASKS] + [funnel()]):
    sheet.alpha_composite(img, (16 * k, 112))
# --- aircraft and airports ---------------------------------------------------
FUSE, FUSE_SH = (240, 242, 246, 255), (196, 200, 212, 255)
LIVERY = (44, 88, 160, 255)
WING, WING_SH = (206, 210, 220, 255), (164, 170, 184, 255)
def plane_e():
    t = Image.new("RGBA", (48, 48), (0, 0, 0, 0))
    d = ImageDraw.Draw(t)
    # wings, swept back, and the tailplane
    for sgn in (1, -1):
        def m(pts): return [(x, 24 + sgn * (y - 24)) for x, y in pts]
        d.polygon(m([(31, 21), (22, 21), (11, 2), (16, 2)]), fill=INK)
        d.polygon(m([(29, 21), (23, 21), (13, 4), (16, 4)]), fill=WING)
        d.line(m([(23, 20), (13, 4)]), fill=WING_SH)
        ey = 11 if sgn == 1 else 35
        d.rectangle((21, ey, 25, ey + 2), fill=INK)  # engines
        d.polygon(m([(11, 21), (5, 21), (1, 12), (4, 12)]), fill=INK)
        d.polygon(m([(10, 21), (6, 21), (3, 14), (4, 14)]), fill=WING)
    d.rounded_rectangle((0, 20, 47, 27), radius=4, fill=INK)
    d.rounded_rectangle((1, 21, 46, 26), radius=3, fill=FUSE)
    d.line((3, 26, 44, 26), fill=FUSE_SH)
    d.line((3, 25, 42, 25), fill=LIVERY)
    for x in range(8, 40, 3):
        d.point((x, 22), fill=(90, 110, 140, 255))  # windows
    d.rectangle((41, 22, 43, 24), fill=(40, 52, 70, 255))  # the cockpit
    d.rectangle((1, 23, 9, 24), fill=LIVERY)  # the fin, seen from above
    return t
pe = plane_e()
planes = (pe, pe.rotate(-90), pe.rotate(90))  # east, south, north
for k, img in enumerate(planes):
    for i in range(9):
        r, c = divmod(i, 3)
        n = 128 + 9 * k + i
        sheet.alpha_composite(img.crop((16 * c, 16 * r, 16 * c + 16, 16 * r + 16)), (16 * (n % 16), 16 * (n // 16)))
def at_tile(n): return (16 * (n % 16), 16 * (n // 16))
# The departures board: a post, and a dark board with amber lines above.
post = Image.new("RGBA", (16, 16), (0, 0, 0, 0))
dp = ImageDraw.Draw(post)
dp.rectangle((6, 0, 9, 15), fill=INK); dp.rectangle((7, 0, 8, 13), fill=IRON); dp.rectangle((4, 13, 11, 15), fill=INK)
board = Image.new("RGBA", (16, 16), (0, 0, 0, 0))
db = ImageDraw.Draw(board)
db.rectangle((0, 2, 15, 13), fill=INK); db.rectangle((1, 3, 14, 12), fill=(30, 34, 44, 255))
for y in (5, 8, 11):
    db.line((3, y, 12, y), fill=(255, 186, 60, 255))
db.rectangle((6, 14, 9, 15), fill=INK)
ASPH = (70, 72, 82, 255)
def asphalt():
    t = Image.new("RGBA", (16, 16), ASPH)
    rr = random.Random(11)
    for _ in range(24):
        t.putpixel((rr.randrange(16), rr.randrange(16)), (82, 84, 96, 255))
    return t
import random
runway = asphalt()
mark = asphalt(); ImageDraw.Draw(mark).rectangle((0, 7, 15, 8), fill=(240, 240, 236, 255))
edge_n = asphalt(); de = ImageDraw.Draw(edge_n)
de.rectangle((0, 1, 15, 2), fill=(240, 240, 236, 255)); de.rectangle((7, 0, 8, 0), fill=(255, 220, 90, 255))
edge_s = edge_n.transpose(Image.FLIP_TOP_BOTTOM)
apron = Image.new("RGBA", (16, 16), (178, 178, 172, 255))
da = ImageDraw.Draw(apron)
da.line((0, 15, 15, 15), fill=(150, 150, 146, 255)); da.line((15, 0, 15, 15), fill=(150, 150, 146, 255))
for k, img in enumerate((post, board, runway, mark, edge_n, edge_s, apron)):
    sheet.alpha_composite(img, at_tile(155 + k))
# --- railway fence and ballast ---------------------------------------------
POST, BAR, BAR_HI = (86, 84, 98, 255), (128, 128, 144, 255), (176, 178, 194, 255)
def fence(mask):
    t = Image.new("RGBA", (16, 16), (0, 0, 0, 0))
    d = ImageDraw.Draw(t)
    def bar(x0, y0, x1, y1):
        d.rectangle((x0, y0, x1, y1), fill=INK)
        if x1 - x0 > y1 - y0:
            d.line((x0, y0 + 1, x1, y0 + 1), fill=BAR_HI)
        else:
            d.line((x0 + 1, y0, x0 + 1, y1), fill=BAR_HI)
    for y in (4, 9):  # two rails each way
        if mask & E: bar(8, y, 15, y + 2)
        if mask & W: bar(0, y, 7, y + 2)
    for x in (5, 9):
        if mask & N: bar(x, 0, x + 2, 7)
        if mask & S: bar(x, 8, x + 2, 15)
    d.rectangle((5, 3, 10, 13), fill=INK)   # the post
    d.rectangle((6, 4, 9, 12), fill=POST)
    d.rectangle((6, 4, 9, 5), fill=BAR)
    return t
for m in range(16):
    sheet.alpha_composite(fence(m), (16 * m, 64))
import random
rnd = random.Random(7)
ballast = Image.new("RGBA", (16, 16), (138, 128, 116, 255))
for _ in range(70):
    x, y = rnd.randrange(16), rnd.randrange(16)
    ballast.putpixel((x, y), rnd.choice([(110, 102, 94, 255), (164, 154, 140, 255), (124, 114, 104, 255)]))
sheet.alpha_composite(ballast, (0, 80))
sheet.save(out)

def prop(name, typ, value):
    return {"name": name, "type": typ, "value": value}
tiles = [
    {"id": 2, "properties": [prop("solid", "bool", True), prop("above", "int", 3), prop("name", "string", "lamp_post")]},
    {"id": 3, "properties": [prop("light", "float", 3.5)]},
    {"id": 4, "properties": [prop("solid", "bool", True), prop("above", "int", 5), prop("name", "string", "bus stop")]},
]
for k, name in enumerate(("taxi front e", "taxi back e", "taxi front s", "taxi back s", "taxi front n", "taxi back n")):
    tiles.append({"id": 81 + k, "properties": [prop("name", "string", name)]})
for k, name in enumerate(("bus front e", "bus back e", "bus front s", "bus back s", "bus front n", "bus back n")):
    tiles.append({"id": 6 + k, "properties": [prop("name", "string", name)]})
tiles += [{"id": 16 + m, "properties": [prop("name", "string", f"rail {m}")]} for m in range(16)]
tiles += [{"id": 32 + m, "properties": [prop("name", "string", f"pier {m}")]} for m in range(16)]
for k, name in enumerate(("train car h", "train car v", "train loco e", "train loco s", "train loco n",
                          "train floor", "train wall", "train window", "train seat", "train door")):
    ps = [prop("name", "string", name)]
    if name in ("train wall", "train window", "train seat", "train door"):
        ps.append(prop("solid", "bool", True))
    tiles.append({"id": 48 + k, "properties": ps})
for k, name in enumerate(("ferry bow e", "ferry stern e", "ferry bow s", "ferry stern s", "ferry bow n", "ferry stern n")):
    tiles.append({"id": 58 + k, "properties": [prop("name", "string", name)]})
for k, name in enumerate(["ferry deck", "ferry sea", "ferry bench", "ferry gangway"] + [f"ferry rail {m}" for m in RAIL_MASKS]):
    ps = [prop("name", "string", name)]
    if name != "ferry deck":
        ps.append(prop("solid", "bool", True))
    tiles.append({"id": 96 + k, "properties": ps})
for k, name in enumerate(["ferry hull ne", "ferry hull se", "ferry hull e", "ferry hull nw", "ferry hull sw"]
                         + [f"ferry roof {m}" for m in ROOF_MASKS] + ["ferry funnel"]):
    tiles.append({"id": 112 + k, "properties": [prop("name", "string", name), prop("solid", "bool", True)]})
for k, name in enumerate(["plane e", "plane s", "plane n"]):
    for i in range(9):
        tiles.append({"id": 128 + 9 * k + i, "properties": [prop("name", "string", f"{name} {i}")]})
tiles.append({"id": 155, "properties": [prop("name", "string", "departures"), prop("solid", "bool", True), prop("above", "int", 156)]})
for k, name in enumerate(["runway", "runway mark", "runway edge n", "runway edge s", "apron"]):
    tiles.append({"id": 157 + k, "properties": [prop("name", "string", name)]})
tiles += [{"id": 64 + m, "properties": [prop("name", "string", f"fence {m}"), prop("solid", "bool", True)]} for m in range(16)]
tiles.append({"id": 80, "properties": [prop("name", "string", "ballast"), prop("solid", "bool", True)]})  # no walking on the line in towns
ts = {"type": "tileset", "version": "1.10", "tiledversion": "1.11.0", "name": "worlds",
      "tilewidth": 16, "tileheight": 16, "tilecount": 176, "columns": 16,
      "image": "../tiles/worlds.png", "imagewidth": 256, "imageheight": 176,
      "margin": 0, "spacing": 0, "tiles": tiles}
with open(tsj, "w") as f:
    f.write(json.dumps(ts, indent=1) + "\n")
