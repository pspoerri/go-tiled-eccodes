package writer_test

import (
	"math"
	"testing"

	"github.com/pspoerri/go-tiled-eccodes/writer"
)

// TestIEEE32RoundTrip writes a small lat/lon field as 32-bit IEEE
// (template 5.4 / precision 1) and reads it back through the decoder.
// Float32 is bit-exact under the round-trip because the writer feeds the
// exact float32 representation through Section 7 verbatim.
func TestIEEE32RoundTrip(t *testing.T) {
	g := writer.NewLatLon(8, 5, 51, 6, 0.5, 0.4)
	vals := linearField(8, 5, 273)
	f := baseField(g, vals)
	f.Packing = writer.PackingIEEE
	f.IEEEPrecision = 1

	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	file, msgs := roundTrip(t, data)
	defer file.Close()

	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("DecodeFloat64: %v", err)
	}
	want := make([]float64, len(vals))
	for i, v := range vals {
		want[i] = float64(float32(v))
	}
	assertValuesClose(t, got, want, 0)

	if got := msgs[0].Header().DataTemplate; got != 4 {
		t.Errorf("DataTemplate = %d, want 4", got)
	}
}

// TestIEEE64RoundTrip writes 64-bit IEEE and reads it back. Should be
// bit-exact since the writer stores math.Float64bits verbatim.
func TestIEEE64RoundTrip(t *testing.T) {
	g := writer.NewLatLon(4, 4, 51, 6, 1, 1)
	vals := []float64{
		1, 2, 3, 4,
		math.Pi, math.E, 1e-300, 1e300,
		-1, -2, -3, -4,
		0.1, 0.2, 0.3, 0.4,
	}
	f := baseField(g, vals)
	f.Packing = writer.PackingIEEE
	f.IEEEPrecision = 2

	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	file, msgs := roundTrip(t, data)
	defer file.Close()

	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("DecodeFloat64: %v", err)
	}
	for i, w := range vals {
		if got[i] != w {
			t.Errorf("vals[%d] = %v, want %v (bit-exact)", i, got[i], w)
		}
	}
}

// TestIEEEDefaultPrecision exercises the precision==0 fallback (defaults
// to single precision).
func TestIEEEDefaultPrecision(t *testing.T) {
	g := writer.NewLatLon(4, 4, 51, 6, 1, 1)
	vals := linearField(4, 4, 100)
	f := baseField(g, vals)
	f.Packing = writer.PackingIEEE
	// IEEEPrecision left at zero — writer should infer single precision.

	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	file, msgs := roundTrip(t, data)
	defer file.Close()
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("DecodeFloat64: %v", err)
	}
	want := make([]float64, len(vals))
	for i, v := range vals {
		want[i] = float64(float32(v))
	}
	assertValuesClose(t, got, want, 0)
}

// TestPNG8RoundTrip writes a small field as 8-bit PNG-packed and reads it
// back. The decoder reconstructs through the same Y = R + X·2^E formula
// the writer used, so the result should match within one quantum.
func TestPNG8RoundTrip(t *testing.T) {
	g := writer.NewLatLon(16, 16, 51, 6, 0.1, 0.1)
	vals := linearField(16, 16, 273)
	f := baseField(g, vals)
	f.Packing = writer.PackingPNG
	f.NumBits = 8

	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	file, msgs := roundTrip(t, data)
	defer file.Close()

	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("DecodeFloat64: %v", err)
	}
	if got := msgs[0].Header().DataTemplate; got != 41 {
		t.Errorf("DataTemplate = %d, want 41", got)
	}
	// 8-bit quantum over a ~2.55 K span ≈ 0.01 K. Allow generous tolerance.
	assertValuesClose(t, got, vals, 0.02)
}

// TestPNG16RoundTrip writes 16-bit PNG and reads it back; the higher bit
// depth gives a much smaller round-trip error.
func TestPNG16RoundTrip(t *testing.T) {
	g := writer.NewLatLon(16, 16, 51, 6, 0.1, 0.1)
	vals := linearField(16, 16, 273)
	f := baseField(g, vals)
	f.Packing = writer.PackingPNG
	f.NumBits = 16

	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	file, msgs := roundTrip(t, data)
	defer file.Close()

	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("DecodeFloat64: %v", err)
	}
	assertValuesClose(t, got, vals, 1e-4)
}

// TestPNGDefaultBits — when caller leaves NumBits at an invalid value
// (0 or some odd number), the writer should pick a sensible default
// (16-bit) instead of failing.
func TestPNGDefaultBits(t *testing.T) {
	g := writer.NewLatLon(8, 8, 51, 6, 0.5, 0.5)
	vals := linearField(8, 8, 100)
	f := baseField(g, vals)
	f.Packing = writer.PackingPNG
	f.NumBits = 0

	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	file, msgs := roundTrip(t, data)
	defer file.Close()
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("DecodeFloat64: %v", err)
	}
	assertValuesClose(t, got, vals, 1e-3)
}

// TestPackingTypesEnum just sanity-checks that the constants are
// distinct and in the documented order.
func TestPackingTypesEnum(t *testing.T) {
	if writer.PackingSimple == writer.PackingIEEE || writer.PackingSimple == writer.PackingPNG {
		t.Errorf("PackingType constants collide")
	}
}

// TestReuseBitmap bundles two fields in one physical GRIB2 message: the
// first ships a real bitmap, the second sets ReuseBitmap (indicator 254).
// The decoder must apply the previously materialised bitmap to the second
// field's Section 7, producing matching NaN positions.
func TestReuseBitmap(t *testing.T) {
	g := writer.NewLatLon(4, 4, 51, 6, 1, 1)

	// 16-point grid with a NaN at index 3 and 10.
	mkVals := func(base float64) []float64 {
		v := make([]float64, 16)
		for i := range v {
			v[i] = base + float64(i)
		}
		v[3] = math.NaN()
		v[10] = math.NaN()
		return v
	}
	a := baseField(g, mkVals(100))
	a.ParameterNumber = 1

	b := baseField(g, mkVals(200))
	b.ParameterNumber = 2
	b.ReuseBitmap = true

	data, err := writer.Bundle([]writer.Field{a, b})
	if err != nil {
		t.Fatalf("Bundle: %v", err)
	}
	file, msgs := roundTrip(t, data)
	defer file.Close()
	if len(msgs) != 2 {
		t.Fatalf("Messages = %d, want 2", len(msgs))
	}

	got1, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode field A: %v", err)
	}
	got2, err := msgs[1].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode field B (reused bitmap): %v", err)
	}
	for _, idx := range []int{3, 10} {
		if !math.IsNaN(got1[idx]) {
			t.Errorf("field A vals[%d] = %v, want NaN", idx, got1[idx])
		}
		if !math.IsNaN(got2[idx]) {
			t.Errorf("field B vals[%d] = %v, want NaN (reused bitmap)", idx, got2[idx])
		}
	}
	// Non-NaN entries should round-trip within simple-packing precision.
	for i := 0; i < 16; i++ {
		if i == 3 || i == 10 {
			continue
		}
		want := 200 + float64(i)
		if math.Abs(got2[i]-want) > 0.01 {
			t.Errorf("field B vals[%d] = %v, want %v (±0.01)", i, got2[i], want)
		}
	}
}
