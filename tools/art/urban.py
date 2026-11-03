"""Adds tile properties to Kenney's RPG Urban tileset and writes the urban
building prefabs:
    python3 tools/art/urban.py assets/tilesets/rpgurban.tsj assets/maps/prefabs
Buildings are a flat roof (seen from above) on a brick facade; the bottom
row of the facade has the door, which opens onto the sidewalk below.
Prefab map properties: kind "building", style (downtown | residential |
suburb | industrial | station), material (red | orange), weight. The door
object is in the bottom row."""
import json, os, sys

tsj, out = sys.argv[1], sys.argv[2]
COLS = 27

# --- tileset properties -------------------------------------------------
props = {}
def add(i, name, typ, value):
    props.setdefault(i, []).append({"name": name, "type": typ, "value": value})
for r in range(8):                       # brick walls, red and orange
    for c in range(16, 23):
        add(r * COLS + c, "solid", "bool", True)
for base in (81, 108, 135, 89, 116, 143):  # flat roofs, tan and grey
    for k in range(8):
        add(base + k, "solid", "bool", True)
for i in (259, 260, 286, 287, 270, 271, 251, 278, 279, 305, 189, 190, 191, 192):
    add(i, "solid", "bool", True)        # trees, benches, hydrant, bins, mailbox, posts
add(192, "above", "int", 165)            # street lamp: post and double lantern
for bottom, top in ((259, 232), (260, 233), (340, 313), (341, 314)):
    add(bottom, "above", "int", top)     # tall trees: the crown goes in the tile above
add(191, "above", "int", 164)            # street lamp: post and single lantern
add(165, "light", "float", 3.5)
add(165, "light_color", "color", "#fffff0d0")
add(164, "light", "float", 3.0)
add(164, "light_color", "color", "#fffff0d0")
for i in (359, 360, 361, 386, 387, 388):  # lit shop windows at night
    add(i, "light", "float", 1.25)
with open(tsj) as f:
    ts = json.load(f)
ts["tiles"] = [{"id": i, "properties": p} for i, p in sorted(props.items())]
with open(tsj, "w") as f:
    f.write(json.dumps(ts, indent=1) + "\n")

# --- building prefabs ---------------------------------------------------
URBAN = 1    # firstgid of rpgurban in the prefab maps
WORLDS = 487 # firstgid of the project's own tiles (after rpgurban's 486)
BALLAST = ("worlds", 80)
WALLS = {  # rows of a facade: top, ledge, plain, bottom; each (left, middle, right)
    "red": dict(top=(17, 18, 19), ledge=(44, 45, 46), plain=(71, 72, 73), bottom=(98, 99, 100)),
    "orange": dict(top=(125, 126, 127), ledge=(152, 153, 154), plain=(179, 180, 181), bottom=(206, 207, 208)),
}
ROOFS = {
    "tan": ((81, 82, 83), (108, 109, 110), (135, 136, 137)),
    "grey": ((89, 90, 91), (116, 117, 118), (143, 144, 145)),
}

def row3(t, w):
    l, m, r = t
    return [l] + [m] * (w - 2) + [r]

def write(name, w, h, ground, deco, door, style, weight, bump, material, shop=False, extra=()):
    def layer(i, lname, data, extra=None):
        d = {"id": i, "name": lname, "type": "tilelayer", "visible": True, "opacity": 1,
             "x": 0, "y": 0, "width": w, "height": h,
             "data": [0 if v is None else (WORLDS + v[1] if isinstance(v, tuple) else v + URBAN) for v in data]}
        if extra:
            d["properties"] = extra
        return d
    obj = {"id": 1, "name": "door", "type": "door", "x": door * 16, "y": (h - 1) * 16, "width": 16, "height": 16,
           "rotation": 0, "visible": True,
           "properties": [{"name": "bump", "type": "string", "value": bump},
                          {"name": "solid", "type": "bool", "value": True}]}
    m = {"type": "map", "version": "1.10", "tiledversion": "1.11.0", "orientation": "orthogonal",
         "renderorder": "right-down", "infinite": False, "compressionlevel": -1,
         "width": w, "height": h, "tilewidth": 16, "tileheight": 16, "nextlayerid": 5, "nextobjectid": 2,
         "properties": [{"name": "kind", "type": "string", "value": "building"},
                        {"name": "style", "type": "string", "value": style},
                        {"name": "material", "type": "string", "value": material},
                        {"name": "shop", "type": "bool", "value": shop},
                        *[{"name": k, "type": "int", "value": v} for k, v in extra],
                        {"name": "weight", "type": "int", "value": weight}],
         "tilesets": [{"firstgid": URBAN, "source": "../../tilesets/rpgurban.tsj"},
                      {"firstgid": WORLDS, "source": "../../tilesets/worlds.tsj"}],
         "layers": [layer(1, "ground", ground), layer(2, "deco", deco),
                    layer(3, "overhead", [None] * (w * h), [{"name": "overhead", "type": "bool", "value": True}]),
                    {"id": 4, "name": "objects", "type": "objectgroup", "visible": True, "opacity": 1,
                     "x": 0, "y": 0, "draworder": "topdown", "objects": [obj]}]}
    with open(os.path.join(out, name + ".tmj"), "w") as f:
        f.write(json.dumps(m, indent=1) + "\n")

