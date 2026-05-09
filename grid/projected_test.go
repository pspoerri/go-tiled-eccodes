package grid

import (
	"math"
	"testing"
)

// roundTrip checks that projecting (lat, lon) to grid coords and back
// recovers the original geographic point, by comparing against a
// fresh project() of the original lat/lon.
func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestMercatorLocate(t *testing.T) {
	// Synthetic Mercator: 360-cell-wide, ~360 m cells at the equator
	// (LaD=0 so scaleR == earthRadiusMeters). Origin at the equator,
	// Lo1 = -180.
	g := Mercator{
		rectScan: rectScan{Nx: 360, Ny: 180, IPositive: true, JPositive: false, Consecutive: true},
		La1:      89, Lo1: -180,
		LaD: 0,
		Di:  111000, // ~1° at equator
		Dj:  111000,
	}
	g.scaleR = earthRadiusMeters * math.Cos(g.LaD*deg2rad)
	g.yOrigin = g.scaleR * math.Log(math.Tan(math.Pi/4+g.La1*deg2rad/2))

	// Origin: (La1, Lo1) → (0, 0) within rounding.
	fi, fj, ok := g.Locate(g.La1, g.Lo1)
	if !ok {
		t.Fatalf("Locate origin: ok=false")
	}
	if !near(fi, 0, 0.5) || !near(fj, 0, 0.5) {
		t.Errorf("Locate origin = (%v, %v), want ≈ (0, 0)", fi, fj)
	}

	// One cell east of origin.
	fi, fj, ok = g.Locate(g.La1, g.Lo1+1)
	if !ok || !near(fi, 1, 0.05) {
		t.Errorf("Locate 1° east = (%v, %v) ok=%v, want fi≈1", fi, fj, ok)
	}

	// Far out of bounds.
	if _, _, ok := g.Locate(0, -200); ok {
		t.Errorf("Locate(0, -200) should be out of bounds")
	}

	// wrap180 sanity.
	for _, c := range []struct {
		in, want float64
	}{{0, 0}, {180, -180}, {-180, -180}, {200, -160}, {-200, 160}, {360, 0}} {
		if got := wrap180(c.in); !near(got, c.want, 1e-9) {
			t.Errorf("wrap180(%v) = %v, want %v", c.in, got, c.want)
		}
	}

	// Degenerate Di/Dj returns ok=false.
	bad := g
	bad.Di = 0
	if _, _, ok := bad.Locate(0, 0); ok {
		t.Errorf("Locate with Di=0 should be ok=false")
	}
}

func TestPolarLocate(t *testing.T) {
	// North-polar with the pole at (90, 0), Dx/Dy=10 km, La1 chosen so the
	// origin sits one cell south of the pole on the central meridian.
	g := Polar{
		rectScan: rectScan{Nx: 100, Ny: 100, IPositive: true, JPositive: false, Consecutive: true},
		La1:      89, Lo1: 0,
		LaD: 60, LoV: 0,
		Dx: 10000, Dy: 10000,
		NorthHem: true,
	}
	g.scaleFromLaD = (1 + math.Sin(math.Abs(g.LaD)*deg2rad)) * earthRadiusMeters
	x0, y0 := g.project(g.La1, g.Lo1)
	g.xOrigin, g.yOrigin = x0, y0

	// Origin → (0, 0).
	fi, fj, ok := g.Locate(g.La1, g.Lo1)
	if !ok || !near(fi, 0, 0.5) || !near(fj, 0, 0.5) {
		t.Errorf("Locate origin = (%v, %v) ok=%v, want ≈ (0,0)", fi, fj, ok)
	}

	// Southern-hemisphere point on a north-polar projection: out of bounds.
	if _, _, ok := g.Locate(-45, 0); ok {
		t.Errorf("Locate(-45, 0) on north-polar should be out of bounds")
	}

	// Switch to south hemisphere; northern points become out of bounds.
	gs := g
	gs.NorthHem = false
	if _, _, ok := gs.Locate(45, 0); ok {
		t.Errorf("Locate(45, 0) on south-polar should be out of bounds")
	}

	// Degenerate Dx returns ok=false.
	bad := g
	bad.Dx = 0
	if _, _, ok := bad.Locate(89, 0); ok {
		t.Errorf("Locate with Dx=0 should be ok=false")
	}
}

func TestLambertLocate(t *testing.T) {
	// Two-parallel cone through a HRRR-like configuration.
	g := Lambert{
		rectScan: rectScan{Nx: 200, Ny: 200, IPositive: true, JPositive: false, Consecutive: true},
		La1:      30, Lo1: -100,
		LaD: 38.5, LoV: -97.5,
		Dx: 3000, Dy: 3000,
		Latin1: 38.5, Latin2: 38.5, // tangent cone (single parallel)
	}
	phi1 := g.Latin1 * deg2rad
	g.n = math.Sin(phi1)
	g.F = math.Cos(phi1) * math.Pow(math.Tan(math.Pi/4+phi1/2), g.n) / g.n
	g.xOrigin, g.yOrigin = g.project(g.La1, g.Lo1)

	// Origin maps to (0, 0).
	fi, fj, ok := g.Locate(g.La1, g.Lo1)
	if !ok || !near(fi, 0, 0.5) || !near(fj, 0, 0.5) {
		t.Errorf("Locate origin = (%v, %v) ok=%v, want ≈ (0,0)", fi, fj, ok)
	}

	// A point well outside the grid extent in projected metres.
	if _, _, ok := g.Locate(0, 100); ok {
		t.Errorf("Locate(0, 100) should be out of bounds")
	}

	// Two-parallel cone (Latin1 != Latin2) hits the alternate branch.
	g2 := g
	g2.Latin1 = 33
	g2.Latin2 = 45
	phi1 = g2.Latin1 * deg2rad
	phi2 := g2.Latin2 * deg2rad
	g2.n = math.Log(math.Cos(phi1)/math.Cos(phi2)) /
		math.Log(math.Tan(math.Pi/4+phi2/2)/math.Tan(math.Pi/4+phi1/2))
	g2.F = math.Cos(phi1) * math.Pow(math.Tan(math.Pi/4+phi1/2), g2.n) / g2.n
	g2.xOrigin, g2.yOrigin = g2.project(g2.La1, g2.Lo1)
	if _, _, ok := g2.Locate(g2.La1, g2.Lo1); !ok {
		t.Errorf("two-parallel Locate origin: ok=false")
	}

	// Degenerate Dx returns ok=false.
	bad := g
	bad.Dx = 0
	if _, _, ok := bad.Locate(g.La1, g.Lo1); ok {
		t.Errorf("Locate with Dx=0 should be ok=false")
	}
}

