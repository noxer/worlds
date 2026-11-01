"""Draws assets/tiles/worlds.png, the project's own tiles (CC0):
    python3 tools/art/worlds_sheet.py assets/tiles/tinybattle.png assets/tiles/worlds.png
0: bicycle   1: scooter (placeholders). Deep water lives in water.png."""
import sys
from PIL import Image, ImageDraw
battle, out = sys.argv[1], sys.argv[2]
src = Image.open(battle).convert("RGBA")
def tile(i): return src.crop((i%18*16, i//18*16, i%18*16+16, i//18*16+16))
sheet = Image.new("RGBA", (16*2, 16), (0, 0, 0, 0))
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
sheet.save(out)