def building(name, style, wall, roof, w, hr, hf, door, kind="plain", weight=1):
    h = hr + hf
    ground, deco = [], [None] * (w * h)
    top, mid, bot = ROOFS[roof]
    for y in range(hr):
        ground += row3(top if y == 0 else bot if y == hr - 1 else mid, w)
    W = WALLS[wall]
    for y in range(hf):
        part = "top" if y == 0 else "bottom" if y == hf - 1 else ("ledge" if hf >= 4 and y == hf // 2 else "plain")
        ground += row3(W[part], w)
    def put(x, y, t):
        deco[y * w + x] = t
    bump = "building_locked"
    for y in range(hr, h):
        bottom = y == h - 1
        for x in range(w):
            if bottom and x == door:
                continue
            if kind == "shop" and bottom:
                put(x, y, 359 if x == 0 else 361 if x == w - 1 else 360)
            elif kind == "shop" and y == h - 2:
                put(x, y, 328 if x == 0 else 330 if x == w - 1 else 329)
            elif kind == "station" and bottom:
                put(x, y, 386 if x == 0 else 388 if x == w - 1 else 387)
            elif kind == "warehouse":
                if bottom and x in (2, w - 3):
                    put(x, y, 339)  # loading bays
                elif not bottom and y == hr and x % 3 == 1:
                    put(x, y, 390)  # a few wide windows high up
            elif (x + y) % 2 == 1 or (kind == "tall" and x % 2 == 1):
                put(x, y, 364 if not bottom else 363)
    if kind == "shop":
        put(door, h - 1, 412)
        bump = "shop_closed"
    elif kind == "station":
        put(door, h - 1, 285)
        put(door + 1, h - 1, 285)
        bump = "station"
        # The track bay: open from the top down to the row above the
        # bottom, where the buffer stop is; walls either side.
        bay = w - 3
        W2 = WALLS[wall]
        for y in range(h - 1):
            ground[y * w + bay] = BALLAST
            deco[y * w + bay] = None
            if y < hr:  # roof edges either side of the bay
                rows = ROOFS[roof]
                part = rows[0] if y == 0 else rows[2] if y == hr - 1 else rows[1]
                ground[y * w + bay - 1] = part[2]
                ground[y * w + bay + 1] = part[0]
            else:
                part = W2["top"] if y == hr else W2["plain"]
                ground[y * w + bay - 1] = part[2]
                ground[y * w + bay + 1] = part[0]
                deco[y * w + bay - 1] = deco[y * w + bay + 1] = None
        write(name, w, h, ground, deco, door, style, weight, bump, wall, False,
              (("bay_x", bay), ("bay_y", h - 2)))
        return
    elif kind == "warehouse":
        put(door, h - 1, 283)
    else:
        put(door, h - 1, 283 if style == "suburb" else 284 if w >= 6 else 282)
    write(name, w, h, ground, deco, door, style, weight, bump, wall, kind == "shop")

n = 0
for wall, roof in (("red", "tan"), ("orange", "grey")):
    for w, hr, hf in ((6, 3, 4), (8, 3, 4), (5, 2, 4)):
        building(f"city_down_{wall}_{w}x{hr+hf}", "downtown", wall, roof, w, hr, hf, w // 2, "tall", 2)
    building(f"city_shop_{wall}_6", "downtown", wall, roof, 6, 2, 3, 3, "shop", 2)
    for w, hr, hf in ((5, 2, 3), (6, 2, 3), (4, 2, 3)):
        building(f"city_res_{wall}_{w}x{hr+hf}", "residential", wall, roof, w, hr, hf, w // 2, "plain", 2)
    building(f"city_shop_{wall}_5", "residential", wall, roof, 5, 2, 3, 2, "shop", 1)
    for w in (4, 5):
        building(f"city_sub_{wall}_{w}", "suburb", wall, roof, w, 2, 2, w // 2, "plain", 1)
    for w in (8, 10):
        building(f"city_ind_{wall}_{w}", "industrial", wall, "grey", w, 2, 3, w // 2, "warehouse", 2 if wall == "orange" else 1)
building("city_station", "station", "orange", "grey", 12, 3, 4, 5, "station", 1)