func TestLatLonBounds(t *testing.T) {
	g := LatLon{Ni: 360, Nj: 181, La1: 90, Lo1: 0, La2: -90, Lo2: 359, Di: 1, Dj: 1}
	s, w, n, e := g.Bounds()
	if !near(n, 90, 1e-9) || !near(s, -90, 1e-9) {
		t.Errorf("Bounds lat = (%v, %v), want (-90, 90)", s, n)
	}
	if !near(w, 0, 1e-9) || !near(e, 359, 1e-9) {
		t.Errorf("Bounds lon = (%v, %v), want (0, 359)", w, e)
	}

	// Inverted convention: La1 < La2 should still produce north > south.
	g2 := LatLon{La1: -10, La2: 10, Lo1: 0, Lo2: 5}
	s, _, n, _ = g2.Bounds()
	if !near(n, 10, 1e-9) || !near(s, -10, 1e-9) {
		t.Errorf("inverted Bounds lat = (%v, %v), want (-10, 10)", s, n)
	}

	// Antimeridian: east < west should be lifted by 360.
	g3 := LatLon{La1: 10, La2: -10, Lo1: 350, Lo2: 10}
	_, w, _, e = g3.Bounds()
	if !near(w, 350, 1e-9) || !near(e, 370, 1e-9) {
		t.Errorf("antimeridian Bounds lon = (%v, %v), want (350, 370)", w, e)
	}
}

func TestLatLonIndexAndIsNatural(t *testing.T) {
	natural := LatLon{Ni: 4, Nj: 3, IPositive: true, JPositive: false, Consecutive: true}
	if !natural.IsNatural() {
		t.Errorf("IsNatural = false for natural-scanning grid")
	}
	if got := natural.Index(2, 1); got != 1*4+2 {
		t.Errorf("Index(2, 1) = %d, want %d", got, 1*4+2)
	}
	if got := natural.Index(-1, 0); got != -1 {
		t.Errorf("out-of-bounds Index = %d, want -1", got)
	}

	// J-positive (south-to-north storage).
	jp := natural
	jp.JPositive = true
	if jp.IsNatural() {
		t.Errorf("IsNatural should reject JPositive grid")
	}
	if got := jp.Index(2, 1); got != (3-1-1)*4+2 {
		t.Errorf("JPositive Index = %d, want %d", got, (3-1-1)*4+2)
	}

	// Column-major (Consecutive=false).
	cm := natural
	cm.Consecutive = false
	if got := cm.Index(2, 1); got != 2*3+1 {
		t.Errorf("column-major Index = %d, want %d", got, 2*3+1)
	}

	// Alternating row direction.
	alt := natural
	alt.Alternate = true
	// For odd j, i is reversed.
	if got := alt.Index(0, 1); got != 1*4+(4-1) {
		t.Errorf("alternate Index(0,1) = %d, want %d", got, 1*4+(4-1))
	}
}

func TestLatLonLocateAntimeridian(t *testing.T) {
	// Grid spanning the antimeridian: Lo1=350, Lo2=10 (after wrap to 370).
	// Lat range La1=10 to La2=-10 in 1° steps → fj measured from north.
	g := LatLon{Ni: 21, Nj: 21, La1: 10, Lo1: 350, La2: -10, Lo2: 10, Di: 1, Dj: 1}

	// fj = (north - lat) / Dj. lat=10 → fj=0; lat=-10 → fj=20.
	fi, fj, ok := g.Locate(10, 5)
	if !ok {
		t.Fatalf("Locate inside antimeridian band: ok=false")
	}
	if !near(fj, 0, 1e-6) {
		t.Errorf("fj = %v, want 0", fj)
	}
	// 5° lon — wrapped into [350, 350+360), that's 365 → fi = (365-350)/1 = 15.
	if !near(fi, 15, 1e-6) {
		t.Errorf("fi = %v, want 15", fi)
	}

	// Lat outside extent.
	if _, _, ok := g.Locate(80, 0); ok {
		t.Errorf("Locate(80, 0) should be out of bounds")
	}
}

func TestParseProjScan(t *testing.T) {
	// Natural: i+, j-, consecutive, no alternate.
	ip, jp, c, a := parseProjScan(0)
	if !ip || jp || !c || a {
		t.Errorf("parseProjScan(0) = (%v,%v,%v,%v), want (true,false,true,false)", ip, jp, c, a)
	}
	// All bits set.
	ip, jp, c, a = parseProjScan(0xf0)
	if ip || !jp || c || !a {
		t.Errorf("parseProjScan(0xf0) wrong")
	}
}
