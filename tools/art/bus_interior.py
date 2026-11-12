"""Writes the inside of a bus, assets/maps/detail/bus.tmj:
    python3 tools/art/bus_interior.py assets/maps/detail/bus.tmj
An ordinary Tiled map afterwards. Conventions as on the train: map property
"vehicle"; a spawn "aboard"; objects of type "train_exit" on the doors
(bump: bus_exit); seats are signs with dialog "train_seat"; the "train
window" tiles (the top row: the bus's left side, the way it drives to the
right) show the road passing by."""
import json, sys
out = sys.argv[1]
W, H = 12, 5
WORLDS, DUNGEON = 1, 177  # room for the worlds sheet to grow
FLOOR, WALL, WINDOW, SEAT, DOOR = 53, 54, 55, 56, 57
ground = [FLOOR] * (W * H)
def put(x, y, t): ground[y * W + x] = t
for x in range(W):
    put(x, 0, WALL if x in (0, W - 1) else WINDOW)
    put(x, H - 1, DOOR if x in (3, 9) else WALL)
for y in range(1, H - 1):
    put(0, y, WALL)
    put(W - 1, y, WINDOW)  # the windscreen
seats = [(x, y) for y in (1, 3) for x in range(1, 9) if not (y == 3 and x == 3)]
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
obj("aboard", "spawn", 9, 2, [("facing", "string", "left")])
for x in (3, 9):
    obj(f"exit_{x}", "train_exit", x, H - 1, [("bump", "string", "bus_exit")])
for x, y in seats:
    obj(f"seat_{x}_{y}", "sign", x, y, [("dialog", "string", "train_seat")])
obj("driver", "npc", 10, 1, [("dialog", "string", "bus_driver"), ("facing", "string", "right")], DUNGEON + 85)
obj("passenger_1", "npc", 5, 2, [("dialog", "string", "bus_passenger_1"), ("facing", "string", "up")], DUNGEON + 99)
m = {"type": "map", "version": "1.10", "tiledversion": "1.11.0", "orientation": "orthogonal",
     "renderorder": "right-down", "infinite": False, "compressionlevel": -1,
     "width": W, "height": H, "tilewidth": 16, "tileheight": 16, "nextlayerid": 5, "nextobjectid": len(objs) + 1,
     "properties": [{"name": "vehicle", "type": "string", "value": "bus"}],
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
