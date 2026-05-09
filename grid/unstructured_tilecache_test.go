package grid

import (
	"math"
	"math/rand"
	"sync"
	"testing"
)

// TestLocateTileIndicesMatchesPerPixelLocate compares the fast-path
// LocateTileIndices output against per-pixel Locate. Out-of-domain
// pixels and cell-mask hits must agree on the -1 sentinel.
func TestLocateTileIndicesMatchesPerPixelLocate(t *testing.T) {
	const N = 5000
	rng := rand.New(rand.NewSource(7))
	lats := make([]float64, N)
	lons := make([]float64, N)
	// Cluster around a mid-latitude band so a tile at z=4 has hits.
	for i := 0; i < N; i++ {
		lats[i] = 30 + rng.Float64()*40
		lons[i] = -10 + rng.Float64()*40
	}
	g := &Unstructured{NumPoints_: N, EarthRadiusMeters: earthRadiusMeters}
	if err := g.SetCoordinates(lats, lons); err != nil {
		t.Fatalf("SetCoordinates: %v", err)
	}
	g.SetMaxDistance(200_000) // 200 km cap so far-away pixels become -1

	const W, H = 64, 64
	const Z, X, Y = 4, 8, 5

	got, err := g.LocateTileIndices(Z, X, Y, W, H)
	if err != nil {
		t.Fatalf("LocateTileIndices: %v", err)
	}
	if len(got) != W*H {
		t.Fatalf("len(got) = %d, want %d", len(got), W*H)
	}

	// Brute reference: build the same lats/lons the implementation does
	// and call Locate per pixel.
	n := float64(int(1) << uint(Z))
	for j := 0; j < H; j++ {
		yn := float64(Y) + (float64(j)+0.5)/float64(H)
		tt := math.Pi - 2*math.Pi*(yn/n)
		lat := 180 / math.Pi * math.Atan(0.5*(math.Exp(tt)-math.Exp(-tt)))
		for i := 0; i < W; i++ {
			lon := (float64(X)+(float64(i)+0.5)/float64(W))/n*360 - 180
			fi, _, ok := g.Locate(lat, lon)
			want := int32(-1)
			if ok {
				want = int32(fi)
			}
			if got[j*W+i] != want {
				t.Fatalf("pixel (%d,%d) lat=%.3f lon=%.3f: got %d, want %d",
					i, j, lat, lon, got[j*W+i], want)
			}
		}
	}
}

// TestLocateTileIndicesCachedAcrossCalls confirms the cache returns
// the same slice instance on repeat calls — proving we skip recompute.
func TestLocateTileIndicesCachedAcrossCalls(t *testing.T) {
	const N = 200
	rng := rand.New(rand.NewSource(1))
	lats := make([]float64, N)
	lons := make([]float64, N)
	for i := 0; i < N; i++ {
		lats[i] = -45 + rng.Float64()*90
		lons[i] = -90 + rng.Float64()*180
	}
	g := &Unstructured{NumPoints_: N, EarthRadiusMeters: earthRadiusMeters}
	_ = g.SetCoordinates(lats, lons)

	a, err := g.LocateTileIndices(3, 2, 3, 32, 32)
	if err != nil {
		t.Fatalf("LocateTileIndices a: %v", err)
	}
	b, err := g.LocateTileIndices(3, 2, 3, 32, 32)
	if err != nil {
		t.Fatalf("LocateTileIndices b: %v", err)
	}
	if &a[0] != &b[0] {
		t.Errorf("cache miss on repeat: got distinct slices")
	}
}

// TestLocateTileIndicesSharedThroughSetCoordinatesFrom validates that
// every Unstructured sharing a mesh via SetCoordinatesFrom also shares
// the cell-index cache.
func TestLocateTileIndicesSharedThroughSetCoordinatesFrom(t *testing.T) {
	const N = 100
	lats := make([]float64, N)
	lons := make([]float64, N)
	for i := 0; i < N; i++ {
		lats[i] = -10 + float64(i)/float64(N)*20
		lons[i] = -10 + float64(i)/float64(N)*20
	}
	primary := &Unstructured{NumPoints_: N, EarthRadiusMeters: earthRadiusMeters}
	_ = primary.SetCoordinates(lats, lons)
	other := &Unstructured{NumPoints_: N, EarthRadiusMeters: earthRadiusMeters}
	if err := other.SetCoordinatesFrom(primary); err != nil {
		t.Fatalf("SetCoordinatesFrom: %v", err)
	}
	a, err := primary.LocateTileIndices(2, 1, 1, 16, 16)
	if err != nil {
		t.Fatalf("primary LocateTileIndices: %v", err)
	}
	b, err := other.LocateTileIndices(2, 1, 1, 16, 16)
	if err != nil {
		t.Fatalf("other LocateTileIndices: %v", err)
	}
	if &a[0] != &b[0] {
		t.Errorf("cache not shared across SetCoordinatesFrom")
	}
}

// TestLocateTileIndicesConcurrent runs many goroutines through the
// cache for the same key. Race detector + identity check confirms
// sync.Once guarantees a single compute.
func TestLocateTileIndicesConcurrent(t *testing.T) {
	const N = 500
	rng := rand.New(rand.NewSource(42))
	lats := make([]float64, N)
	lons := make([]float64, N)
	for i := 0; i < N; i++ {
		lats[i] = rng.Float64()*60 - 30
		lons[i] = rng.Float64()*60 - 30
	}
	g := &Unstructured{NumPoints_: N, EarthRadiusMeters: earthRadiusMeters}
	_ = g.SetCoordinates(lats, lons)

	const G = 32
	var wg sync.WaitGroup
	results := make([][]int32, G)
	for k := 0; k < G; k++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			indices, err := g.LocateTileIndices(3, 4, 4, 32, 32)
			if err != nil {
				t.Errorf("goroutine %d: %v", k, err)
				return
			}
			results[k] = indices
		}(k)
	}
	wg.Wait()
	for k := 1; k < G; k++ {
		if &results[k][0] != &results[0][0] {
			t.Errorf("goroutine %d got distinct slice (sync.Once leaked)", k)
		}
	}
}
