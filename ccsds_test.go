package grib_test

import (
	"math"
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
)

// TestCCSDSConstantField decodes a CCSDS-packed copy of the regular_ll fixture
// (template 5.42). Source value is 273.15 K everywhere (nbits=0 short-circuit).
func TestCCSDSConstantField(t *testing.T) {
	f, err := grib.Open(loadTestdata(t, "regular_ll_ccsds.grib2"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	m := f.Messages()[0]
	if h := m.Header(); h.DataTemplate != 42 || h.Ni != 16 || h.Nj != 31 {
		t.Fatalf("header = tmpl %d %dx%d, want 42 16x31", h.DataTemplate, h.Ni, h.Nj)
	}
	vals, err := m.DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(vals) != 16*31 {
		t.Fatalf("len = %d, want 496", len(vals))
	}
	for i, v := range vals {
		if math.Abs(v-273.15) > 1e-3 {
			t.Fatalf("vals[%d] = %g, want 273.15", i, v)
		}
	}
}

// TestCCSDSICOND2 decodes a CCSDS-packed ICON-D2 t_2m forecast and cross-checks
// per-cell values against the simple-packed reference (≤ 0.05 K). This is the
// real exercise of the pure-Go AEC decoder.
func TestCCSDSICOND2(t *testing.T) {
	ref, err := grib.Open(loadTestdata(t, "icon-d2_t_2m.grib2"))
	if err != nil {
		t.Fatalf("open ref: %v", err)
	}
	defer ref.Close()
	got, err := grib.Open(loadTestdata(t, "icon-d2_t_2m_ccsds.grib2"))
	if err != nil {
		t.Fatalf("open ccsds: %v", err)
	}
	defer got.Close()

	rv, _ := ref.Messages()[0].DecodeFloat64(nil)
	gv, err := got.Messages()[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode ccsds: %v", err)
	}
	if len(rv) != len(gv) {
		t.Fatalf("len mismatch: ref=%d ccsds=%d", len(rv), len(gv))
	}
	maxDiff, mismatches := 0.0, 0
	for i := range rv {
		if math.IsNaN(rv[i]) && math.IsNaN(gv[i]) {
			continue
		}
		if math.IsNaN(rv[i]) != math.IsNaN(gv[i]) {
			mismatches++
			continue
		}
		if d := math.Abs(rv[i] - gv[i]); d > maxDiff {
			maxDiff = d
		}
	}
	t.Logf("ICON-D2 CCSDS roundtrip: %d points, max diff %.4f K, NaN mismatches %d", len(rv), maxDiff, mismatches)
	if mismatches != 0 {
		t.Fatalf("NaN mismatches: %d", mismatches)
	}
	if maxDiff > 0.05 {
		t.Fatalf("max diff = %.4f K, want ≤ 0.05", maxDiff)
	}
}
