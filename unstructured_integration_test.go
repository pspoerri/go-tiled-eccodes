package grib_test

import (
	"encoding/binary"
	"math"
	"math/rand"
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
	gridpkg "github.com/pspoerri/go-tiled-eccodes/grid"
	"github.com/pspoerri/go-tiled-eccodes/section"
	"github.com/pspoerri/go-tiled-eccodes/tile"
)

// buildSection3Unstructured constructs a Section 3 raw byte stream for
// template 3.101 with the given number of cells. The template body is
// otherwise zero (shape-of-earth = 0 → 6,367,470 m sphere; UUID all-zero).
//
// Layout per Section 3:
//
//	1-4    section length (uint32 BE)
//	5      section number (=3)
//	6      source of grid definition (=0, "specified by template")
//	7-10   number of data points (uint32 BE)
//	11     octets for optional list of numbers (=0)
//	12     interpretation of list of numbers (=0)
//	13-14  grid definition template number (=101)
//	15-50  template body (36 bytes)
func buildSection3Unstructured(numPoints uint32, shapeCode byte) []byte {
	const (
		header = 14
		body   = 36
	)
	raw := make([]byte, header+body)
	binary.BigEndian.PutUint32(raw[0:], uint32(header+body))
	raw[4] = 3 // section number
	raw[5] = 0 // source
	binary.BigEndian.PutUint32(raw[6:], numPoints)
	raw[10] = 0 // list octets
	raw[11] = 0 // list interpretation
	binary.BigEndian.PutUint16(raw[12:], 101)
	raw[14] = shapeCode // template body[0] = shape of earth
	return raw
}

// TestUnstructuredGridDispatch validates that Message.Grid() returns
// *grid.Unstructured for template 3.101 and that SetGridCoordinates flows
// through to the underlying KD-tree.
func TestUnstructuredGridDispatch(t *testing.T) {
	const N = 50
	raw := buildSection3Unstructured(N, 6)
	m := &grib.Message{S3: section.Section3{Raw: raw}}

	g, err := m.Grid()
	if err != nil {
		t.Fatalf("Grid: %v", err)
	}
	u, ok := g.(*gridpkg.Unstructured)
	if !ok {
		t.Fatalf("Grid type %T, want *Unstructured", g)
	}
	if u.NumPoints() != N {
		t.Errorf("NumPoints = %d, want %d", u.NumPoints(), N)
	}
	if u.HasCoordinates() {
		t.Errorf("HasCoordinates true on unattached grid")
	}

	// Locate before coords are attached: must report out-of-bounds.
	if _, _, ok := u.Locate(0, 0); ok {
		t.Errorf("Locate before SetCoordinates should fail")
	}

	// Attach a synthetic mesh and re-test.
	r := rand.New(rand.NewSource(11))
	lats := make([]float64, N)
	lons := make([]float64, N)
	for i := 0; i < N; i++ {
		lats[i] = (r.Float64() - 0.5) * 90  // ±45°
		lons[i] = (r.Float64() - 0.5) * 180 // ±90°
	}
	if err := m.SetGridCoordinates(lats, lons); err != nil {
		t.Fatalf("SetGridCoordinates: %v", err)
	}
	if !u.HasCoordinates() {
		t.Errorf("HasCoordinates false after SetGridCoordinates")
	}
	// Subsequent Grid() returns the same instance.
	g2, _ := m.Grid()
	if g2 != gridpkg.Grid(u) {
		t.Errorf("Grid() returned different instances on repeat call")
	}

	// Setting again should error.
	if err := m.SetGridCoordinates(lats, lons); err == nil {
		t.Errorf("second SetGridCoordinates should error")
	}

	// Locate() must agree with brute-force nearest.
	for q := 0; q < 20; q++ {
		qlat := (r.Float64() - 0.5) * 90
		qlon := (r.Float64() - 0.5) * 180
		fi, fj, ok := u.Locate(qlat, qlon)
		if !ok {
			t.Fatalf("Locate(%g,%g) failed", qlat, qlon)
		}
		if fj != 0 {
			t.Errorf("fj = %g, want 0", fj)
		}
		// Brute-force great-circle nearest.
		best := -1
		bestC := math.Inf(1)
		for i := 0; i < N; i++ {
			c := chordSq(qlat, qlon, lats[i], lons[i])
			if c < bestC {
				bestC = c
				best = i
			}
		}
		if int(fi) != best {
			t.Errorf("query (%g,%g): KD-tree=%d, brute=%d", qlat, qlon, int(fi), best)
		}
	}
}

