package grib

import (
	"math"
	"testing"

	"github.com/pspoerri/go-tiled-eccodes/section"
)

// makeMessageWithParam builds a *Message whose Section 0 / Section 4 bytes
// carry the given (discipline, category, number) triple. Only the fields
// pairLatLonWithMessages reads need valid data; the rest is left zeroed.
func makeMessageWithParam(disc, cat, num uint8) *Message {
	s0 := make([]byte, section.Section0Size)
	copy(s0[:4], []byte("GRIB"))
	s0[6] = disc
	s0[7] = 2 // edition

	// Section 4 minimum: 9 bytes header + 2 bytes (cat, num) + slack so
	// the byte indices used by accessors don't go out of bounds.
	s4 := make([]byte, 16)
	s4[9] = cat
	s4[10] = num

	return &Message{
		S0: section.Section0{Raw: s0},
		S4: section.Section4{Raw: s4},
	}
}

// pairLatLon and AttachCoordinates are the public-ish helpers; we test them
// in the internal grib package so we can call pairLatLon directly without
// having to ship a synthetic GRIB file fixture.

func TestPairLatLonDegrees(t *testing.T) {
	lats := []float64{45.5, -10, 0, 89, -89.5}
	lons := []float64{170, -179, 0, 359, 1}
	gotLats, gotLons, err := pairLatLon([][]float64{lons, lats})
	if err != nil {
		t.Fatalf("pairLatLon: %v", err)
	}
	// pairLatLon should pick lats by max-abs in [0..90+ε], lons by [0..360+ε].
	// First fitting "lat" array wins, regardless of input order, so even
	// though we passed lons-then-lats the output should still be lats=lats.
	for i, v := range gotLats {
		if v != lats[i] {
			t.Fatalf("gotLats[%d] = %g, want %g", i, v, lats[i])
		}
	}
	for i, v := range gotLons {
		if v != lons[i] {
			t.Fatalf("gotLons[%d] = %g, want %g", i, v, lons[i])
		}
	}
}

func TestPairLatLonRadiansConvertedToDegrees(t *testing.T) {
	// DWD ICON ships horizontal_constants in radians. Verify auto-detect
	// and conversion. lats max-abs ≤ π/2 → latitude in radians; lons
	// max-abs ≤ π → longitude in radians.
	latsRad := []float64{0.5, -1.2, 0, 1.5, -1.5} // max ≈ 1.5 < π/2+0.01? π/2 ≈ 1.5708, so yes
	lonsRad := []float64{2.5, -3.0, 0, 3.0, 0.1}  // max = 3.0 < π+0.01
	gotLats, gotLons, err := pairLatLon([][]float64{latsRad, lonsRad})
	if err != nil {
		t.Fatalf("pairLatLon: %v", err)
	}
	const tol = 1e-9
	for i, v := range gotLats {
		want := latsRad[i] * 180 / math.Pi
		if math.Abs(v-want) > tol {
			t.Errorf("gotLats[%d] = %g, want %g (rad→deg)", i, v, want)
		}
	}
	for i, v := range gotLons {
		want := lonsRad[i] * 180 / math.Pi
		if math.Abs(v-want) > tol {
			t.Errorf("gotLons[%d] = %g, want %g (rad→deg)", i, v, want)
		}
	}
}

func TestPairLatLonInsufficientMessages(t *testing.T) {
	if _, _, err := pairLatLon([][]float64{{0, 1, 2}}); err == nil {
		t.Fatal("pairLatLon with one message should error")
	}
}

func TestPairLatLonWithExtraNonCoordinateMessages(t *testing.T) {
	// horizontal_constants files often ship cell area, vertices, etc. in
	// addition to clat/clon. Extra messages whose value range matches
	// neither lat nor lon should be ignored. Here cell area is in m² and
	// has values in the millions.
	lats := []float64{45, -45, 0, 90, -90}
	lons := []float64{0, 90, 180, 270, 359}
	area := []float64{1.5e6, 1.5e6, 1.6e6, 1.4e6, 1.5e6}
	got1, got2, err := pairLatLon([][]float64{area, lats, lons})
	if err != nil {
		t.Fatalf("pairLatLon: %v", err)
	}
	if !sliceEqual(got1, lats) {
		t.Errorf("lat picked = %v, want %v", got1, lats)
	}
	if !sliceEqual(got2, lons) {
		t.Errorf("lon picked = %v, want %v", got2, lons)
	}
}

