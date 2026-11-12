"""Writes the cabin of an airliner, assets/maps/detail/plane.tmj:
    python3 tools/art/plane_interior.py assets/maps/detail/plane.tmj
An ordinary Tiled map afterwards. Conventions as on the train: map property
"vehicle"; a spawn "aboard"; an object of type "train_exit" on the door at
the back (bump: plane_exit); seats are signs with dialog "plane_seat" (rest
until landing); the "train window" tiles along both sides show the view
(the ground near the airports, the clouds at cruise)."""
import json, sys
out = sys.argv[1]
W, H = 18, 6
WORLDS, DUNGEON = 1, 177
FLOOR, WALL, WINDOW, SEAT, DOOR = 53, 54, 55, 56, 57
ground = [FLOOR] * (W * H)
def put(x, y, t): ground[y * W + x] = t
for x in range(W):
    edge = x in (0, 1, W - 3, W - 2, W - 1)
    put(x, 0, WALL if edge else WINDOW)
    put(x, H - 1, WALL if edge else WINDOW)
for y in range(1, H - 1):
    put(0, y, WALL)
    put(W - 1, y, WALL)
put(0, 2, DOOR)  # the door at the back, on the left
# Rows of seats either side of the aisle (row 3), the galley at the back
# and the cockpit door at the front.
seats = [(x, y) for y in (1, 2, 4) for x in range(3, W - 3)]
for x, y in seats:
    put(x, y, SEAT)
put(W - 2, 3, DOOR)  # the cockpit (locked: a wall to walk into)
objs = []
def obj(name, typ, x, y, props=(), gid=None):
    o = {"id": len(objs) + 1, "name": name, "type": typ, "x": x * 16, "y": (y + (1 if gid else 0)) * 16,
         "width": 16, "height": 16, "rotation": 0, "visible": True,
         "properties": [{"name": k, "type": t, "value": v} for k, t, v in props]}
    if gid:
        o["gid"] = gid
    objs.append(o)
obj("aboard", "spawn", 2, 3, [("facing", "string", "right")])
obj("exit", "train_exit", 0, 2, [("bump", "string", "plane_exit")])
obj("cockpit", "sign", W - 2, 3, [("dialog", "string", "plane_cockpit")])
for x, y in seats:
    obj(f"seat_{x}_{y}", "sign", x, y, [("dialog", "string", "plane_seat")])
obj("attendant", "npc", 9, 3, [("dialog", "string", "plane_attendant"), ("facing", "string", "left")], DUNGEON + 98)
obj("passenger_1", "npc", 14, 3, [("dialog", "string", "plane_passenger_1"), ("facing", "string", "up")], DUNGEON + 87)
m = {"type": "map", "version": "1.10", "tiledversion": "1.11.0", "orientation": "orthogonal",
     "renderorder": "right-down", "infinite": False, "compressionlevel": -1,
     "width": W, "height": H, "tilewidth": 16, "tileheight": 16, "nextlayerid": 5, "nextobjectid": len(objs) + 1,
     "properties": [{"name": "vehicle", "type": "string", "value": "plane"}],
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
