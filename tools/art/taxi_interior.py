"""Writes the inside of a taxi, assets/maps/detail/taxi.tmj:
    python3 tools/art/taxi_interior.py assets/maps/detail/taxi.tmj
An ordinary Tiled map afterwards. Conventions as on the train: map property
"vehicle"; a spawn "aboard"; objects of type "train_exit" on the doors
(bump: taxi_exit); the back seat is a sign with dialog "train_seat" (rest
until there); the "train window" tiles show the way passing by."""
import json, sys
out = sys.argv[1]
W, H = 8, 4
WORLDS, DUNGEON = 1, 129
FLOOR, WALL, WINDOW, SEAT, DOOR = 53, 54, 55, 56, 57
ground = [FLOOR] * (W * H)
def put(x, y, t): ground[y * W + x] = t
for x in range(W):
    put(x, 0, WALL if x in (0, W - 1) else WINDOW)
    put(x, H - 1, DOOR if x in (2, 5) else WALL)
for y in range(1, H - 1):
    put(0, y, WALL)
    put(W - 1, y, WINDOW)  # the windscreen
seats = [(1, 1), (2, 1), (3, 1)]  # the back seat
for x, y in seats:
    put(x, y, SEAT)
objs = []
def obj(name, typ, x, y, props=(), gid=None):
    o = {"id": len(objs) + 1, "name": name, "type": typ, "x": x * 16, "y": (y + (1 if gid else 0)) * 16,
         "width": 16, "height": 16, "rotation": 0, "visible": True,
         "properties": [{"name": k, "type": t, "value": v} for k, t, v in props]}
    if gid:
        o["gid"] = gid
    objs.append(o)
obj("aboard", "spawn", 2, 2, [("facing", "string", "up")])
for x in (2, 5):
    obj(f"exit_{x}", "train_exit", x, H - 1, [("bump", "string", "taxi_exit")])
for x, y in seats:
    obj(f"seat_{x}_{y}", "sign", x, y, [("dialog", "string", "train_seat")])
obj("driver", "npc", 6, 1, [("dialog", "string", "taxi_driver"), ("facing", "string", "right")], DUNGEON + 97)
m = {"type": "map", "version": "1.10", "tiledversion": "1.11.0", "orientation": "orthogonal",
     "renderorder": "right-down", "infinite": False, "compressionlevel": -1,
     "width": W, "height": H, "tilewidth": 16, "tileheight": 16, "nextlayerid": 5, "nextobjectid": len(objs) + 1,
     "properties": [{"name": "vehicle", "type": "string", "value": "taxi"}],
     "tilesets": [{"firstgid": WORLDS, "source": "../../tilesets/worlds.tsj"},
                  {"firstgid": DUNGEON, "source": "../../tilesets/tinydungeon.tsj"}],
     "layers": [
         {"id": 1, "name": "ground", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": [t + WORLDS for t in ground]},
         {"id": 2, "name": "deco", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": [0] * (W * H)},
         {"id": 3, "name": "overhead", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": [0] * (W * H),
          "properties": [{"name": "overhead", "type": "bool", "value": True}]},
         {"id": 4, "name": "objects", "type": "objectgroup", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "draworder": "topdown", "objects": objs}]}
with open(out, "w") as f:
    f.write(json.dumps(m, indent=1) + "\n")
