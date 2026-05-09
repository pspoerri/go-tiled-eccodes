package grid

import (
	"math"
	"math/rand"
	"testing"
)

// buildRegionalMesh returns N cells uniformly distributed over a small
// lat/lon box — meant to mimic an iconch1/icond2 limited-area
// icosahedral mesh: ~1.1 km native pitch over Switzerland-sized
// territory. The exact distribution does not matter for the benchmark,
// only that the mesh covers a small fraction of the planet so most
// pixels of a low-zoom tile fall outside its footprint.
func buildRegionalMesh(n int, latLo, latHi, lonLo, lonHi float64) (lats, lons []float64) {
	rng := rand.New(rand.NewSource(int64(n)))
	lats = make([]float64, n)
	lons = make([]float64, n)
	for i := 0; i < n; i++ {
		lats[i] = latLo + rng.Float64()*(latHi-latLo)
		lons[i] = lonLo + rng.Float64()*(lonHi-lonLo)
	}
	return
}

// BenchmarkUnstructuredTileSetRegionalCH exercises the iconch1-shaped
// workload: a regional mesh confined to Switzerland (≈5.5–10.5°E,
// 45.5–47.8°N), rendered through a 4×4 set of distinct z=5 tiles
// centred on the mesh. ~5 % of each tile's pixels are inside the
// mesh's convex hull; the other ~95 % must terminate the ring scan
// via the MaxChordSquared cap.
//
// This is the workload the production logs hit (status:0, dur_ms ≈
// 498 000 on /iconch1/tiles/data/t_2m/.../5/.../...pb). Before the
// "drop bestIdx<0 from early-terminate" fix the ring scan expanded
// to maxRing = nLat ≈ √N on every out-of-footprint pixel; after,
// the scan stops as soon as chord² ≥ MaxChordSquared.
//
// We use a 100 k-cell mesh (rather than iconch1's actual ~1.1 M)
// because pre-fix the cost is quadratic in nLat ≈ √N and the bench
// would take many minutes per iter — the fix's effect is identical
// in shape, just smaller in absolute numbers, on the smaller mesh.
//
// MaxNNDistanceMeters mirrors iconch1's gribstore index setting (5 km).
func BenchmarkUnstructuredTileSetRegionalCH(b *testing.B) {
	const (
		N     = 100_000
		latLo = 45.5
		latHi = 47.8
		lonLo = 5.5
		lonHi = 10.5
		Z     = 5
		W, H  = 256, 256
		Side  = 4
		// 4×4 region centred so the mesh sits inside the bottom-right
		// few tiles. At z=5 each tile is ~11.25° wide; tiles
		// (16,11)..(19,14) span roughly 0–45°E, 30–55°N — covers
		// Switzerland with most pixels out-of-footprint.
		baseX = 16
		baseY = 11
	)
	lats, lons := buildRegionalMesh(N, latLo, latHi, lonLo, lonHi)
	g := &Unstructured{NumPoints_: N, EarthRadiusMeters: earthRadiusMeters}
	_ = g.SetCoordinates(lats, lons)
	g.SetMaxDistance(5_000) // iconch1's footprint cap
	g.Locate(46.0, 8.0)     // prime the hash
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		// Force every iter to recompute every tile's cell-index map —
		// production rotates Readers (and tile-locate caches) at each
		// run rotation, so cold tile sweeps are the cost we care about.
		g.tileIdx = nil
		for ty := 0; ty < Side; ty++ {
			for tx := 0; tx < Side; tx++ {
				if _, err := g.LocateTileIndices(Z, baseX+tx, baseY+ty, W, H); err != nil {
					b.Fatal(err)
				}
			}
		}
	}
}

// BenchmarkUnstructuredTileSetRegionalDE mirrors the icond2 shape: a
// ~600 k-cell mesh over Germany + neighbours (~5–16°E, 47–56°N) with a
// 5 km footprint cap. icond2 hits the same out-of-footprint
// termination path as iconch1; this confirms the fix carries across
// the wider regional models too.
func BenchmarkUnstructuredTileSetRegionalDE(b *testing.B) {
	const (
		N     = 80_000
		latLo = 47.0
		latHi = 56.0
		lonLo = 5.0
		lonHi = 16.0
		Z     = 5
		W, H  = 256, 256
		Side  = 4
		baseX = 16
		baseY = 10
	)
	lats, lons := buildRegionalMesh(N, latLo, latHi, lonLo, lonHi)
	g := &Unstructured{NumPoints_: N, EarthRadiusMeters: earthRadiusMeters}
	_ = g.SetCoordinates(lats, lons)
	g.SetMaxDistance(5_000)
	g.Locate(51.0, 10.0)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		g.tileIdx = nil
		for ty := 0; ty < Side; ty++ {
			for tx := 0; tx < Side; tx++ {
				if _, err := g.LocateTileIndices(Z, baseX+tx, baseY+ty, W, H); err != nil {
					b.Fatal(err)
				}
			}
		}
	}
}

