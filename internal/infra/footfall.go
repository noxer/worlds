package infra

// Footfall: where ambient pedestrians walk, and how busy it is there. They
// keep to sidewalks, plazas and park paths in towns and cities, and to the
// square, lanes, paths and footpaths in villages; they cross roads only at
// crosswalks.

// Footfall says how busy tile (x, y) is: 0 where pedestrians don't go
// (roads, yards, the countryside), else 1 (quiet) to 4 (busy). walk is
// true where pedestrians may walk, which includes crosswalks (busy 0: they
// cross there, but nobody turns up there). urban tells city people from
// villagers.
func (in *Infra) Footfall(x, y int64) (busy int, walk, urban bool) {
	for _, c := range in.CitiesIn(x, y, 1, 1) {
		if b, w := c.footfall(x, y); w {
			return b, true, true
		}
	}
	for _, s := range in.SitesIn(x, y, 1, 1) {
		dx, dy := in.delta(s.X0, s.Y0, x, y)
		switch s.At(dx, dy) {
		case Square:
			return 2, true, false
		case Lane, Foot, Street:
			return 1, true, false
		}
	}
	return 0, false, false
}

func (c *CityPlan) footfall(x, y int64) (busy int, walk bool) {
	t := c.At(x, y)
	_, _, u, v := c.grid(x, y)
	switch t.Kind {
	case CCrossH, CCrossV:
		return 0, true
	case CRoad:
		// Minor streets (inside a superblock) can be crossed anywhere,
		// arterials only at crosswalks.
		return 0, u >= arterial && v >= arterial
	case CWalk, CPaved:
		if t.Deco != DNone {
			return 0, false // a tree, bench or lamp
		}
	default:
		return 0, false
	}
	i, j, _, _ := c.grid(x, y)
	sb := c.sb(i, j)
	switch {
	case sb.style == sbStation || sb.style == sbSquare || sb.style == sbDowntown || sb.district == dOld:
		return 4, true
	case sb.style == sbResidential || sb.style == sbPark:
		return 2, true
	}
	return 1, true // suburbs, industrial estates
}
