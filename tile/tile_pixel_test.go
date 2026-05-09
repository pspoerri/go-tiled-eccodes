package tile

import (
	"math"
	"testing"
)

// Tile (0,0,0) covers the world. The pixel at the centre of the tile should
// land near (0, 0). Edge pixels approach ±~85° / ±180°.
func TestPixelWorldTile(t *testing.T) {
	tl := XYZ{Z: 0, X: 0, Y: 0}

	// Pixel centres are 0.5-pixel-inset, so the most central we can sample
	// in a 256-wide tile is column 127 or 128. (127.5/256)·360 - 180 ≈ -0.7°.
	for _, px := range []int{127, 128} {
		_, lon := tl.Pixel(px, 128, 256, 256)
		if math.Abs(lon) > 1.0 {
			t.Errorf("near-centre lon at px=%d = %v, want |lon| < 1°", px, lon)
		}
	}

	// (0, 0) pixel sits at the north-west of the tile.
	_, lon := tl.Pixel(0, 0, 256, 256)
	if lon < -180 || lon > -179 {
		t.Errorf("NW lon = %v, want close to -180", lon)
	}

	// (255, 255) is the south-east pixel.
	_, lon = tl.Pixel(255, 255, 256, 256)
	if lon < 179 || lon > 180 {
		t.Errorf("SE lon = %v, want close to +180", lon)
	}
}

// Pixel uses a linear-in-tile-y latitude approximation (an explicit perf
// trade documented in tile.go), while Build uses the true mercY per row.
// Within one tile the gap should be sub-pixel for typical zooms — verify
// it stays within ~1° at z=5.
func TestPixelLinearLatTradeoff(t *testing.T) {
	tl := XYZ{Z: 5, X: 17, Y: 10}
	g := Build(tl, 256, 256)

	// Longitude is linear in both, so they should match exactly.
	for _, px := range []int{0, 64, 128, 255} {
		_, lon := tl.Pixel(px, 0, 256, 256)
		if math.Abs(lon-g.Lons[px]) > 1e-9 {
			t.Errorf("Pixel/Build lon mismatch px=%d: %v vs %v", px, lon, g.Lons[px])
		}
	}

	// Latitudes diverge but stay within ~1°.
	for _, py := range []int{0, 64, 128, 200, 255} {
		lat, _ := tl.Pixel(0, py, 256, 256)
		if math.Abs(lat-g.Lats[py]) > 1.0 {
			t.Errorf("Pixel/Build lat divergence py=%d > 1°: %v vs %v", py, lat, g.Lats[py])
		}
	}
}
