//go:build cgo

package grib_test

import (
	"math"
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
)

// TestJPEG2000ICOND2 mirrors TestCCSDSICOND2 for template 5.40. JPEG2000
// is configured by eccodes for lossless mode by default when packingType
// is set to grid_jpeg, so we expect the same precision as CCSDS.
func TestJPEG2000ICOND2(t *testing.T) {
	ref, err := grib.Open(loadTestdata(t, "icon-d2_t_2m.grib2"))
	if err != nil {
		t.Fatalf("open ref: %v", err)
	}
	defer ref.Close()
	got, err := grib.Open(loadTestdata(t, "icon-d2_t_2m_jpeg.grib2"))
	if err != nil {
		t.Fatalf("open jpeg: %v", err)
	}
	defer got.Close()

	rv, _ := ref.Messages()[0].DecodeFloat64(nil)
	gv, err := got.Messages()[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode jpeg: %v", err)
	}
	if len(rv) != len(gv) {
		t.Fatalf("len mismatch: ref=%d jpeg=%d", len(rv), len(gv))
	}
	maxDiff := 0.0
	mismatches := 0
	for i := range rv {
		if math.IsNaN(rv[i]) && math.IsNaN(gv[i]) {
			continue
		}
		if math.IsNaN(rv[i]) != math.IsNaN(gv[i]) {
			mismatches++
			continue
		}
		d := math.Abs(rv[i] - gv[i])
		if d > maxDiff {
			maxDiff = d
		}
	}
	t.Logf("ICON-D2 JPEG2000 roundtrip: %d points, max diff %.4f K, NaN mismatches %d", len(rv), maxDiff, mismatches)
	if mismatches != 0 {
		t.Fatalf("NaN mismatches: %d", mismatches)
	}
	if maxDiff > 0.05 {
		t.Fatalf("max diff = %.4f K, want ≤ 0.05", maxDiff)
	}
}

// TestJPEG2000ConstantField decodes a JPEG2000-packed copy of the
// regular_ll fixture (template 5.40). Source value is 273.15 K everywhere.
func TestJPEG2000ConstantField(t *testing.T) {
	f, err := grib.Open(loadTestdata(t, "regular_ll_jpeg.grib2"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	msgs := f.Messages()
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	m := msgs[0]
	h := m.Header()
	if h.DataTemplate != 40 {
		t.Fatalf("DataTemplate = %d, want 40", h.DataTemplate)
	}
	if h.Ni != 16 || h.Nj != 31 {
		t.Fatalf("dims = %dx%d, want 16x31", h.Ni, h.Nj)
	}

	vals, err := m.DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := len(vals); got != 16*31 {
		t.Fatalf("decoded len = %d, want 496", got)
	}
	for i, v := range vals {
		if math.Abs(v-273.15) > 1e-3 {
			t.Fatalf("vals[%d] = %g, want 273.15 (±1e-3)", i, v)
		}
	}
}
