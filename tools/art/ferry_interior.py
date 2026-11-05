"""Writes the ferry's deck, assets/maps/detail/ferry.tmj:
    python3 tools/art/ferry_interior.py assets/maps/detail/ferry.tmj
An ordinary Tiled map afterwards. Conventions as on the train: map property
"vehicle"; a spawn "aboard"; objects of type "train_exit" on the gangways
(bump: ferry_exit); benches are signs with dialog "ferry_bench". The
"ferry sea" tiles round the deck, and the sea side of the hull tiles, show
the sea passing by (the game draws the world under them, and all round the
map)."""
import json, sys
out = sys.argv[1]
W, H = 21, 9
WORLDS, DUNGEON = 1, 129
DECK, SEA, BENCH, GANGWAY = 96, 97, 98, 99
N, E, S, Wd = 1, 2, 4, 8
RAIL = {m: 100 + k for k, m in enumerate((N, E, S, Wd, N | Wd, N | E, S | Wd, S | E))}
HULL_NE, HULL_SE, HULL_E, HULL_NW, HULL_SW = 112, 113, 114, 115, 116
ROOF = {m: 117 + k for k, m in enumerate((N | Wd, N, N | E, Wd, 0, E, S | Wd, S, S | E))}
FUNNEL = 126
ground = [SEA] * (W * H)
def put(x, y, t): ground[y * W + x] = t
# The hull: a rounded stern at the left (column 1), straight sides along
# rows 1 and 7, and a pointed bow at the right (columns 16-19), sailing
# right along the middle row.
for y in range(1, 8):
    for x in range(1, 16):
        m = (N if y == 1 else 0) | (S if y == 7 else 0) | (Wd if x == 1 else 0)
        put(x, y, RAIL[m] if m else DECK)
put(1, 1, HULL_NW)
put(1, 7, HULL_SW)
for k, x in enumerate((16, 17, 18)):
    put(x, 1 + k, HULL_NE)
    put(x, 7 - k, HULL_SE)
    for y in range(2 + k, 7 - k):
        put(x, y, DECK)
put(19, 4, HULL_E)
gangways = (4, 12)
for x in gangways:
    put(x, 7, GANGWAY)
# The wheelhouse amidships.
cx0, cy0, cx1, cy1 = 7, 3, 10, 5
for y in range(cy0, cy1 + 1):
    for x in range(cx0, cx1 + 1):
        m = (N if y == cy0 else 0) | (S if y == cy1 else 0) | (Wd if x == cx0 else 0) | (E if x == cx1 else 0)
        put(x, y, ROOF[m])
put(8, 4, FUNNEL)
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
obj("aboard", "spawn", 6, 4, [("facing", "string", "down")])
for x in gangways:
    obj(f"exit_{x}", "train_exit", x, 7, [("bump", "string", "ferry_exit")])
for x, y in benches:
    obj(f"bench_{x}_{y}", "sign", x, y, [("dialog", "string", "ferry_bench")])
obj("passenger_1", "npc", 9, 2, [("dialog", "string", "ferry_passenger_1"), ("facing", "string", "up")], DUNGEON + 87)
obj("passenger_2", "npc", 17, 4, [("dialog", "string", "ferry_passenger_2"), ("facing", "string", "right")], DUNGEON + 99)
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
