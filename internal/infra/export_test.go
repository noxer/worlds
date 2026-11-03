package infra

// Exports for the external tests.

func Member(n Network, s Settlement) bool    { return member(n, s) }
func (in *Infra) Dist(a, b Settlement) int64 { return isqrt(in.dist2(a, b)) }
func Gap(t Tier) int64                       { return gap(t) }
func (n Network) Grid() int64                { return networks[n].grid }

func (in *Infra) BlockReady(x, y int64) bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	_, ok := in.blocks[[2]int64{x / BlockSize, y / BlockSize}]
	return ok
}

func (c *CityPlan) District(i, j int64) uint8        { return c.district(i, j) }
func (c *CityPlan) RadiusTowards(dx, dy int64) int64 { return c.radiusTowards(dx, dy) }
