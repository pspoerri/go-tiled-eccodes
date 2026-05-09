package tile

import (
	"math"
	"testing"
)

func TestBoundsZ0(t *testing.T) {
	// At z=0 a single tile spans the whole world (in spherical Mercator).
	s, w, n, e := XYZ{Z: 0, X: 0, Y: 0}.Bounds()
	if math.Abs(w+180) > 1e-9 || math.Abs(e-180) > 1e-9 {
		t.Fatalf("lon = [%g .. %g], want [-180 .. 180]", w, e)
	}
	if n < 85 || n > 86 {
		t.Fatalf("north = %g, want ~85.05", n)
	}
	if s > -85 || s < -86 {
		t.Fatalf("south = %g, want ~-85.05", s)
	}
}

func TestBoundsZ1Quadrants(t *testing.T) {
	// At z=1, four tiles partition the world. Tile (0,0) is NW.
	_, w, n, _ := XYZ{Z: 1, X: 0, Y: 0}.Bounds()
	if math.Abs(w+180) > 1e-9 {
		t.Errorf("NW west = %g, want -180", w)
	}
	if n < 85 || n > 86 {
		t.Errorf("NW north = %g, want ~85.05", n)
	}
	// Tile (1,1) is SE.
	s, _, _, e := XYZ{Z: 1, X: 1, Y: 1}.Bounds()
	if math.Abs(e-180) > 1e-9 {
		t.Errorf("SE east = %g, want 180", e)
	}
	if s > -85 || s < -86 {
		t.Errorf("SE south = %g, want ~-85.05", s)
	}
}

func TestPixelRoundtrip(t *testing.T) {
	tile := XYZ{Z: 5, X: 17, Y: 10}
	g := Build(tile, 256, 256)
	if len(g.Lons) != 256 || len(g.Lats) != 256 {
		t.Fatalf("dims = %d/%d, want 256/256", len(g.Lons), len(g.Lats))
	}
	// Pixel (0,0) and (255,255) bracket the tile bounds.
	s, w, n, e := tile.Bounds()
	if g.Lons[0] < w || g.Lons[0] > e {
		t.Errorf("first lon = %g, expected in [%g, %g]", g.Lons[0], w, e)
	}
	if g.Lats[0] > n || g.Lats[0] < s {
		t.Errorf("first lat = %g, expected in [%g, %g]", g.Lats[0], s, n)
	}
	// Latitudes are strictly decreasing (N→S).
	for i := 1; i < len(g.Lats); i++ {
		if g.Lats[i] >= g.Lats[i-1] {
			t.Fatalf("lat not strictly decreasing at %d: %g, %g", i, g.Lats[i-1], g.Lats[i])
		}
	}
}
