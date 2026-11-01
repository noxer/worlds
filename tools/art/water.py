"""Generates assets/tiles/water.png and assets/tilesets/water.tsj: Kenney Tiny
Battle's water tiles plus a deep variant, each with 4 animation frames in
which the wave highlights drift sideways. Row k holds tile type k; column f is
frame f. Re-run after changing the source art:
    python3 tools/art/water.py assets/tiles/tinybattle.png assets/tiles/water.png assets/tilesets/water.tsj
"""
import json, sys
from PIL import Image
src, out_png, out_tsj = sys.argv[1:4]
im = Image.open(src).convert("RGBA")
def tile(i): return im.crop((i % 18 * 16, i // 18 * 16, i % 18 * 16 + 16, i // 18 * 16 + 16))

BASE = (117, 227, 255, 255)
HIGHLIGHTS = {(217, 247, 255, 255), (159, 235, 255, 255)}
WATER = HIGHLIGHTS | {BASE}
WATER_TILES = [3, 4, 18, 19, 20, 21, 22, 36, 37, 38, 39, 54, 55, 56, 57, 72, 73, 74, 75, 90, 91, 92, 93]
OFFSETS = [0, 1, 2, 1]   # waves drift right and back
DURATION = 350           # ms per frame

def components(px):
    seen, comps = set(), []
    for y in range(16):
        for x in range(16):
            if (x, y) in seen or px[x, y] not in HIGHLIGHTS:
                continue
            comp, stack = [], [(x, y)]
            seen.add((x, y))
            while stack:
                cx, cy = stack.pop()
                comp.append((cx, cy))
                for dx in (-1, 0, 1):
                    for dy in (-1, 0, 1):
                        nx, ny = cx + dx, cy + dy
                        if 0 <= nx < 16 and 0 <= ny < 16 and (nx, ny) not in seen and px[nx, ny] in HIGHLIGHTS:
                            seen.add((nx, ny)); stack.append((nx, ny))
            comps.append(comp)
    return comps

def is_wave(px, comp):
    """A wave dash is surrounded by water only; foam touches the shore."""
    for x, y in comp:
        for dx in (-1, 0, 1):
            for dy in (-1, 0, 1):
                nx, ny = x + dx, y + dy
                if 0 <= nx < 16 and 0 <= ny < 16 and px[nx, ny] not in WATER:
                    return False
    return True

def frame(t, off):
    f = t.copy()
    src_px, px = t.load(), f.load()
    for comp in components(src_px):
        if off == 0 or not is_wave(src_px, comp):
            continue
        cells = set(comp)
        targets = [(x + off, y) for x, y in comp]
        if any(not (0 <= x < 16) or (src_px[x, y] != BASE and (x, y) not in cells) for x, y in targets):
            continue  # no room to drift: keep this dash still
        for x, y in comp:
            px[x, y] = BASE
        for (x, y), (tx, ty) in zip(comp, targets):
            px[tx, ty] = src_px[x, y]
    return f

def darken(t):
    t = t.copy(); px = t.load()
    for y in range(16):
        for x in range(16):
            r, g, b, a = px[x, y]
            px[x, y] = (int(r * 0.35), int(g * 0.55), int(b * 0.85), a)
    return t

rows = [(f"battle {i}", tile(i), "shallow") for i in WATER_TILES]
rows.append(("deep", tile(37), "deep"))
sheet = Image.new("RGBA", (16 * len(OFFSETS), 16 * len(rows)), (0, 0, 0, 0))
tiles = []
for k, (name, t, terrain) in enumerate(rows):
    for f, off in enumerate(OFFSETS):
        fr = frame(t, off)
        if terrain == "deep":
            fr = darken(fr)
        sheet.paste(fr, (16 * f, 16 * k))
    first = k * len(OFFSETS)
    for f in range(len(OFFSETS)):
        entry = {"id": first + f, "properties": [{"name": "terrain", "type": "string", "value": terrain}]}
        if f == 0:
            entry["animation"] = [{"tileid": first + g, "duration": DURATION} for g in range(len(OFFSETS))]
        tiles.append(entry)
sheet.save(out_png)
ts = {"type": "tileset", "version": "1.10", "tiledversion": "1.11.0", "name": "water",
      "tilewidth": 16, "tileheight": 16, "tilecount": len(rows) * len(OFFSETS), "columns": len(OFFSETS),
      "image": "../tiles/water.png", "imagewidth": sheet.width, "imageheight": sheet.height,
      "margin": 0, "spacing": 0, "tiles": tiles}
with open(out_tsj, "w") as f:
    json.dump(ts, f, indent=1); f.write("\n")
# index of each source tile's first frame, for map editing scripts
print(json.dumps({name: k * len(OFFSETS) for k, (name, _, _) in enumerate(rows)}))
