package grid

import (
	"math"
	"math/rand"
	"testing"
)

func TestUnstructuredParseTemplate(t *testing.T) {
	// Synthetic template-101 body: shape (1), grid-used (3),
	// grid-reference (1), UUID (16).
	body := make([]byte, 21)
	body[0] = 6
	body[1] = 0x0a
	body[2] = 0x0b
	body[3] = 0x0c
	body[4] = 7
	for i := 0; i < 16; i++ {
		body[5+i] = byte(i + 1)
	}
	g := ParseUnstructured(body, 100)
	if g.NumPoints_ != 100 {
		t.Errorf("NumPoints = %d, want 100", g.NumPoints_)
	}
	if g.GridUsed != 0x000a0b0c {
		t.Errorf("GridUsed = %#x, want 0x000a0b0c", g.GridUsed)
	}
	if g.GridReference != 7 {
		t.Errorf("GridReference = %d, want 7", g.GridReference)
	}
	if g.UUID[0] != 1 || g.UUID[15] != 16 {
		t.Errorf("UUID = %v", g.UUID)
	}
	if g.EarthRadiusMeters != 6371229 {
		t.Errorf("EarthRadiusMeters = %v, want 6371229", g.EarthRadiusMeters)
	}
	if g.HasCoordinates() {
		t.Errorf("HasCoordinates true on freshly-parsed grid")
	}
	if lats, lons := g.Coordinates(); lats != nil || lons != nil {
		t.Errorf("Coordinates non-nil before SetCoordinates: %v %v", lats, lons)
	}
}

func TestUnstructuredCoordinates(t *testing.T) {
	g := ParseUnstructured(make([]byte, 36), 3)
	lats := []float64{1, 2, 3}
	lons := []float64{4, 5, 6}
	if err := g.SetCoordinates(lats, lons); err != nil {
		t.Fatalf("SetCoordinates: %v", err)
	}
	gotLats, gotLons := g.Coordinates()
	if &gotLats[0] != &lats[0] || &gotLons[0] != &lons[0] {
		t.Error("Coordinates did not return the backing slices")
	}
}

func TestUnstructuredNearestMatchesBruteForce(t *testing.T) {
	// Build a small synthetic mesh: 200 random points on the sphere.
	r := rand.New(rand.NewSource(42))
	n := 200
	lats := make([]float64, n)
	lons := make([]float64, n)
	for i := 0; i < n; i++ {
		// Uniform on the sphere via inverse CDF on z.
		z := 2*r.Float64() - 1
		lats[i] = math.Asin(z) * 180 / math.Pi
		lons[i] = r.Float64()*360 - 180
	}
	body := make([]byte, 36)
	g := ParseUnstructured(body, n)
	if err := g.SetCoordinates(lats, lons); err != nil {
		t.Fatalf("SetCoordinates: %v", err)
	}

	// For 100 random queries, KD-tree's nearest must agree with brute force.
	for q := 0; q < 100; q++ {
		qz := 2*r.Float64() - 1
		qlat := math.Asin(qz) * 180 / math.Pi
		qlon := r.Float64()*360 - 180

		fi, fj, ok := g.Locate(qlat, qlon)
		if !ok {
			t.Fatalf("query %d: Locate returned ok=false", q)
		}
		if fj != 0 {
			t.Fatalf("query %d: fj = %v, want 0", q, fj)
		}

		// Brute-force nearest.
		best := -1
		bestD2 := math.Inf(1)
		qx, qy, qzz := latLonToVec3(qlat, qlon)
		for i := 0; i < n; i++ {
			x, y, zz := latLonToVec3(lats[i], lons[i])
			d2 := (x-qx)*(x-qx) + (y-qy)*(y-qy) + (zz-qzz)*(zz-qzz)
			if d2 < bestD2 {
				bestD2 = d2
				best = i
			}
		}
		if int(fi) != best {
			t.Fatalf("query %d: hash nearest = %d, brute = %d", q, int(fi), best)
		}
	}
}

func TestUnstructuredMaxDistance(t *testing.T) {
	// Two cells: one at (0, 0), one at (45, 0). A query at (89, 0) is far
	// from both — with no max distance it picks the closer one; with a
	// 100km cap it returns out-of-bounds.
	lats := []float64{0, 45}
	lons := []float64{0, 0}
	body := make([]byte, 36)
	g := ParseUnstructured(body, 2)
	g.SetCoordinates(lats, lons)
	g.EarthRadiusMeters = 6371229

	if _, _, ok := g.Locate(89, 0); !ok {
		t.Fatal("uncapped Locate should always succeed")
	}
	g.SetMaxDistance(100_000) // 100 km
	if _, _, ok := g.Locate(89, 0); ok {
		t.Fatal("capped Locate should reject (89, 0) — nearest cell is ~5000 km away")
	}
	// A query within ~10 km of cell 0 is well inside the cap.
	if fi, _, ok := g.Locate(0.05, 0); !ok || int(fi) != 0 {
		t.Fatalf("near-cell query: got fi=%v ok=%v, want fi=0 ok=true", fi, ok)
	}
}

