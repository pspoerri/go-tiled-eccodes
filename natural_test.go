package grib_test

import (
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
	"github.com/pspoerri/go-tiled-eccodes/tile"
)

// TestRegularLatLonAccessor checks the normalized grid-def against the known
// 16x31 natural lat/lon fixture (La1=60, Lo1=0, Di=Dj=2, N→S/W→E scan).
func TestRegularLatLonAccessor(t *testing.T) {
	f, err := grib.Open(loadTestdata(t, "regular_ll_sfc.grib2"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	rl, ok := f.Messages()[0].RegularLatLon()
	if !ok {
		t.Fatal("RegularLatLon ok=false on a template-3.0 message")
	}
	want := grib.RegularLatLon{Nx: 16, Ny: 31, Lat0: 60, Lon0: 0, DLat: -2, DLon: 2}
	if rl != want {
		t.Errorf("RegularLatLon = %+v, want %+v", rl, want)
	}
}

// TestRegularLatLonNormalizesScan checks corner derivation on icon-d2, a
// regular lat/lon grid scanned S→N (JPositive). The message's first point is
// the SOUTH edge (La1=43.18), so a correct accessor must walk to the north:
// Lat0≈58.08 with DLat<0, not the raw La1.
func TestRegularLatLonNormalizesScan(t *testing.T) {
	f, err := grib.Open(loadTestdata(t, "icon-d2_t_2m.grib2"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	rl, ok := f.Messages()[0].RegularLatLon()
	if !ok {
		t.Fatal("RegularLatLon ok=false")
	}
	if rl.Lat0 < 58.0 || rl.Lat0 > 58.1 {
		t.Errorf("Lat0=%v, want ~58.08 (north point, not the La1 south point)", rl.Lat0)
	}
	if rl.Lon0 < 356.0 || rl.Lon0 > 356.1 {
		t.Errorf("Lon0=%v, want ~356.06 (west point)", rl.Lon0)
	}
	if rl.DLat >= 0 {
		t.Errorf("DLat=%v, want <0 (N→S)", rl.DLat)
	}
	if rl.DLon <= 0 {
		t.Errorf("DLon=%v, want >0 (W→E)", rl.DLon)
	}
}

// TestRegularLatLonRejectsOther confirms ok=false for non-template-3.0 grids.
func TestRegularLatLonRejectsOther(t *testing.T) {
	for _, name := range []string{"rotated.grib2", "regular_gg.grib2", "polar_sfc.grib2"} {
		f, err := grib.Open(loadTestdata(t, name))
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		if _, ok := f.Messages()[0].RegularLatLon(); ok {
			t.Errorf("%s: RegularLatLon ok=true, want false", name)
		}
		f.Close()
	}
}

// TestDecodeNaturalOrdering checks that DecodeNatural agrees with DecodeFloat32
// (storage order) on a natural grid and genuinely *differs* on a non-natural
// one — the latter proves the scan was actually un-scrambled rather than
// copied through. icon-d2 scans S→N, so its storage order is row-flipped vs
// natural order.
func TestDecodeNaturalOrdering(t *testing.T) {
	for _, tc := range []struct {
		name    string
		natural bool
	}{
		{"regular_ll_sfc.grib2", true},
		{"icon-d2_t_2m.grib2", false},
	} {
		f, err := grib.Open(loadTestdata(t, tc.name))
		if err != nil {
			t.Fatalf("open %s: %v", tc.name, err)
		}
		m := f.Messages()[0]
		storage, err := m.DecodeFloat32(nil) // message's own scan order
		if err != nil {
			t.Fatalf("%s DecodeFloat32: %v", tc.name, err)
		}
		nat, err := m.DecodeNaturalFloat32(nil)
		if err != nil {
			t.Fatalf("%s DecodeNaturalFloat32: %v", tc.name, err)
		}
		if len(nat) != len(storage) {
			t.Fatalf("%s: len(nat)=%d, len(storage)=%d", tc.name, len(nat), len(storage))
		}
		eq := equalF32(nat, storage)
		if tc.natural && !eq {
			t.Errorf("%s: natural grid but DecodeNatural != DecodeFloat32", tc.name)
		}
		if !tc.natural && eq {
			t.Errorf("%s: non-natural grid but DecodeNatural == DecodeFloat32 (scan not reordered)", tc.name)
		}
		f.Close()
	}
}

// TestDecodeNaturalGeographic is the independent correctness anchor for the
// reorder path: on the non-natural icon-d2 grid, the natural-order value at
// (i,j) must equal a Nearest sample taken at that grid point's lat/lon. ValueAt
// reaches the value through Locate (a different path than DecodeNatural's direct
// Index walk), so agreement means the reorder lands values at the right
// geography — a swapped axis or off-by-one would surface here.
func TestDecodeNaturalGeographic(t *testing.T) {
	f, err := grib.Open(loadTestdata(t, "icon-d2_t_2m.grib2"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	m := f.Messages()[0]
	rl, ok := m.RegularLatLon()
	if !ok {
		t.Fatal("RegularLatLon ok=false")
	}
	nat, err := m.DecodeNaturalFloat32(nil)
	if err != nil {
		t.Fatalf("DecodeNaturalFloat32: %v", err)
	}
	// Spread of (i,j) including both corners and interior points.
	for _, p := range [][2]int{{0, 0}, {1214, 745}, {0, 745}, {1214, 0}, {100, 50}, {600, 373}, {1000, 700}} {
		i, j := p[0], p[1]
		lat := rl.Lat0 + float64(j)*rl.DLat
		lon := rl.Lon0 + float64(i)*rl.DLon
		v, err := m.ValueAt(lat, lon, tile.Nearest)
		if err != nil {
			t.Fatalf("ValueAt(%g,%g): %v", lat, lon, err)
		}
		got, want := nat[j*rl.Nx+i], float32(v)
		if isNaN32(got) || isNaN32(want) {
			if isNaN32(got) != isNaN32(want) {
				t.Errorf("(%d,%d): NaN mismatch nat=%v ValueAt=%v", i, j, got, want)
			}
			continue
		}
		if got != want {
			t.Errorf("(%d,%d) lat=%g lon=%g: nat=%v, ValueAt=%v", i, j, lat, lon, got, want)
		}
	}
}

// TestDecodeNaturalReusesBuffer confirms a large-enough dst is reused in place.
func TestDecodeNaturalReusesBuffer(t *testing.T) {
	f, err := grib.Open(loadTestdata(t, "regular_ll_sfc.grib2"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	buf := make([]float32, 16*31)
	out, err := f.Messages()[0].DecodeNaturalFloat32(buf)
	if err != nil {
		t.Fatalf("DecodeNaturalFloat32: %v", err)
	}
	if &out[0] != &buf[0] {
		t.Error("DecodeNaturalFloat32 allocated a new slice instead of reusing dst")
	}
}

// TestDecodeNaturalFloat64 confirms the float64 variant produces the same
// ordering and values (modulo the float32 narrowing) as the float32 variant,
// exercising the generic core for T=float64 on the non-natural icon-d2 grid.
func TestDecodeNaturalFloat64(t *testing.T) {
	f, err := grib.Open(loadTestdata(t, "icon-d2_t_2m.grib2"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	m := f.Messages()[0]
	f32, err := m.DecodeNaturalFloat32(nil)
	if err != nil {
		t.Fatalf("DecodeNaturalFloat32: %v", err)
	}
	f64, err := m.DecodeNaturalFloat64(nil)
	if err != nil {
		t.Fatalf("DecodeNaturalFloat64: %v", err)
	}
	if len(f32) != len(f64) {
		t.Fatalf("len mismatch: f32=%d f64=%d", len(f32), len(f64))
	}
	for i := range f64 {
		got, want := f32[i], float32(f64[i])
		if got != want && !(isNaN32(got) && isNaN32(want)) {
			t.Fatalf("index %d: float32=%v, float32(float64)=%v", i, got, want)
		}
	}
}

func isNaN32(f float32) bool { return f != f }

// equalF32 compares element-wise, treating NaN as equal to NaN (so a natural
// grid's missing values don't read as a difference).
func equalF32(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] && !(isNaN32(a[i]) && isNaN32(b[i])) {
			return false
		}
	}
	return true
}