func TestPairLatLonMissingLat(t *testing.T) {
	// Two arrays both in the lon range — no lat message → error.
	a := []float64{200, 300}
	b := []float64{100, 250}
	if _, _, err := pairLatLon([][]float64{a, b}); err == nil {
		t.Fatal("expected error when no lat-shaped array present")
	}
}

func sliceEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestPairLatLonWithMessagesByParamCode locks in that the WMO parameter
// codes (discipline=0, category=191, number=1/2) are honoured even when
// the value-range heuristic would mis-classify everything as "lat" — the
// MeteoSwiss ICON-CH-EPS regression: regional lats sit in [42, 51] and
// lons in [−1, 18], both ≤ 90, so both look lat-shaped to the legacy
// heuristic. With param codes consulted first we still pick the right
// pair.
func TestPairLatLonWithMessagesByParamCode(t *testing.T) {
	// Surrounding fields that all classify as "lat" under the legacy
	// heuristic (max-abs in [0, 90]):
	frLand := []float64{0.5, 1, 0, 0.7, 0.3} // 0..1 → looks like lat-radians
	hsurf := []float64{500, 1200, 0, 4000, 200}
	soiltyp := []float64{1, 5, 9, 3, 8}     // → looks like lat-radians
	tlon := []float64{1, 5, 12, -0.5, 17}   // CH lon range, classifies as lat-degrees
	tlat := []float64{42.5, 47, 50, 49, 43} // CH lat range, lat-degrees

	messages := []*Message{
		makeMessageWithParam(2, 0, 0),   // FR_LAND
		makeMessageWithParam(0, 3, 6),   // HSURF
		makeMessageWithParam(2, 3, 196), // SOILTYP
		makeMessageWithParam(0, 191, 2), // tlon
		makeMessageWithParam(0, 191, 1), // tlat
	}
	decoded := [][]float64{frLand, hsurf, soiltyp, tlon, tlat}

	gotLats, gotLons, err := pairLatLonWithMessages(messages, decoded)
	if err != nil {
		t.Fatalf("pairLatLonWithMessages: %v", err)
	}
	if !sliceEqual(gotLats, tlat) {
		t.Errorf("got lats %v, want tlat %v", gotLats, tlat)
	}
	if !sliceEqual(gotLons, tlon) {
		t.Errorf("got lons %v, want tlon %v", gotLons, tlon)
	}
}

// TestPairLatLonWithMessagesFallsBack ensures unstructured grids whose
// horizontal_constants don't tag clat/clon with the WMO code still load
// via the value-range heuristic.
func TestPairLatLonWithMessagesFallsBack(t *testing.T) {
	lats := []float64{45, -45, 0, 90, -90}
	lons := []float64{0, 90, 180, 270, 359}
	messages := []*Message{
		makeMessageWithParam(0, 0, 0), // unrelated param
		makeMessageWithParam(0, 0, 1), // unrelated param
	}
	gotLats, gotLons, err := pairLatLonWithMessages(messages, [][]float64{lats, lons})
	if err != nil {
		t.Fatalf("pairLatLonWithMessages: %v", err)
	}
	if !sliceEqual(gotLats, lats) || !sliceEqual(gotLons, lons) {
		t.Errorf("fallback failed: lats=%v lons=%v", gotLats, gotLons)
	}
}

// TestPairLatLonWithMessagesAutoConvertsRadians keeps DWD-style radian
// horizontal_constants working even when the param codes are present.
func TestPairLatLonWithMessagesAutoConvertsRadians(t *testing.T) {
	latsRad := []float64{0.5, -1.0, 0, 1.5, -1.4}
	lonsRad := []float64{2.5, -3.0, 0, 3.0, 0.1}
	messages := []*Message{
		makeMessageWithParam(0, 191, 1), // clat
		makeMessageWithParam(0, 191, 2), // clon
	}
	gotLats, gotLons, err := pairLatLonWithMessages(messages, [][]float64{latsRad, lonsRad})
	if err != nil {
		t.Fatalf("pairLatLonWithMessages: %v", err)
	}
	const tol = 1e-9
	for i, v := range gotLats {
		want := latsRad[i] * 180 / math.Pi
		if math.Abs(v-want) > tol {
			t.Errorf("lats[%d] = %g, want %g (rad→deg)", i, v, want)
		}
	}
	for i, v := range gotLons {
		want := lonsRad[i] * 180 / math.Pi
		if math.Abs(v-want) > tol {
			t.Errorf("lons[%d] = %g, want %g (rad→deg)", i, v, want)
		}
	}
}
