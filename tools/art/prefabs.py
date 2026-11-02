"""Writes the starting set of building prefabs (assets/maps/prefabs/*.tmj)
from Tiny Town tiles, in the style of Brookhaven's houses:
    python3 tools/art/prefabs.py assets/maps/prefabs
The files are ordinary Tiled maps; edit them in Tiled afterwards (this
script overwrites them). Conventions (see ARCHITECTURE.md, Infrastructure):
  - map property "kind": house | garden | well; "weight" (default 1);
  - a house has an object of type "door" on its door tile, in the row above
    the bottom row; the cell below it (bottom row) is its front step, which
    the village connects to a street;
  - empty cells keep the generated ground."""
import json, os, sys

out = sys.argv[1]
TOWN = 1  # firstgid of tinytown

def tmj(name, w, h, layers, objects=(), props=()):
    def layer(i, lname, data, extra=()):
        d = {"id": i, "name": lname, "type": "tilelayer", "visible": True, "opacity": 1,
             "x": 0, "y": 0, "width": w, "height": h,
             "data": [0 if v is None else v + TOWN for v in data]}
        if extra:
            d["properties"] = list(extra)
        return d
    m = {"type": "map", "version": "1.10", "tiledversion": "1.11.0", "orientation": "orthogonal",
         "renderorder": "right-down", "infinite": False, "compressionlevel": -1,
         "width": w, "height": h, "tilewidth": 16, "tileheight": 16,
         "nextlayerid": 5, "nextobjectid": len(objects) + 1,
         "properties": [{"name": k, "type": t, "value": v} for k, t, v in props],
         "tilesets": [{"firstgid": TOWN, "source": "../../tilesets/tinytown.tsj"}],
         "layers": [
             layer(1, "ground", layers["ground"]),
             layer(2, "deco", layers["deco"]),
             layer(3, "overhead", layers["overhead"], [{"name": "overhead", "type": "bool", "value": True}]),
             {"id": 4, "name": "objects", "type": "objectgroup", "visible": True, "opacity": 1, "x": 0, "y": 0,
              "draworder": "topdown", "objects": [dict(o, id=i + 1) for i, o in enumerate(objects)]},
         ]}
    with open(os.path.join(out, name + ".tmj"), "w") as f:
        f.write(json.dumps(m, indent=1) + "\n")

def grid(w, h):
    return [None] * (w * h)

# Houses: roof (overhead), gable row and front row (deco), front step.
STYLES = {
    "red": dict(roof=(52, 53, 54), wall=(64, 65, 67, 66), front=(76, 88, 90, 79)),   # stone walls
    "grey": dict(roof=(48, 49, 50), wall=(60, 61, 63, 62), front=(72, 84, 85, 75)),  # timber walls
}

def house(style, w, door, weight):
    s = STYLES[style]
    h = 4
    L = {k: grid(w, h) for k in ("ground", "deco", "overhead")}
    rl, rm, rr = s["roof"]
    wl, wm, wg, wr = s["wall"]
    fl, fwin, fdoor, fr = s["front"]
    for x in range(w):
        L["overhead"][x] = rl if x == 0 else rr if x == w - 1 else rm
        L["deco"][w + x] = wl if x == 0 else wr if x == w - 1 else (wg if x == door else wm)
        L["deco"][2 * w + x] = fl if x == 0 else fr if x == w - 1 else (fdoor if x == door else fwin)
    L["ground"][3 * w + door] = 43  # the front step
    obj = {"name": "door", "type": "door", "x": door * 16, "y": 2 * 16, "width": 16, "height": 16,
           "rotation": 0, "visible": True,
           "properties": [{"name": "bump", "type": "string", "value": "house_locked"},
                          {"name": "solid", "type": "bool", "value": True}]}
    tmj(f"house_{style}_{w}", w, h, L, [obj], [("kind", "string", "house"), ("weight", "int", weight)])

house("red", 3, 1, 2)
house("red", 4, 2, 3)
house("red", 5, 2, 1)
house("grey", 3, 1, 2)
house("grey", 4, 2, 3)
house("grey", 5, 2, 1)

# A garden: flowers and beehives behind a fence.
w, h = 5, 3
L = {k: grid(w, h) for k in ("ground", "deco", "overhead")}
for i in range(w * 2):
    L["ground"][i] = 2
L["deco"][1], L["deco"][3] = 94, 94
L["deco"][w + 2] = 29
for x in range(w):
    L["deco"][2 * w + x] = 80 if x == 0 else 82 if x == w - 1 else 81
tmj("garden_bees", w, h, L, (), [("kind", "string", "garden"), ("weight", "int", 1)])

# The well on the village square: cobbles around it.
w, h = 3, 3
L = {k: grid(w, h) for k in ("ground", "deco", "overhead")}
for i in range(w * h):
    L["ground"][i] = 43
L["overhead"][1] = 92
L["deco"][w + 1] = 104
tmj("well", w, h, L, (), [("kind", "string", "well")])
