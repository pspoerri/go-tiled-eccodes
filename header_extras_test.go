package grib_test

import (
	"math"
	"testing"
	"time"

	grib "github.com/pspoerri/go-tiled-eccodes"
	"github.com/pspoerri/go-tiled-eccodes/writer"
)

// minimalField returns a small lat/lon writer.Field used by the header /
// FileOffset / DecodeFloat32 tests. Tests in the writer package have their
// own baseField helper; we duplicate a stripped-down version here so this
// file stays in the grib_test package.
func minimalField(g writer.Grid, vals []float64) writer.Field {
	return writer.Field{
		Discipline:              0,
		Centre:                  78,
		ReferenceTime:           time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC),
		ParameterCategory:       0,
		ParameterNumber:         0,
		UnitOfTimeRange:         1,
		TypeOfFirstFixedSurface: 103,
		ScaledValueFirstSurface: 2,
		Grid:                    g,
		Values:                  vals,
		NumBits:                 16,
	}
}

// TestDecodeFloat32 exercises the float32 fast-conversion path through
// the public DecodeFloat32 API. Uses an IEEE-encoded message so the
// values are bit-exact under the round-trip.
func TestDecodeFloat32(t *testing.T) {
	g := writer.NewLatLon(4, 4, 51, 6, 1, 1)
	vals := []float64{
		1, 2, 3, 4, 5, 6, 7, 8,
		9, 10, 11, 12, 13, 14, 15, 16,
	}
	f := minimalField(g, vals)
	f.Packing = writer.PackingIEEE
	f.IEEEPrecision = 1
	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	file, err := grib.FromBytes(data)
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}
	defer file.Close()

	// dst=nil → fresh buffer.
	got, err := file.Messages()[0].DecodeFloat32(nil)
	if err != nil {
		t.Fatalf("DecodeFloat32: %v", err)
	}
	for i, v := range vals {
		if float64(got[i]) != v {
			t.Errorf("got[%d] = %v, want %v", i, got[i], v)
		}
	}

	// dst reuse path — pre-allocated slice with sufficient cap.
	buf := make([]float32, 16)
	got, err = file.Messages()[0].DecodeFloat32(buf)
	if err != nil {
		t.Fatalf("DecodeFloat32 (reuse): %v", err)
	}
	if &got[0] != &buf[0] {
		t.Errorf("DecodeFloat32 should reuse the supplied buffer")
	}
}

// TestFileOffset checks that FileOffset reports a usable byte offset
// (zero for the first message in a single-message file).
func TestFileOffset(t *testing.T) {
	g := writer.NewLatLon(2, 2, 0, 0, 1, 1)
	data, err := writer.Single(minimalField(g, []float64{1, 2, 3, 4}))
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	f, err := grib.FromBytes(data)
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}
	defer f.Close()

	if got := f.Messages()[0].FileOffset(); got != 0 {
		t.Errorf("FileOffset = %d, want 0", got)
	}

	// Two-message file: the second message's offset equals the first
	// message's total length (Section 0 octets 9-16).
	twoMsg, _ := writer.Series([]writer.Field{
		minimalField(g, []float64{1, 2, 3, 4}),
		minimalField(g, []float64{5, 6, 7, 8}),
	})
	f2, _ := grib.FromBytes(twoMsg)
	defer f2.Close()
	msgs := f2.Messages()
	if len(msgs) != 2 {
		t.Fatalf("Messages = %d, want 2", len(msgs))
	}
	if msgs[1].FileOffset() <= 0 {
		t.Errorf("second message FileOffset = %d, want > 0", msgs[1].FileOffset())
	}
}

// TestSurfaceLevel covers the float-conversion helper on Header,
// including the missing-value sentinel.
func TestSurfaceLevel(t *testing.T) {
	g := writer.NewLatLon(2, 2, 0, 0, 1, 1)

	// Surface 103 (height above ground) at 2 m: scale=0, value=2 → 2.0.
	f := minimalField(g, []float64{1, 2, 3, 4})
	f.ScaleFactorFirstSurface = 0
	f.ScaledValueFirstSurface = 2
	data, _ := writer.Single(f)
	file, _ := grib.FromBytes(data)
	defer file.Close()
	if got := file.Messages()[0].Header().SurfaceLevel(); got != 2.0 {
		t.Errorf("SurfaceLevel = %v, want 2.0", got)
	}

	// Scale factor 1 → value × 10^-1 = 0.2.
	f.ScaleFactorFirstSurface = 1
	f.ScaledValueFirstSurface = 2
	data, _ = writer.Single(f)
	file2, _ := grib.FromBytes(data)
	defer file2.Close()
	if got := file2.Messages()[0].Header().SurfaceLevel(); math.Abs(got-0.2) > 1e-12 {
		t.Errorf("SurfaceLevel = %v, want 0.2", got)
	}

	// Missing-value sentinel: ScaledValue = 0xffffffff → NaN.
	f.ScaledValueFirstSurface = 0xffffffff
	data, _ = writer.Single(f)
	file3, _ := grib.FromBytes(data)
	defer file3.Close()
	if got := file3.Messages()[0].Header().SurfaceLevel(); !math.IsNaN(got) {
		t.Errorf("SurfaceLevel = %v, want NaN for missing sentinel", got)
	}
}

// TestDecodeFloat32SmallDst confirms the dst-too-small fallback path
// re-allocates rather than panicking.
func TestDecodeFloat32SmallDst(t *testing.T) {
	g := writer.NewLatLon(4, 4, 0, 0, 1, 1)
	vals := make([]float64, 16)
	for i := range vals {
		vals[i] = float64(i)
	}
	f := minimalField(g, vals)
	f.Packing = writer.PackingIEEE
	data, _ := writer.Single(f)
	file, _ := grib.FromBytes(data)
	defer file.Close()

	tiny := make([]float32, 0, 4) // cap 4 < 16 → reallocation expected
	got, err := file.Messages()[0].DecodeFloat32(tiny)
	if err != nil {
		t.Fatalf("DecodeFloat32: %v", err)
	}
	if len(got) != 16 {
		t.Errorf("len(got) = %d, want 16", len(got))
	}
}
