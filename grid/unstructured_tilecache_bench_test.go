package grid

import (
	"math"
	"math/rand"
	"sync"
	"testing"
)

// buildSyntheticMesh returns a (lats, lons) pair drawn from a uniform
// random distribution over the sphere — quasi-icosahedral enough for
// benchmark purposes (a real ICON-Global mesh has ~2.95M cells; we use
// a smaller N so the test stays fast).
func buildSyntheticMesh(n int) (lats, lons []float64) {
	rng := rand.New(rand.NewSource(int64(n)))
	lats = make([]float64, n)
	lons = make([]float64, n)
	for i := 0; i < n; i++ {
		// Distribute uniformly on the sphere by inverting cumulative area.
		s := 2*rng.Float64() - 1
		lats[i] = 180 / math.Pi * math.Asin(s)
		lons[i] = 360*rng.Float64() - 180
	}
	return
}

// BenchmarkUnstructuredLocateTileIndicesCold exercises the very first
// call against a fresh cache — every pixel goes through KD-tree NN.
func BenchmarkUnstructuredLocateTileIndicesCold(b *testing.B) {
	const N = 200_000
	lats, lons := buildSyntheticMesh(N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g := &Unstructured{NumPoints_: N, EarthRadiusMeters: earthRadiusMeters}
		_ = g.SetCoordinates(lats, lons)
		if _, err := g.LocateTileIndices(5, 12, 10, 256, 256); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUnstructuredLocateTileIndicesWarm hits the cached entry on
// every iteration — measures the slice-pointer return path that the
// rendering fast path takes for every tile after the first.
func BenchmarkUnstructuredLocateTileIndicesWarm(b *testing.B) {
	const N = 200_000
	lats, lons := buildSyntheticMesh(N)
	g := &Unstructured{NumPoints_: N, EarthRadiusMeters: earthRadiusMeters}
	_ = g.SetCoordinates(lats, lons)
	if _, err := g.LocateTileIndices(5, 12, 10, 256, 256); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := g.LocateTileIndices(5, 12, 10, 256, 256); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkUnstructuredLocateBaseline measures the per-pixel Locate
// path across a 256x256 tile — what the renderer paid before the
// cache. This is the apples-to-apples reference for the cold
// LocateTileIndices benchmark.
func BenchmarkUnstructuredLocateBaseline(b *testing.B) {
	const N = 200_000
	lats, lons := buildSyntheticMesh(N)
	g := &Unstructured{NumPoints_: N, EarthRadiusMeters: earthRadiusMeters}
	_ = g.SetCoordinates(lats, lons)
	g.Locate(0, 0) // prime the tree
	const W, H = 256, 256
	tileLats, tileLons := buildTileLatLons(5, 12, 10, W, H)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for j := 0; j < H; j++ {
			lat := tileLats[j]
			for i := 0; i < W; i++ {
				g.Locate(lat, tileLons[i])
			}
		}
	}
}

// BenchmarkUnstructuredLocateTileIndicesParallel hammers a warm cache
// from many goroutines simultaneously. With sync.Map + sync.Once on
// each entry, contention stays near zero.
func BenchmarkUnstructuredLocateTileIndicesParallel(b *testing.B) {
	const N = 200_000
	lats, lons := buildSyntheticMesh(N)
	g := &Unstructured{NumPoints_: N, EarthRadiusMeters: earthRadiusMeters}
	_ = g.SetCoordinates(lats, lons)
	if _, err := g.LocateTileIndices(5, 12, 10, 256, 256); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	var wg sync.WaitGroup
	const G = 32
	per := b.N / G
	if per <= 0 {
		per = 1
	}
	for k := 0; k < G; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				if _, err := g.LocateTileIndices(5, 12, 10, 256, 256); err != nil {
					b.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// buildTileLatLons mirrors tile.Build's per-pixel lat/lon math. Inlined
// here to avoid a package-cycle import (tile depends on grid for the
// renderer glue, and benchmarks live in the grid package).
func buildTileLatLons(z, x, y, w, h int) (lats, lons []float64) {
	n := float64(int(1) << uint(z))
	lons = make([]float64, w)
	for i := 0; i < w; i++ {
		lons[i] = (float64(x)+(float64(i)+0.5)/float64(w))/n*360 - 180
	}
	lats = make([]float64, h)
	for j := 0; j < h; j++ {
		yn := float64(y) + (float64(j)+0.5)/float64(h)
		t := math.Pi - 2*math.Pi*(yn/n)
		lats[j] = 180 / math.Pi * math.Atan(0.5*(math.Exp(t)-math.Exp(-t)))
	}
	return
}
