"""Writes the ferry's deck, assets/maps/detail/ferry.tmj:
    python3 tools/art/ferry_interior.py assets/maps/detail/ferry.tmj
An ordinary Tiled map afterwards. Conventions as on the train: map property
"vehicle"; a spawn "aboard"; objects of type "train_exit" on the gangways
(bump: ferry_exit); benches are signs with dialog "ferry_bench". The
"ferry sea" tiles round the deck show the sea passing by (the game draws
the world under them, and all round the map)."""
import json, sys
out = sys.argv[1]
W, H = 18, 9
WORLDS, DUNGEON = 1, 129
DECK, SEA, BENCH, GANGWAY = 96, 97, 98, 99
N, E, S, Wd = 1, 2, 4, 8
RAIL = {m: 100 + k for k, m in enumerate((N, E, S, Wd, N | Wd, N | E, S | Wd, S | E))}
ground = [SEA] * (W * H)
def put(x, y, t): ground[y * W + x] = t
x0, y0, x1, y1 = 1, 1, W - 2, H - 2  # the deck
for y in range(y0, y1 + 1):
    for x in range(x0, x1 + 1):
        m = (N if y == y0 else 0) | (S if y == y1 else 0) | (Wd if x == x0 else 0) | (E if x == x1 else 0)
        put(x, y, RAIL[m] if m else DECK)
gangways = (4, 13)
for x in gangways:
    put(x, y1, GANGWAY)
benches = [(x, y) for y in (3, 5) for x in (3, 4, 5, 12, 13, 14)]
for x, y in benches:
    put(x, y, BENCH)
objs = []
def obj(name, typ, x, y, props=(), gid=None):
    o = {"id": len(objs) + 1, "name": name, "type": typ, "x": x * 16, "y": (y + (1 if gid else 0)) * 16,
         "width": 16, "height": 16, "rotation": 0, "visible": True,
         "properties": [{"name": k, "type": t, "value": v} for k, t, v in props]}
    if gid:
        o["gid"] = gid
    objs.append(o)
obj("aboard", "spawn", 8, 4, [("facing", "string", "down")])
for x in gangways:
    obj(f"exit_{x}", "train_exit", x, y1, [("bump", "string", "ferry_exit")])
for x, y in benches:
    obj(f"bench_{x}_{y}", "sign", x, y, [("dialog", "string", "ferry_bench")])
obj("passenger_1", "npc", 9, 2, [("dialog", "string", "ferry_passenger_1"), ("facing", "string", "up")], DUNGEON + 87)
obj("passenger_2", "npc", 15, 6, [("dialog", "string", "ferry_passenger_2"), ("facing", "string", "right")], DUNGEON + 99)
def gid(t): return t + WORLDS if t is not None else 0
m = {"type": "map", "version": "1.10", "tiledversion": "1.11.0", "orientation": "orthogonal",
     "renderorder": "right-down", "infinite": False, "compressionlevel": -1,
     "width": W, "height": H, "tilewidth": 16, "tileheight": 16, "nextlayerid": 5, "nextobjectid": len(objs) + 1,
     "properties": [{"name": "vehicle", "type": "string", "value": "ferry"}],
     "tilesets": [{"firstgid": WORLDS, "source": "../../tilesets/worlds.tsj"},
                  {"firstgid": DUNGEON, "source": "../../tilesets/tinydungeon.tsj"}],
     "layers": [
         {"id": 1, "name": "ground", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": [gid(t) for t in ground]},
         {"id": 2, "name": "deco", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": [0] * (W * H)},
         {"id": 3, "name": "overhead", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": [0] * (W * H),
          "properties": [{"name": "overhead", "type": "bool", "value": True}]},
         {"id": 4, "name": "objects", "type": "objectgroup", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "draworder": "topdown", "objects": objs}]}
with open(out, "w") as f:
    f.write(json.dumps(m, indent=1) + "\n")
