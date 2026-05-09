package grid

import (
	"math"
	"testing"
)

// BenchmarkUnstructuredLocateBruteForce is the apples-to-apples reference
// for the hash: linear scan over all cells, computing chord-squared
// distance to each, on the same 256x256 tile geometry as the cold/warm
// hash benchmarks. Quadratic in mesh size — included so we can quote a
// concrete speed-up factor over the naive lookup.
func BenchmarkUnstructuredLocateBruteForce(b *testing.B) {
	const N = 200_000
	lats, lons := buildSyntheticMesh(N)
	xs := make([]float64, N)
	ys := make([]float64, N)
	zs := make([]float64, N)
	for i := 0; i < N; i++ {
		xs[i], ys[i], zs[i] = latLonToVec3(lats[i], lons[i])
	}
	const W, H = 256, 256
	tileLats, tileLons := buildTileLatLons(5, 12, 10, W, H)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for j := 0; j < H; j++ {
			for i := 0; i < W; i++ {
				qx, qy, qz := latLonToVec3(tileLats[j], tileLons[i])
				best := -1
				bestD2 := math.Inf(1)
				for c := 0; c < N; c++ {
					dx := xs[c] - qx
					dy := ys[c] - qy
					dz := zs[c] - qz
					d2 := dx*dx + dy*dy + dz*dz
					if d2 < bestD2 {
						bestD2 = d2
						best = c
					}
				}
				_ = best
			}
		}
	}
}

// BenchmarkUnstructuredHashCold builds the hash + sweeps a 256x256 tile.
// Mirrors the structure of LocateTileIndicesCold but without the per-call
// sync.Map / sync.Once overhead, so the build-vs-query split is visible.
func BenchmarkUnstructuredHashCold(b *testing.B) {
	const N = 200_000
	lats, lons := buildSyntheticMesh(N)
	const W, H = 256, 256
	tileLats, tileLons := buildTileLatLons(5, 12, 10, W, H)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		h := newUniformHash(lats, lons)
		for j := 0; j < H; j++ {
			lat := tileLats[j]
			for i := 0; i < W; i++ {
				h.nearest(lat, tileLons[i], 0)
			}
		}
	}
}

// BenchmarkUnstructuredHashBuild isolates the index-construction cost so
// we can subtract it from the cold tile bench and read off the per-pixel
// query time.
func BenchmarkUnstructuredHashBuild(b *testing.B) {
	const N = 200_000
	lats, lons := buildSyntheticMesh(N)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = newUniformHash(lats, lons)
	}
}

// BenchmarkUnstructuredHashQuery measures per-pixel query cost on a
// pre-built hash. Numerator is one 256x256 sweep so the divide is by
// 65 536 to get ns/pixel.
func BenchmarkUnstructuredHashQuery(b *testing.B) {
	const N = 200_000
	lats, lons := buildSyntheticMesh(N)
	h := newUniformHash(lats, lons)
	const W, H = 256, 256
	tileLats, tileLons := buildTileLatLons(5, 12, 10, W, H)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for j := 0; j < H; j++ {
			lat := tileLats[j]
			for i := 0; i < W; i++ {
				h.nearest(lat, tileLons[i], 0)
			}
		}
	}
}

// BenchmarkUnstructuredTileSetBatched runs a 4×4 region of distinct
// 256×256 tiles at zoom 5 against a single shared hash — the realistic
// tile-server cold-pass shape (one mesh, many tiles, no warm cache).
// Reports ns/op for the whole 16-tile = 1 048 576-pixel set.
func BenchmarkUnstructuredTileSetBatched(b *testing.B) {
	const N = 200_000
	lats, lons := buildSyntheticMesh(N)
	g := &Unstructured{NumPoints_: N, EarthRadiusMeters: earthRadiusMeters}
	_ = g.SetCoordinates(lats, lons)
	g.Locate(0, 0) // prime the hash so we measure per-tile cost only
	const Z = 5
	const W, H = 256, 256
	const Side = 4
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		// Bypass the sync.Map cache by clearing it each iter — every
		// tile takes the cold-compute path.
		g.tileIdx = nil
		for ty := 0; ty < Side; ty++ {
			for tx := 0; tx < Side; tx++ {
				if _, err := g.LocateTileIndices(Z, 12+tx, 10+ty, W, H); err != nil {
					b.Fatal(err)
				}
			}
		}
	}
}

// BenchmarkUnstructuredTileSetPerPixel is the apples-to-apples reference:
// the same 16-tile set, but routed through the un-batched per-pixel
// hash.nearest path. The delta vs the Batched bench is the spatial-batch
// win for a tile-set workload.
func BenchmarkUnstructuredTileSetPerPixel(b *testing.B) {
	const N = 200_000
	lats, lons := buildSyntheticMesh(N)
	h := newUniformHash(lats, lons)
	const Z = 5
	const W, H = 256, 256
	const Side = 4
	// Pre-compute lats/lons for each (tx, ty) in the 4×4 region so the
	// loop measures the lookup, not the projection arithmetic.
	type tileGeom struct{ lats, lons []float64 }
	tiles := make([]tileGeom, 0, Side*Side)
	for ty := 0; ty < Side; ty++ {
		for tx := 0; tx < Side; tx++ {
			tlats, tlons := buildTileLatLons(Z, 12+tx, 10+ty, W, H)
			tiles = append(tiles, tileGeom{lats: tlats, lons: tlons})
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, t := range tiles {
			for j := 0; j < H; j++ {
				lat := t.lats[j]
				for i := 0; i < W; i++ {
					h.nearest(lat, t.lons[i], 0)
				}
			}
		}
	}
}
