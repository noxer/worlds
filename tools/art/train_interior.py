"""Writes the inside of a train carriage, assets/maps/detail/train.tmj:
    python3 tools/art/train_interior.py assets/maps/detail/train.tmj
An ordinary Tiled map afterwards. Conventions: map property "vehicle"; a
spawn "aboard" where passengers arrive; objects of type "train_exit" on the
doors (bump: train_exit); seats are signs with dialog "train_seat"; the
"train window" tiles show the world passing outside."""
import json, sys
out = sys.argv[1]
W, H = 14, 6
WORLDS, DUNGEON = 1, 97
FLOOR, WALL, WINDOW, SEAT, DOOR = 53, 54, 55, 56, 57
ground = [FLOOR] * (W * H)
def put(x, y, t): ground[y * W + x] = t
for x in range(W):
    put(x, 0, WALL if x in (0, 3, 6, 9, 13) else WINDOW)
    put(x, H - 1, DOOR if x in (3, 10) else WALL)
for y in range(1, H - 1):
    put(0, y, WALL)
    put(W - 1, y, WALL)
seats = [(x, y) for y in (1, 4) for x in range(1, W - 1) if x not in (3, 10)]
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
obj("aboard", "spawn", 6, 2, [("facing", "string", "down")])
for x in (3, 10):
    obj(f"exit_{x}", "train_exit", x, H - 1, [("bump", "string", "train_exit")])
for x, y in seats:
    obj(f"seat_{x}_{y}", "sign", x, y, [("dialog", "string", "train_seat")])
obj("passenger_1", "npc", 2, 3, [("dialog", "string", "train_passenger_1"), ("facing", "string", "up")], DUNGEON + 86)
obj("passenger_2", "npc", 11, 2, [("dialog", "string", "train_passenger_2"), ("facing", "string", "down")], DUNGEON + 98)
m = {"type": "map", "version": "1.10", "tiledversion": "1.11.0", "orientation": "orthogonal",
     "renderorder": "right-down", "infinite": False, "compressionlevel": -1,
     "width": W, "height": H, "tilewidth": 16, "tileheight": 16, "nextlayerid": 5, "nextobjectid": len(objs) + 1,
     "properties": [{"name": "vehicle", "type": "string", "value": "train"}],
     "tilesets": [{"firstgid": WORLDS, "source": "../../tilesets/worlds.tsj"},
                  {"firstgid": DUNGEON, "source": "../../tilesets/tinydungeon.tsj"}],
     "layers": [
         {"id": 1, "name": "ground", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": [t + WORLDS - 1 + 1 if t else 0 for t in ground]},
         {"id": 2, "name": "deco", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": [0] * (W * H)},
         {"id": 3, "name": "overhead", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": [0] * (W * H),
          "properties": [{"name": "overhead", "type": "bool", "value": True}]},
         {"id": 4, "name": "objects", "type": "objectgroup", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "draworder": "topdown", "objects": objs}]}
with open(out, "w") as f:
    f.write(json.dumps(m, indent=1) + "\n")
