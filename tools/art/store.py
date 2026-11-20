"""Writes Brookhaven's general store, assets/maps/detail/store.tmj:
    python3 tools/art/store.py assets/maps/detail/store.tmj
An ordinary Tiled map afterwards. One room: the shopkeeper behind the end
of his counter, shelves and barrels of goods."""
import json, sys
out = sys.argv[1]
W, H = 10, 9
DUNGEON = 1
WALL, FACE, TORCH, FLOOR, STEP, TABLE, STOOL, SHELF, BARREL = 0, 40, 29, 48, 49, 72, 73, 63, 66
plan = [
    "##########",
    "#==t==t==#",
    "#hfhfrrhh#",
    "#ffffffff#",
    "#ccccffff#",
    "#ffffffrr#",
    "#ffffffff#",
    "#ffffffff#",
    "####E#####",
]
assert all(len(r) == W for r in plan) and len(plan) == H
ground, deco, objs = [], [], []
def obj(name, typ, x, y, props=(), gid=None):
    o = {"id": len(objs) + 1, "name": name, "type": typ, "x": x * 16, "y": (y + (1 if gid else 0)) * 16,
         "width": 16, "height": 16, "rotation": 0, "visible": True,
         "properties": [{"name": k, "type": t, "value": v} for k, t, v in props]}
    if gid:
        o["gid"] = gid
    objs.append(o)
for y, row in enumerate(plan):
    for x, c in enumerate(row):
        if c == "#":
            ground.append(DUNGEON + WALL)
        elif c == "=":
            ground.append(DUNGEON + FACE)
        elif c == "t" and y == 1:
            ground.append(DUNGEON + TORCH)
        elif c == "E":
            ground.append(DUNGEON + STEP)
        else:
            ground.append(DUNGEON + FLOOR)
        deco.append({"h": DUNGEON + SHELF, "c": DUNGEON + TABLE, "r": DUNGEON + BARREL, "s": DUNGEON + STOOL}.get(c, 0) if y > 1 else 0)
        if y > 1 and c == "t":
            deco[-1] = DUNGEON + TABLE
obj("shopkeeper", "npc", 5, 4, [("dialog", "string", "shopkeeper"), ("facing", "string", "down")], DUNGEON + 85)
obj("entrance", "spawn", 4, 7, [("facing", "string", "up")])
obj("exit", "warp", 4, 8, [("to_map", "string", "main"), ("to_spawn", "string", "store_door")])
m = {"type": "map", "version": "1.10", "tiledversion": "1.11.0", "orientation": "orthogonal",
     "renderorder": "right-down", "infinite": False, "compressionlevel": -1,
     "width": W, "height": H, "tilewidth": 16, "tileheight": 16, "nextlayerid": 5, "nextobjectid": len(objs) + 1,
     "tilesets": [{"firstgid": DUNGEON, "source": "../../tilesets/tinydungeon.tsj"}],
     "layers": [
         {"id": 1, "name": "ground", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": ground},
         {"id": 2, "name": "deco", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": deco},
         {"id": 3, "name": "overhead", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": [0] * (W * H),
          "properties": [{"name": "overhead", "type": "bool", "value": True}]},
         {"id": 4, "name": "objects", "type": "objectgroup", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "draworder": "topdown", "objects": objs}]}
with open(out, "w") as f:
    f.write(json.dumps(m, indent=1) + "\n")
