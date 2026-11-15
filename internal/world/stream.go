package world

import (
	"runtime"
	"sync"
	"time"

	"github.com/noxer/worlds/internal/geom"
)

// Streaming: main-map chunks around the party are built ahead of time, so
// moving fast never stalls a frame on chunk generation.
//
// The chunk cache belongs to the game thread. Builders only produce chunks
// (World.resolve is safe for concurrent use) and hand them back; Pump moves
// finished chunks into the cache. On platforms with threads a worker
// goroutine builds chunks. In the browser (WebAssembly is single-threaded) a
// goroutine would still stall the frame, so Pump builds queued chunks itself
// within a small time budget instead.

// Prefetch geometry, in chunks.
const (
	PrefetchRadius = 2 // around the anchor
	PrefetchAhead  = 3 // further in the direction of travel
)

// InlineBudget is how long Pump may spend building chunks per call when
// there is no worker (browser).
const InlineBudget = 3 * time.Millisecond

// Stats counts chunk builds.
type Stats struct {
	SyncBuilt  int // built on demand because they were needed immediately
	Prefetched int // built ahead of time and delivered by Pump
}

// Stats returns build counters.
func (w *World) Stats() Stats { return w.stats }

type builtChunk struct {
	key [2]int64
	c   *Chunk
}

type streamer struct {
	mu       sync.Mutex
	cond     *sync.Cond
	queue    [][2]int64        // wanted, most urgent first
	inflight map[[2]int64]bool // taken by a worker, not yet delivered
	done     chan builtChunk   // workers -> Pump
	inline   bool              // no workers: Pump builds
	closed   bool
	last     prefetchRequest // avoids rebuilding the queue every tick
}

type prefetchRequest struct {
	cx, cy int64
	dir    Dir
	moving bool
}

// StartStreaming enables background chunk building with n worker
// goroutines. n = 0 builds in Pump instead (for single-threaded platforms).
// Without StartStreaming, chunks are only built on demand.
func (w *World) StartStreaming(n int) {
	if w.stream != nil {
		return
	}
	s := &streamer{inflight: map[[2]int64]bool{}, done: make(chan builtChunk, 64), inline: n <= 0}
	s.cond = sync.NewCond(&s.mu)
	s.last.cx = -1
	w.stream = s
	for i := 0; i < n; i++ {
		go w.work()
	}
}

// DefaultWorkers is the number of streaming workers suited to the platform:
// none in the browser, one elsewhere (generation is fast; one worker keeps
// the game thread's CPU free).
func DefaultWorkers() int {
	if runtime.GOARCH == "wasm" {
		return 0
	}
	return 1
}

// StopStreaming ends the workers. Chunks are built on demand afterwards.
func (w *World) StopStreaming() {
	s := w.stream
	if s == nil {
		return
	}
	s.mu.Lock()
	s.closed = true
	s.cond.Broadcast()
	s.mu.Unlock()
	w.stream = nil
}

func (w *World) work() {
	s := w.stream
	for {
		s.mu.Lock()
		for len(s.queue) == 0 && !s.closed {
			s.cond.Wait()
		}
		if s.closed {
			s.mu.Unlock()
			return
		}
		key := s.queue[0]
		s.queue = s.queue[1:]
		s.inflight[key] = true
		s.mu.Unlock()

		s.done <- builtChunk{key, w.resolve(key[0], key[1])}
	}
}

// Prefetch asks for the chunks around a main-map anchor (usually the party
// leader), plus some ahead of it when moving. It replaces earlier requests.
// Call it from the game thread, e.g. every tick; it only does work when the
// anchor's chunk or direction changed.
func (w *World) Prefetch(anchor Pos, dir Dir, moving bool) {
	s := w.stream
	if s == nil || !anchor.Map.IsMain() {
		return
	}
	cw, ch := w.W/ChunkSize, w.H/ChunkSize
	req := prefetchRequest{geom.FloorDiv(anchor.X, ChunkSize), geom.FloorDiv(anchor.Y, ChunkSize), dir, moving}
	if req == s.last {
		return
	}
	s.last = req
	if p, ok := w.gen.(Prefetcher); ok {
		p.Prefetch(anchor.X, anchor.Y)
	}

	type want struct {
		key  [2]int64
		prio int64
	}
	var wants []want
	seen := map[[2]int64]bool{}
	dx, dy := dir.Delta()
	if !moving {
		dx, dy = 0, 0
	}
	// Priority: distance from a point one chunk ahead of the anchor, so the
	// chunks in the direction of travel come first.
	fx, fy := req.cx+dx, req.cy+dy
	add := func(cx, cy int64) {
		key := [2]int64{geom.Mod(cx, cw), geom.Mod(cy, ch)}
		if seen[key] || w.cache.has(key) {
			return
		}
		seen[key] = true
		wants = append(wants, want{key, max(geom.Abs(cx-fx), geom.Abs(cy-fy))})
	}
	for y := -int64(PrefetchRadius); y <= PrefetchRadius; y++ {
		for x := -int64(PrefetchRadius); x <= PrefetchRadius; x++ {
			add(req.cx+x, req.cy+y)
		}
	}
	if moving {
		for k := int64(PrefetchRadius + 1); k <= PrefetchRadius+PrefetchAhead; k++ {
			for side := int64(-1); side <= 1; side++ {
				// side steps perpendicular to the direction of travel
				add(req.cx+dx*k+dy*side, req.cy+dy*k+dx*side)
			}
		}
	}
	// Stable insertion sort by priority (few entries).
	for i := 1; i < len(wants); i++ {
		for j := i; j > 0 && wants[j].prio < wants[j-1].prio; j-- {
			wants[j], wants[j-1] = wants[j-1], wants[j]
		}
	}
	s.mu.Lock()
	s.queue = s.queue[:0]
	for _, wt := range wants {
		if !s.inflight[wt.key] {
			s.queue = append(s.queue, wt.key)
		}
	}
	s.cond.Broadcast()
	s.mu.Unlock()
}

// Pump moves finished chunks into the cache. Without workers it builds
// queued chunks itself, for at most InlineBudget. Call it once per tick from
// the game thread.
func (w *World) Pump() {
	s := w.stream
	if s == nil {
		return
	}
	if s.inline {
		start := time.Now()
		for time.Since(start) < InlineBudget {
			s.mu.Lock()
			if len(s.queue) == 0 {
				s.mu.Unlock()
				break
			}
			key := s.queue[0]
			s.queue = s.queue[1:]
			s.mu.Unlock()
			if !w.cache.has(key) {
				w.deliver(builtChunk{key, w.resolve(key[0], key[1])})
			}
		}
		return
	}
	for {
		select {
		case b := <-s.done:
			s.mu.Lock()
			delete(s.inflight, b.key)
			s.mu.Unlock()
			w.deliver(b)
		default:
			return
		}
	}
}

func (w *World) deliver(b builtChunk) {
	if w.cache.has(b.key) {
		return // built on demand meanwhile; identical anyway
	}
	w.cache.put(b.key, b.c)
	w.stats.Prefetched++
}

// Pending reports how many prefetch requests are queued or being built.
func (w *World) Pending() int {
	s := w.stream
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue) + len(s.inflight)
}

// IsCached reports whether the main-map chunk containing p is in memory.
func (w *World) IsCached(p Pos) bool {
	p, _ = w.Normalize(p)
	return w.cache.has([2]int64{geom.FloorDiv(p.X, ChunkSize), geom.FloorDiv(p.Y, ChunkSize)})
}
