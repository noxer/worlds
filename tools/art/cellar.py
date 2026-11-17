"""Writes the old cellar outside Brookhaven, assets/maps/detail/cellar.tmj:
    python3 tools/art/cellar.py assets/maps/detail/cellar.tmj
An ordinary Tiled map afterwards. Three rooms: the entry (south) with a
lever that opens the first door; a store room (north-east) with a crate to
push onto a pressure plate, which opens the second door; and the back room
(north-west). Puzzle objects (see internal/scene/puzzle.go): "lever" and
"plate" name the doors they work in "target"; "block" is pushed by walking
into it; "door" opens when every lever and plate wired to it is on."""
import json, sys
out = sys.argv[1]
W, H = 16, 12
DUNGEON, WORLDS = 1, 133
WALL, FACE, FLOOR, STEP = 0, 40, 48, 49              # tinydungeon tiles
LEVER, PLATE, BLOCK, DOOR = 162, 164, 166, 167        # worlds tiles
plan = [
    "################",
    "#ffffff#fffffff#",
    "#ffffff#fffffff#",
    "#ffffff#ffBffff#",
    "#ffffffDfffffff#",
    "#ffffff#fffffff#",
    "#ffffff#ffffPff#",
    "########D#######",
    "#ffLfffffffffff#",
    "#ffffffffffffff#",
    "#ffffffSfffffff#",
    "#######E########",
]
assert all(len(r) == W for r in plan) and len(plan) == H
# A wall face under each wall that has floor below it, for depth.
ground = []
objs = []
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
            below = plan[y + 1][x] if y + 1 < H else "#"
            ground.append(DUNGEON + (FACE if below != "#" and y > 0 else WALL))
        elif c == "E":
            ground.append(DUNGEON + STEP)
        else:
            ground.append(DUNGEON + FLOOR)
        if c == "L":
            obj("lever_1", "lever", x, y, [("target", "string", "door_1")], WORLDS + LEVER)
        elif c == "P":
            obj("plate_1", "plate", x, y, [("target", "string", "door_2")], WORLDS + PLATE)
        elif c == "B":
            obj("crate_1", "block", x, y, (), WORLDS + BLOCK)
        elif c == "D":
            n = "door_1" if y == 7 else "door_2"
            obj(n, "door", x, y, (), WORLDS + DOOR)
        elif c == "S":
            obj("entrance", "spawn", x, y, [("facing", "string", "up")])
        elif c == "E":
            obj("exit", "warp", x, y, [("to_map", "string", "main"), ("to_spawn", "string", "cellar_door")])
m = {"type": "map", "version": "1.10", "tiledversion": "1.11.0", "orientation": "orthogonal",
     "renderorder": "right-down", "infinite": False, "compressionlevel": -1,
     "width": W, "height": H, "tilewidth": 16, "tileheight": 16, "nextlayerid": 5, "nextobjectid": len(objs) + 1,
     "tilesets": [{"firstgid": DUNGEON, "source": "../../tilesets/tinydungeon.tsj"},
                  {"firstgid": WORLDS, "source": "../../tilesets/worlds.tsj"}],
     "layers": [
         {"id": 1, "name": "ground", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": ground},
         {"id": 2, "name": "deco", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": [0] * (W * H)},
         {"id": 3, "name": "overhead", "type": "tilelayer", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "width": W, "height": H, "data": [0] * (W * H),
          "properties": [{"name": "overhead", "type": "bool", "value": True}]},
         {"id": 4, "name": "objects", "type": "objectgroup", "visible": True, "opacity": 1, "x": 0, "y": 0,
          "draworder": "topdown", "objects": objs}]}
with open(out, "w") as f:
    f.write(json.dumps(m, indent=1) + "\n")