func TestUnstructuredCellMask(t *testing.T) {
	// Three cells along the equator. Mask the middle one. A query closest
	// to that middle cell must return out-of-bounds; queries closest to
	// the unmasked cells must still resolve.
	lats := []float64{0, 0, 0}
	lons := []float64{-1, 0, 1}
	body := make([]byte, 36)
	g := ParseUnstructured(body, 3)
	if err := g.SetCoordinates(lats, lons); err != nil {
		t.Fatalf("SetCoordinates: %v", err)
	}

	if g.HasCellMask() {
		t.Fatalf("HasCellMask true on freshly-coordinated grid")
	}
	mask := []bool{false, true, false}
	if err := g.SetCellMask(mask); err != nil {
		t.Fatalf("SetCellMask: %v", err)
	}
	if !g.HasCellMask() {
		t.Fatalf("HasCellMask false after SetCellMask")
	}

	// Query at (0, 0) is closest to cell 1 (masked) → out-of-bounds.
	if _, _, ok := g.Locate(0, 0); ok {
		t.Fatal("Locate at masked cell should return ok=false")
	}
	// Query at (0, -1) is closest to cell 0 (unmasked) → ok.
	if fi, _, ok := g.Locate(0, -1); !ok || int(fi) != 0 {
		t.Fatalf("Locate at unmasked cell: fi=%v ok=%v, want 0/true", fi, ok)
	}
	// Query at (0, 1) is closest to cell 2 (unmasked) → ok.
	if fi, _, ok := g.Locate(0, 1); !ok || int(fi) != 2 {
		t.Fatalf("Locate at unmasked cell 2: fi=%v ok=%v, want 2/true", fi, ok)
	}

	// Length mismatch must error.
	if err := g.SetCellMask([]bool{true}); err == nil {
		t.Fatal("SetCellMask with wrong length should error")
	}

	// Clearing the mask restores the masked cell.
	if err := g.SetCellMask(nil); err != nil {
		t.Fatalf("SetCellMask(nil): %v", err)
	}
	if g.HasCellMask() {
		t.Fatalf("HasCellMask true after clearing")
	}
	if fi, _, ok := g.Locate(0, 0); !ok || int(fi) != 1 {
		t.Fatalf("Locate after clearing mask: fi=%v ok=%v, want 1/true", fi, ok)
	}
}

func TestUnstructuredCellMaskSharedFromOther(t *testing.T) {
	// Two grids on the same mesh. Configure one with coords + mask, share
	// to the other via SetCoordinatesFrom; the share must propagate the
	// mask too so masked cells render NaN through both grid handles.
	lats := []float64{0, 0}
	lons := []float64{0, 1}
	body := make([]byte, 36)
	gA := ParseUnstructured(body, 2)
	gB := ParseUnstructured(body, 2)
	if err := gA.SetCoordinates(lats, lons); err != nil {
		t.Fatalf("SetCoordinates A: %v", err)
	}
	if err := gA.SetCellMask([]bool{true, false}); err != nil {
		t.Fatalf("SetCellMask A: %v", err)
	}
	if err := gB.SetCoordinatesFrom(gA); err != nil {
		t.Fatalf("SetCoordinatesFrom: %v", err)
	}
	if !gB.HasCellMask() {
		t.Fatalf("gB has no mask after SetCoordinatesFrom")
	}
	if _, _, ok := gB.Locate(0, 0); ok {
		t.Fatal("gB.Locate at masked cell should return ok=false")
	}
}

func TestUnstructuredLocateWithDistance(t *testing.T) {
	// Single cell at the equator/prime-meridian; query at 1° east. The
	// great-circle arc between the two points on a 6371229m sphere is
	// about 6371229 * π/180 ≈ 111.2 km.
	lats := []float64{0}
	lons := []float64{0}
	body := make([]byte, 36)
	body[0] = 6 // shape-of-earth = 6371229 m sphere
	g := ParseUnstructured(body, 1)
	g.SetCoordinates(lats, lons)

	idx, dist, ok := g.LocateWithDistance(0, 1)
	if !ok || idx != 0 {
		t.Fatalf("idx=%d ok=%v, want 0/true", idx, ok)
	}
	want := 6371229.0 * math.Pi / 180
	if math.Abs(dist-want) > 1.0 {
		t.Errorf("distance = %.3f m, want ≈ %.3f m", dist, want)
	}
}

func TestUnstructuredIndex(t *testing.T) {
	body := make([]byte, 36)
	g := ParseUnstructured(body, 5)
	for i := 0; i < 5; i++ {
		if got := g.Index(i, 0); got != i {
			t.Errorf("Index(%d, 0) = %d, want %d", i, got, i)
		}
	}
	if g.Index(5, 0) != -1 {
		t.Errorf("Index(5, 0) should be -1 (out of bounds)")
	}
	if g.Index(0, 1) != -1 {
		t.Errorf("Index(0, 1) should be -1 (j>0 invalid)")
	}
}

func TestUnstructuredBuildIndexErrorWithoutCoords(t *testing.T) {
	body := make([]byte, 36)
	g := ParseUnstructured(body, 10)
	if err := g.BuildIndex(); err == nil {
		t.Fatal("BuildIndex without coords should error")
	}
}
