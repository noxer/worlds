package scene

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/noxer/worlds/internal/geom"
	"github.com/noxer/worlds/internal/gfx"
	"github.com/noxer/worlds/internal/infra"
	"github.com/noxer/worlds/internal/input"
	"github.com/noxer/worlds/internal/world"
)

// WorldMap is the map of the world, opened from the pause menu. It is
// drawn from the generator's cheap terrain preview, a few rows a frame,
// with the settlements that suit the zoom, the names of cities and of the
// places the party has been to, and where the party is. Arrows pan,
// Confirm (or a tap) zooms in, Cancel zooms out and, at the widest, closes;
// Menu closes.
type WorldMap struct {
	e    *Explore
	zoom int   // index into mapZooms
	cx   int64 // the tile at the centre of the view
	cy   int64
	w, h int

	img    *ebiten.Image
	pix    []byte
	row    int  // rows of pix drawn so far
	dirty  bool // pix needs uploading
	marks  []mapMark
	marked bool // marks are for the current view
}

// mapZooms are the tiles per pixel of each zoom level, widest first.
var mapZooms = []int64{4096, 512, 64, 8}

// mapTiers is the smallest tier shown at each zoom level.
var mapTiers = []infra.Tier{infra.Metropolis, infra.City, infra.Town, infra.Village}

// mapBudget is the time per frame spent drawing the terrain.
const mapBudget = 6 * time.Millisecond

type mapMark struct {
	s       infra.Settlement
	x, y    int // pixel
	visited bool
}

// NewWorldMap opens the map at a medium zoom, centred on the party.
func NewWorldMap(e *Explore) *WorldMap {
	p := e.partyOnMain()
	return &WorldMap{e: e, zoom: 2, cx: p.X, cy: p.Y}
}

// partyOnMain is where the party is on the main map (inside, the last
// place it was outside).
func (e *Explore) partyOnMain() world.Pos {
	if l := e.party.Leader(); l.Pos.Map.IsMain() {
		return l.Pos
	}
	if e.lastMain.Map.IsMain() && e.lastMain != (world.Pos{}) {
		return e.lastMain
	}
	return e.home
}

func (m *WorldMap) scale() int64 { return mapZooms[m.zoom] }

// worldAt returns the tile under pixel (px, py).
func (m *WorldMap) worldAt(px, py int) (int64, int64) {
	z := m.scale()
	return geom.Mod(m.cx+int64(px-m.w/2)*z, m.e.w.W), geom.Mod(m.cy+int64(py-m.h/2)*z, m.e.w.H)
}

// pixelOf returns the pixel of tile (x, y), the shortest way round.
func (m *WorldMap) pixelOf(x, y int64) (int, int) {
	z := m.scale()
	dx, dy := geom.WrapDelta(m.cx, x, m.e.w.W), geom.WrapDelta(m.cy, y, m.e.w.H)
	return m.w/2 + int(geom.FloorDiv(dx, z)), m.h/2 + int(geom.FloorDiv(dy, z))
}

// reset starts drawing the view anew.
func (m *WorldMap) reset() { m.row, m.marked = 0, false }

var (
	mapDeep    = color.RGBA{46, 96, 168, 255}
	mapShallow = color.RGBA{92, 160, 214, 255}
	mapLand    = color.RGBA{118, 170, 92, 255}
	mapUnknown = color.RGBA{27, 27, 40, 255}
)

// Update implements Scene.
func (m *WorldMap) Update(ctx *Context) {
	in := ctx.Input
	if ctx.W != m.w || ctx.H != m.h || m.pix == nil {
		m.w, m.h = ctx.W, ctx.H
		m.pix = make([]byte, 4*m.w*m.h)
		m.img = ensureImage(m.img, m.w, m.h)
		m.reset()
	}
	switch {
	case in.Pressed(input.Menu):
		ctx.Pop()
		return
	case in.Pressed(input.Cancel):
		if m.zoom == 0 {
			ctx.Pop()
			return
		}
		m.zoom--
		m.reset()
	case in.Pressed(input.Confirm) && m.zoom < len(mapZooms)-1:
		m.zoom++
		m.reset()
	}
	if p, ok := in.Tap(); ok && m.zoom < len(mapZooms)-1 {
		m.cx, m.cy = m.worldAt(p.X, p.Y)
		m.zoom++
		m.reset()
	}
	if d, ok := in.Dir(); ok && in.DirPressed() {
		dx, dy := d.Delta()
		step := int64(m.w/4) * m.scale()
		if dy != 0 {
			step = int64(m.h/4) * m.scale()
		}
		m.cx, m.cy = geom.Mod(m.cx+int64(dx)*step, m.e.w.W), geom.Mod(m.cy+int64(dy)*step, m.e.w.H)
		m.reset()
	}
	m.render()
	if !m.marked {
		m.mark()
	}
}

