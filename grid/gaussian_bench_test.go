package grid

import (
	"sort"
	"testing"
)

// buildGaussianBench builds a regular F320 Gaussian grid (Nj=640) with the
// lat-bucket hash already populated by ParseGaussian.
func buildGaussianBench() Gaussian {
	body := make([]byte, 58)
	// Ni = 1280, Nj = 640.
	body[16] = 0
	body[17] = 0
	body[18] = 0x05
	body[19] = 0x00 // Ni = 1280
	body[20] = 0
	body[21] = 0
	body[22] = 0x02
	body[23] = 0x80 // Nj = 640
	// N (number of parallels per hemisphere) at byte 53: 320.
	body[53] = 0
	body[54] = 0
	body[55] = 0x01
	body[56] = 0x40 // N = 320
	return ParseGaussian(body, nil, 0)
}

// BenchmarkGaussianLocateHash measures Locate via the lat-bucket hash that
// ParseGaussian builds. F320 has 640 latitudes; the hash gives an O(1)
// jump-into the row range plus ≤1 refinement step.
func BenchmarkGaussianLocateHash(b *testing.B) {
	g := buildGaussianBench()
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

// BenchmarkGaussianLocateLinear is the naive reference: walk the
// descending Lats array until we land below the query. O(Nj) per query —
// the apples-to-apples baseline for the hashed path.
func BenchmarkGaussianLocateLinear(b *testing.B) {
	g := buildGaussianBench()
	const W, H = 256, 256
	tileLats, tileLons := buildTileLatLons(5, 12, 10, W, H)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for j := 0; j < H; j++ {
			lat := tileLats[j]
			for i := 0; i < W; i++ {
				locateLinear(g, lat, tileLons[i])
			}
		}
	}
}

// BenchmarkGaussianLocateBinary is sort.Search over a closure — the path
// the previous Locate took. Quoted alongside the hash to call out the
// closure-indirection penalty Go's binary search pays in this shape.
func BenchmarkGaussianLocateBinary(b *testing.B) {
	g := buildGaussianBench()
	const W, H = 256, 256
	tileLats, tileLons := buildTileLatLons(5, 12, 10, W, H)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for j := 0; j < H; j++ {
			lat := tileLats[j]
			for i := 0; i < W; i++ {
				locateBinary(g, lat, tileLons[i])
			}
		}
	}
}

// locateLinear and locateBinary mirror the full Locate path (lat-row
// search + within-row lon math) using the labelled row-search strategy.
// Apples-to-apples comparison for the hashed Locate.
// BenchmarkGaussianTileSetHash runs a 4×4 region of distinct 256×256
// tiles at zoom 5 against a single regular Gaussian grid. Same shape as
// the unstructured tile-set bench, so the two are directly comparable.
func BenchmarkGaussianTileSetHash(b *testing.B) {
	g := buildGaussianBench()
	const Z = 5
	const W, H = 256, 256
	const Side = 4
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
					g.Locate(lat, t.lons[i])
				}
			}
		}
	}
}

// BenchmarkGaussianTileSetLinear is the naive reference: same 16-tile
// region, but Locate walks Lats[] linearly instead of jumping into the
// hash bucket.
func BenchmarkGaussianTileSetLinear(b *testing.B) {
	g := buildGaussianBench()
	const Z = 5
	const W, H = 256, 256
	const Side = 4
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
					locateLinear(g, lat, t.lons[i])
				}
			}
		}
	}
}

func locateLinear(g Gaussian, lat, lon float64) (float64, float64, bool) {
	nj := len(g.Lats)
	if lat > g.Lats[0]+1e-9 || lat < g.Lats[nj-1]-1e-9 {
		return 0, 0, false
	}
	jUpper := 0
	for jUpper < nj && g.Lats[jUpper] > lat {
		jUpper++
	}
	return finishLocate(g, lat, lon, jUpper)
}

func locateBinary(g Gaussian, lat, lon float64) (float64, float64, bool) {
	nj := len(g.Lats)
	if lat > g.Lats[0]+1e-9 || lat < g.Lats[nj-1]-1e-9 {
		return 0, 0, false
	}
	jUpper := sort.Search(nj, func(i int) bool { return g.Lats[i] <= lat })
	return finishLocate(g, lat, lon, jUpper)
}

func finishLocate(g Gaussian, lat, lon float64, jUpper int) (float64, float64, bool) {
	nj := len(g.Lats)
	var fj float64
	switch {
	case jUpper == 0:
		fj = 0
	case jUpper >= nj:
		fj = float64(nj - 1)
	default:
		l1 := g.Lats[jUpper-1]
		l2 := g.Lats[jUpper]
		t := (l1 - lat) / (l1 - l2)
		fj = float64(jUpper-1) + t
	}
	jRound := int(fj + 0.5)
	if jRound < 0 {
		jRound = 0
	}
	if jRound >= nj {
		jRound = nj - 1
	}
	rowW := g.rowOffsets[jRound+1] - g.rowOffsets[jRound]
	di := 360.0 / float64(rowW)
	west := g.Lo1
	lonN := wrap360(lon, west)
	fi := (lonN - west) / di
	if fi < 0 {
		fi += float64(rowW)
	}
	if fi >= float64(rowW) {
		fi -= float64(rowW)
	}
	return fi, fj, true
}