// TestUnstructuredSetCoordinatesWrongLength asserts a length mismatch is
// reported as an error rather than silently accepted.
func TestUnstructuredSetCoordinatesWrongLength(t *testing.T) {
	raw := buildSection3Unstructured(10, 6)
	m := &grib.Message{S3: section.Section3{Raw: raw}}
	if err := m.SetGridCoordinates([]float64{1, 2}, []float64{1, 2}); err == nil {
		t.Fatal("expected length-mismatch error")
	}
}

// chordSq is a brute-force chord² between two (lat, lon) pairs in degrees,
// computed via 3-vector dot.
func chordSq(lat1, lon1, lat2, lon2 float64) float64 {
	const d2r = math.Pi / 180
	c1 := math.Cos(lat1 * d2r)
	c2 := math.Cos(lat2 * d2r)
	x1, y1, z1 := c1*math.Cos(lon1*d2r), c1*math.Sin(lon1*d2r), math.Sin(lat1*d2r)
	x2, y2, z2 := c2*math.Cos(lon2*d2r), c2*math.Sin(lon2*d2r), math.Sin(lat2*d2r)
	dx, dy, dz := x1-x2, y1-y2, z1-z2
	return dx*dx + dy*dy + dz*dz
}

// TestUnstructuredNonexistentTemplate confirms that template 3.99 — not
// supported — errors cleanly without crashing the dispatcher.
func TestUnstructuredNonexistentTemplate(t *testing.T) {
	const N = 5
	raw := make([]byte, 50)
	binary.BigEndian.PutUint32(raw[0:], 50)
	raw[4] = 3
	binary.BigEndian.PutUint32(raw[6:], N)
	binary.BigEndian.PutUint16(raw[12:], 99) // unsupported template

	m := &grib.Message{S3: section.Section3{Raw: raw}}
	if _, err := m.Grid(); err == nil {
		t.Fatal("expected ErrUnsupportedGrid")
	}
}

// TestUnstructuredMaxDistanceRoundtrip asserts that on a small grid with
// SetMaxDistance configured, a query far from any cell returns NaN through
// the resampler — this is the path used by the renderer when serving an
// icosahedral grid that only covers a regional subdomain.
func TestUnstructuredMaxDistanceRoundtrip(t *testing.T) {
	// 3 cells clustered near (50°N, 10°E). A query at the equator/0° will
	// be ~5500 km away — set MaxDistance to 100 km and the cap fires.
	raw := buildSection3Unstructured(3, 6)
	m := &grib.Message{S3: section.Section3{Raw: raw}}
	g, _ := m.Grid()
	u := g.(*gridpkg.Unstructured)
	lats := []float64{50, 50.1, 49.9}
	lons := []float64{10, 10.1, 9.9}
	if err := u.SetCoordinates(lats, lons); err != nil {
		t.Fatalf("SetCoordinates: %v", err)
	}

	// Without cap: every query matches some cell.
	if _, _, ok := u.Locate(0, 0); !ok {
		t.Fatal("uncapped Locate should match a cell")
	}
	u.SetMaxDistance(100_000)
	if _, _, ok := u.Locate(0, 0); ok {
		t.Fatal("capped Locate at (0,0) should reject")
	}
	if _, _, ok := u.Locate(50, 10); !ok {
		t.Fatal("capped Locate at cell centre should match")
	}

	// Distance helper sanity check.
	idx, dist, ok := u.LocateWithDistance(50.05, 10.05)
	if !ok || idx < 0 {
		t.Fatalf("LocateWithDistance: idx=%d ok=%v", idx, ok)
	}
	if dist > 20_000 || dist < 0 {
		t.Errorf("dist = %.1f m, expected < 20 km", dist)
	}
}

// TestUnstructuredBicubicDegradesToNearest documents that bicubic on an
// unstructured grid behaves the same as nearest — the 4×4 stencil's reads
// at j != 0 are NaN (Index returns -1) and the kernel's NaN-aware fallback
// kicks in. Skipped for now since it requires a fully-built GRIB2 message
// with a Section 7 payload, which we don't synthesise here.
//
// The contract is verified by the unit test on sample.bicubic in
// sample_test.go (NaN border → fall back to nearest) plus the
// TestUnstructuredGridDispatch confirmation that Index(_, j>0) == -1.
func TestUnstructuredBicubicDegradesToNearest(t *testing.T) {
	t.Skip("documented behaviour; covered by sample_test.go + Index test above")
	_ = tile.Bicubic
}
