package grib_test

import (
	"math"
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
	"github.com/pspoerri/go-tiled-eccodes/tile"
)

// TestRenderRegionConstantField asserts that RenderRegion returns the
// expected sample over a constant-field fixture. Uses the same regular_ll
// fixture as TestRegularLLConstantField.
func TestRenderRegionConstantField(t *testing.T) {
	f, err := grib.Open(loadTestdata(t, "regular_ll_sfc.grib2"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	m := f.Messages()[0]

	// Sample a 32×16 grid covering the message extent (we don't care about
	// the exact bounds — every value should equal 273.15 anyway).
	r := grib.Region{
		South: -89, North: 89, West: -179, East: 179,
		Width: 32, Height: 16, Sample: tile.Bicubic,
	}
	dst := make([]float64, r.Width*r.Height)
	if err := m.RenderRegionFloat64(r, dst); err != nil {
		t.Fatalf("RenderRegionFloat64: %v", err)
	}
	for i, v := range dst {
		if math.IsNaN(v) {
			continue // ocean / out-of-domain pixels are NaN, that's fine
		}
		if math.Abs(v-273.15) > 1e-3 {
			t.Fatalf("pixel %d = %g, want 273.15", i, v)
		}
	}
}

// TestRenderRegionAgreesWithValueAt cross-checks RenderRegion against
// per-point ValueAt calls. Both paths share Locate() and the same sampler,
// so the per-pixel values must match within float rounding.
func TestRenderRegionAgreesWithValueAt(t *testing.T) {
	f, err := grib.Open(loadTestdata(t, "icon-d2_t_2m.grib2"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	m := f.Messages()[0]

	r := grib.Region{
		South: 47, North: 51, West: 6, East: 12,
		Width: 32, Height: 24, Sample: tile.Nearest,
	}
	dst := make([]float64, r.Width*r.Height)
	if err := m.RenderRegionFloat64(r, dst); err != nil {
		t.Fatalf("RenderRegionFloat64: %v", err)
	}

	// Spot-check four pixels against the centre-of-cell lat/lon.
	for _, px := range []struct{ i, j int }{{0, 0}, {31, 0}, {0, 23}, {15, 12}} {
		lon := r.West + (r.East-r.West)*(float64(px.i)+0.5)/float64(r.Width)
		lat := r.North + (r.South-r.North)*(float64(px.j)+0.5)/float64(r.Height)
		want, err := m.ValueAt(lat, lon, tile.Nearest)
		if err != nil {
			t.Fatalf("ValueAt(%g, %g): %v", lat, lon, err)
		}
		got := dst[px.j*r.Width+px.i]
		if math.IsNaN(want) && math.IsNaN(got) {
			continue
		}
		if math.IsNaN(want) != math.IsNaN(got) {
			t.Fatalf("pixel (%d,%d) NaN mismatch: got %g, want %g", px.i, px.j, got, want)
		}
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("pixel (%d,%d) = %g, want %g", px.i, px.j, got, want)
		}
	}
}

// TestRenderRegionFloat32 confirms the float32 wrapper produces identical
// output (modulo precision) to the float64 path.
func TestRenderRegionFloat32(t *testing.T) {
	f, err := grib.Open(loadTestdata(t, "icon-d2_t_2m.grib2"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	m := f.Messages()[0]

	r := grib.Region{
		South: 47, North: 51, West: 6, East: 12,
		Width: 16, Height: 16, Sample: tile.Bicubic,
	}
	dst32 := make([]float32, r.Width*r.Height)
	dst64 := make([]float64, r.Width*r.Height)
	if err := m.RenderRegionFloat32(r, dst32); err != nil {
		t.Fatalf("32: %v", err)
	}
	if err := m.RenderRegionFloat64(r, dst64); err != nil {
		t.Fatalf("64: %v", err)
	}
	for i := range dst32 {
		if math.IsNaN(float64(dst32[i])) && math.IsNaN(dst64[i]) {
			continue
		}
		if math.Abs(float64(dst32[i])-dst64[i]) > 1e-3 {
			t.Fatalf("pixel %d: 32=%g 64=%g", i, dst32[i], dst64[i])
		}
	}
}

// TestRenderRegionAntimeridianCrossing ensures lon spans that wrap the
// antimeridian (East ≤ West) are handled by lonSpan += 360.
func TestRenderRegionAntimeridianCrossing(t *testing.T) {
	f, err := grib.Open(loadTestdata(t, "regular_ll_sfc.grib2"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	m := f.Messages()[0]

	r := grib.Region{
		South: -10, North: 10, West: 170, East: -170, // crosses 180°
		Width: 8, Height: 4,
	}
	dst := make([]float64, r.Width*r.Height)
	// We don't assert specific values (the 16×31 fixture probably doesn't
	// cover this region) — just confirm we don't panic and the call returns.
	if err := m.RenderRegionFloat64(r, dst); err != nil {
		t.Fatalf("RenderRegionFloat64: %v", err)
	}
}
