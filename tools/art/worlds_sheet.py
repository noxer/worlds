"""Draws assets/tiles/worlds.png, the project's own tiles (CC0):
    python3 tools/art/worlds_sheet.py assets/tiles/tinybattle.png assets/tiles/worlds.png
0: bicycle   1: scooter (placeholders)   2: lamp post   3: its lantern (the
tile above). Deep water lives in water.png."""
import sys
from PIL import Image, ImageDraw
battle, out = sys.argv[1], sys.argv[2]
src = Image.open(battle).convert("RGBA")
def tile(i): return src.crop((i%18*16, i//18*16, i%18*16+16, i//18*16+16))
sheet = Image.new("RGBA", (16*4, 16), (0, 0, 0, 0))
INK = (43, 29, 35, 255)
def bike(ox, frame, accent, deck=False):
    d = ImageDraw.Draw(sheet)
    # wheels (outlined rings) at the bottom of the tile
    for cx in (ox+4, ox+12):
        d.ellipse((cx-3, 9, cx+3, 15), outline=INK, fill=(0,0,0,0), width=1)
        d.point((cx, 12), fill=INK)
    if deck:   # scooter: low deck and a tall stem
        d.line((ox+4, 12, ox+12, 12), fill=INK, width=2)
        d.line((ox+12, 12, ox+12, 4), fill=accent, width=2)
        d.line((ox+10, 4, ox+14, 4), fill=INK, width=1)
    else:      # bicycle: diamond frame, saddle and handlebar
        d.line((ox+4, 12, ox+8, 8), fill=accent, width=1)
        d.line((ox+8, 8, ox+12, 12), fill=accent, width=1)
        d.line((ox+8, 8, ox+11, 8), fill=accent, width=1)
        d.line((ox+11, 8, ox+12, 12), fill=accent, width=1)
        d.line((ox+8, 8, ox+7, 6), fill=INK, width=1)
        d.line((ox+6, 6, ox+9, 6), fill=INK, width=1)
        d.line((ox+11, 8, ox+12, 5), fill=INK, width=1)
        d.line((ox+12, 5, ox+14, 5), fill=INK, width=1)
bike(0, 0, (200, 90, 60, 255))
bike(16, 0, (70, 140, 200, 255), deck=True)

# Lamp post, two tiles high, in the Kenney style: dark outline, flat colours.
IRON, IRON_HI = (74, 72, 92, 255), (112, 110, 132, 255)
GLASS, GLASS_HI = (255, 206, 92, 255), (255, 244, 184, 255)
def rect(x0, y0, x1, y1, c):
    ImageDraw.Draw(sheet).rectangle((x0, y0, x1, y1), fill=c)
def lamp(ox):
    # post (bottom tile): pole with a wide foot
    rect(ox+6, 0, ox+9, 15, INK)
    rect(ox+7, 0, ox+8, 12, IRON)
    rect(ox+7, 0, ox+7, 12, IRON_HI)
    rect(ox+4, 11, ox+11, 15, INK)
    rect(ox+5, 12, ox+10, 14, IRON)
    rect(ox+5, 12, ox+10, 12, IRON_HI)
def lantern(ox):
    # lantern (top tile): roof, glass, and the pole below it
    rect(ox+6, 13, ox+9, 15, INK)
    rect(ox+7, 13, ox+8, 15, IRON)
    rect(ox+7, 0, ox+8, 1, INK)            # finial
    rect(ox+5, 2, ox+10, 2, INK)           # roof
    rect(ox+3, 3, ox+12, 5, INK)
    rect(ox+5, 3, ox+10, 4, IRON)
    rect(ox+4, 5, ox+11, 13, INK)          # body
    rect(ox+5, 6, ox+10, 11, GLASS)
    rect(ox+6, 7, ox+7, 9, GLASS_HI)
    rect(ox+5, 12, ox+10, 12, IRON)
lamp(32)
lantern(48)
sheet.save(out)
