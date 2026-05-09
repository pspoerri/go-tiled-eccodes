package grib_test

import (
	"math"
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
)

// TestComplexPackingRoundtrip decodes the same ICON-D2 t_2m field encoded
// three ways (simple 5.0, complex 5.2, complex+spatial-diff 5.3) and asserts
// the values agree within the float32 quantisation noise inherent to the
// reference value.
func TestComplexPackingRoundtrip(t *testing.T) {
	simple := decodeFixture(t, "icon-d2_t_2m.grib2")
	complex_ := decodeFixture(t, "icon-d2_t_2m_complex.grib2")
	spdiff := decodeFixture(t, "icon-d2_t_2m_spdiff.grib2")

	if len(simple) != len(complex_) || len(simple) != len(spdiff) {
		t.Fatalf("len mismatch: simple=%d complex=%d spdiff=%d",
			len(simple), len(complex_), len(spdiff))
	}

	// Compare element-wise. NaN-vs-NaN is fine; otherwise tolerance is the
	// binary-scale step of the original packing (2^-11 ≈ 5e-4) widened a bit
	// for float32 round-trip noise.
	const tol = 1e-3
	var (
		mismatchedComplex int
		mismatchedSpdiff  int
	)
	for i := range simple {
		s, c, d := simple[i], complex_[i], spdiff[i]
		if math.IsNaN(s) {
			if !math.IsNaN(c) {
				mismatchedComplex++
			}
			if !math.IsNaN(d) {
				mismatchedSpdiff++
			}
			continue
		}
		if math.IsNaN(c) || math.Abs(s-c) > tol {
			mismatchedComplex++
		}
		if math.IsNaN(d) || math.Abs(s-d) > tol {
			mismatchedSpdiff++
		}
	}
	if mismatchedComplex > 0 {
		t.Errorf("complex packing: %d / %d points differ from simple", mismatchedComplex, len(simple))
	}
	if mismatchedSpdiff > 0 {
		t.Errorf("spatial differencing: %d / %d points differ from simple", mismatchedSpdiff, len(simple))
	}
}

func decodeFixture(t *testing.T, name string) []float64 {
	t.Helper()
	f, err := grib.Open(loadTestdata(t, name))
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer f.Close()
	msgs := f.Messages()
	if len(msgs) != 1 {
		t.Fatalf("%s: messages = %d, want 1", name, len(msgs))
	}
	v, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return v
}