// render draws terrain rows until the frame's budget is spent.
func (m *WorldMap) render() {
	if m.row >= m.h {
		return
	}
	t0 := time.Now()
	for m.row < m.h && time.Since(t0) < mapBudget {
		for px := 0; px < m.w; px++ {
			c := mapUnknown
			x, y := m.worldAt(px, m.row)
			if t, ok := m.e.w.Preview(x, y); ok {
				switch t {
				case world.Deep:
					c = mapDeep
				case world.Shallow:
					c = mapShallow
				default:
					c = mapLand
				}
			}
			i := 4 * (m.row*m.w + px)
			m.pix[i], m.pix[i+1], m.pix[i+2], m.pix[i+3] = c.R, c.G, c.B, 255
		}
		m.row++
	}
	// Rows not drawn yet stay dark.
	for i := 4 * m.row * m.w; i < len(m.pix); i += 4 {
		m.pix[i], m.pix[i+1], m.pix[i+2], m.pix[i+3] = mapUnknown.R, mapUnknown.G, mapUnknown.B, 255
	}
	m.dirty = true
}

// mark finds the settlements in view.
func (m *WorldMap) mark() {
	m.marked = true
	m.marks = m.marks[:0]
	if m.e.infra == nil {
		return
	}
	z := m.scale()
	vw, vh := int64(m.w)*z, int64(m.h)*z
	x0, y0 := m.worldAt(0, 0)
	if vw >= m.e.w.W {
		x0, vw = 0, m.e.w.W
	}
	if vh >= m.e.w.H {
		y0, vh = 0, m.e.w.H
	}
	lowest := mapTiers[m.zoom]
	if m.zoom > 0 {
		lowest = infra.Village // the places visited show at every zoom but the widest
	}
	for _, s := range m.e.infra.InRect(x0, y0, vw, vh, lowest) {
		px, py := m.pixelOf(s.X, s.Y)
		if px < 0 || py < 0 || px >= m.w || py >= m.h {
			continue
		}
		_, visited := m.e.visited[s.ID]
		if !visited && s.ID.Tier < mapTiers[m.zoom] {
			continue
		}
		m.marks = append(m.marks, mapMark{s: s, x: px, y: py, visited: visited})
	}
}

// Draw implements Scene.
func (m *WorldMap) Draw(dst *ebiten.Image) {
	if m.img == nil {
		return
	}
	if m.dirty {
		m.img.WritePixels(m.pix)
		m.dirty = false
	}
	dst.DrawImage(m.img, nil)
	// Settlements: bigger for bigger places; visited ones ringed.
	var labels []image.Rectangle
	for _, mk := range m.marks {
		r := map[infra.Tier]int{infra.Town: 1, infra.City: 1, infra.Metropolis: 2}[mk.s.ID.Tier]
		dot := image.Rect(mk.x-r-1, mk.y-r-1, mk.x+r+2, mk.y+r+2)
		if mk.visited {
			gfx.FillRect(dst, dot.Inset(-1), gfx.ColorAccent)
		}
		gfx.FillRect(dst, dot, gfx.ColorInk)
		gfx.FillRect(dst, dot.Inset(1), gfx.ColorPaper)
	}
	for _, mk := range m.marks {
		// Names by zoom: metropolises always, cities from the second zoom,
		// towns from the third, villages close up; places visited always.
		if !mk.visited && int(mk.s.ID.Tier) < int(mapTiers[0])-m.zoom {
			continue
		}
		name := m.e.infra.Name(mk.s)
		w := gfx.TextWidth(name)
		r := image.Rect(mk.x-w/2-1, mk.y+4, mk.x+w/2+2, mk.y+4+gfx.LineHeight)
		if r.Min.X < 0 || r.Max.X > m.w || r.Max.Y > m.h-gfx.LineHeight-2 || overlaps(labels, r) {
			continue
		}
		labels = append(labels, r)
		gfx.FillRect(dst, r, color.RGBA{0xf4, 0xe9, 0xd2, 0xc0})
		col := gfx.ColorInk
		if mk.visited {
			col = gfx.ColorAccent
		}
		gfx.Text(dst, name, r.Min.X+1, r.Min.Y-1, col)
	}
	// The party, with a blinking ring.
	p := m.e.partyOnMain()
	px, py := m.pixelOf(p.X, p.Y)
	if (ebiten.Tick()/20)%2 == 0 {
		gfx.FillRect(dst, image.Rect(px-3, py-3, px+4, py+4), gfx.ColorPaper)
	}
	gfx.FillRect(dst, image.Rect(px-2, py-2, px+3, py+3), gfx.ColorInk)
	gfx.FillRect(dst, image.Rect(px-1, py-1, px+2, py+2), gfx.ColorAccent)
	// The scale and the keys.
	bar := image.Rect(0, m.h-gfx.LineHeight-2, m.w, m.h)
	gfx.FillRect(dst, bar, color.RGBA{0x1b, 0x1b, 0x28, 0xd0})
	gfx.Text(dst, fmt.Sprintf("1 px = %s  Enter + Esc -  Tab close", mapDistance(m.scale())),
		3, bar.Min.Y, gfx.ColorPaper)
	if m.row < m.h {
		gfx.Text(dst, "...", m.w-24, 2, gfx.ColorPaper)
	}
}

func overlaps(rs []image.Rectangle, r image.Rectangle) bool {
	for _, o := range rs {
		if o.Overlaps(r) {
			return true
		}
	}
	return false
}

// mapDistance writes a length in tiles (metres) for the scale bar.
func mapDistance(tiles int64) string {
	if tiles >= 1000 {
		return fmt.Sprintf("%.1f km", float64(tiles)/1000)
	}
	return fmt.Sprintf("%d m", tiles)
}
