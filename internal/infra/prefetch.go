package infra

import (
	"runtime"
	"time"

	"github.com/noxer/worlds/internal/geom"
)

// Computing the ways of a block can take a few hundred milliseconds (all
// routes that cross it). Prefetch does that ahead of time, for the blocks
// around the party, on a background goroutine; chunk generation then finds
// them cached. A block that isn't ready yet is still computed on demand.

// Prefetch asks for the ways of the blocks around tile (x, y), starting
// with the block of (x, y) itself. It returns at once.
func (in *Infra) Prefetch(x, y int64) {
	nbx, nby := in.w/BlockSize, in.h/BlockSize
	bx, by := geom.FloorDiv(x, BlockSize), geom.FloorDiv(y, BlockSize)
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.prefetchAt == [2]int64{bx, by} && in.prefetchStarted {
		return
	}
	in.prefetchAt, in.prefetchStarted = [2]int64{bx, by}, true
	var keys [][2]int64
	for _, d := range [9][2]int64{{0, 0}, {-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
		k := [2]int64{geom.Mod(bx+d[0], nbx), geom.Mod(by+d[1], nby)}
		if _, ok := in.blocks[k]; !ok {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return
	}
	if in.prefetchQueue == nil {
		in.prefetchQueue = make(chan [2]int64, 64)
		go in.prefetchLoop()
	}
	for _, k := range keys {
		select {
		case in.prefetchQueue <- k:
		default: // the queue is full; the block is computed on demand
		}
	}
}

func (in *Infra) prefetchLoop() {
	y := &yielder{}
	for k := range in.prefetchQueue {
		in.mu.Lock()
		_, done := in.blocks[k]
		in.mu.Unlock()
		if !done {
			in.blockWith(k[0], k[1], y)
			in.SitesIn(k[0]*BlockSize, k[1]*BlockSize, BlockSize, BlockSize)
		}
	}
}

// yielder lets a long computation in the background give way to the game.
// In the browser, Go runs on one thread and only returns to the browser
// (which delivers frames) when every goroutine is blocked, so the
// computation sleeps briefly every few milliseconds. Elsewhere the
// background goroutine has a core of its own.
type yielder struct{ last time.Time }

const yieldEvery = 2 * time.Millisecond

func (y *yielder) maybeYield() {
	if y == nil || runtime.GOARCH != "wasm" {
		return
	}
	if time.Since(y.last) >= yieldEvery {
		time.Sleep(time.Millisecond)
		y.last = time.Now()
	}
}