// BenchmarkUnstructuredTileSetRegionalCHFar renders a 4×4 z=5 tile set
// far from the Switzerland mesh — every pixel is out-of-footprint, so
// 100 % of the time goes through the MaxChordSquared termination
// path. This is the upper bound on the bug's blast radius (and the
// upper bound on the fix's payoff).
func BenchmarkUnstructuredTileSetRegionalCHFar(b *testing.B) {
	const (
		N     = 100_000
		latLo = 45.5
		latHi = 47.8
		lonLo = 5.5
		lonHi = 10.5
		Z     = 5
		W, H  = 256, 256
		Side  = 4
		// Tiles over the Pacific — totally outside the mesh.
		baseX = 1
		baseY = 12
	)
	lats, lons := buildRegionalMesh(N, latLo, latHi, lonLo, lonHi)
	g := &Unstructured{NumPoints_: N, EarthRadiusMeters: earthRadiusMeters}
	_ = g.SetCoordinates(lats, lons)
	g.SetMaxDistance(5_000)
	g.Locate(46.0, 8.0)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		g.tileIdx = nil
		for ty := 0; ty < Side; ty++ {
			for tx := 0; tx < Side; tx++ {
				if _, err := g.LocateTileIndices(Z, baseX+tx, baseY+ty, W, H); err != nil {
					b.Fatal(err)
				}
			}
		}
	}
}

// BenchmarkUnstructuredAutoLadderTileSet emulates the wetter-api
// composite ("auto") ladder for a single tile request: every
// contributor's mesh has its own LocateTileIndices sweep, and the
// composite blender walks them all before producing the output
// pixel. Production observed z=5 auto chunk requests landing at
// 60–100 s post-fix; the inputs to those chunks are this set of
// per-contributor LocateTileIndices calls. We render every mesh
// against the same 4×4 z=5 tile set so the bench captures the
// per-tile cost the blender pays N_contributors times over.
//
// Mesh shapes mirror the four physical models in the auto ladder:
//
//	iconch1     R19B08    Switzerland     5 km cap   100k cells
//	icond2      R19B07    Germany+Alps    5 km cap    80k cells
//	iconeueps   R03B07    Europe         40 km cap   100k cells
//	icondglobal R03B07    Global         30 km cap   200k cells
//
// (Cell counts are scaled down from production by the same ratio
// across all four so the bench finishes in seconds even on the
// pre-fix code; the per-pixel cost shape is unchanged.)
func BenchmarkUnstructuredAutoLadderTileSet(b *testing.B) {
	type rung struct {
		name    string
		n       int
		latLo, latHi, lonLo, lonHi float64
		maxNN   float64 // metres
	}
	rungs := []rung{
		{"iconch1", 100_000, 45.5, 47.8, 5.5, 10.5, 5_000},
		{"icond2", 80_000, 47.0, 56.0, 5.0, 16.0, 5_000},
		{"iconeueps", 100_000, 30.0, 73.0, -25.0, 65.0, 40_000},
		{"icondglobal", 200_000, -90.0, 90.0, -180.0, 180.0, 30_000},
	}
	meshes := make([]*Unstructured, len(rungs))
	for i, r := range rungs {
		lats, lons := buildRegionalMesh(r.n, r.latLo, r.latHi, r.lonLo, r.lonHi)
		g := &Unstructured{NumPoints_: r.n, EarthRadiusMeters: earthRadiusMeters}
		_ = g.SetCoordinates(lats, lons)
		g.SetMaxDistance(r.maxNN)
		// Prime every mesh's hash + a tile index so we measure the
		// per-tile sweep cost, not the one-time hash build.
		g.Locate((r.latLo+r.latHi)/2, (r.lonLo+r.lonHi)/2)
		meshes[i] = g
	}
	const (
		Z     = 5
		W, H  = 256, 256
		Side  = 4
		baseX = 16
		baseY = 11
	)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, g := range meshes {
			g.tileIdx = nil
			for ty := 0; ty < Side; ty++ {
				for tx := 0; tx < Side; tx++ {
					if _, err := g.LocateTileIndices(Z, baseX+tx, baseY+ty, W, H); err != nil {
						b.Fatal(err)
					}
				}
			}
		}
	}
}

// guardAgainstUnusedMath silences the import; the constants section uses
// math via earthRadiusMeters which lives in unstructured.go.
var _ = math.Pi
