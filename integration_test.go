package grib_test

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
	gridpkg "github.com/pspoerri/go-tiled-eccodes/grid"
	"github.com/pspoerri/go-tiled-eccodes/tile"
)

// loadTestdata returns the path to a fixture file under testdata/, skipping
// the test if the file is absent (it may be downloaded out-of-band).
func loadTestdata(tb testing.TB, name string) string {
	tb.Helper()
	p := filepath.Join("testdata", name)
	if _, err := os.Stat(p); err != nil {
		tb.Skipf("fixture %s not present: %v", name, err)
	}
	return p
}

func TestRegularLLConstantField(t *testing.T) {
	// regular_ll_sfc_grib2.tmpl is a 16x31 regular lat/lon grid with simple
	// packing, bitsPerValue=0 — every value equals the reference (273.15 K).
	f, err := grib.Open(loadTestdata(t, "regular_ll_sfc.grib2"))
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
	if h.Ni != 16 || h.Nj != 31 {
		t.Fatalf("dims = %dx%d, want 16x31", h.Ni, h.Nj)
	}

	vals, err := m.DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(vals) != 16*31 {
		t.Fatalf("decoded len = %d, want 496", len(vals))
	}
	for i, v := range vals {
		// reference value is stored as IEEE float32 — relax tolerance accordingly.
		if math.Abs(v-273.15) > 1e-4 {
			t.Fatalf("vals[%d] = %g, want 273.15", i, v)
		}
	}
}

func TestICOND2T2m(t *testing.T) {
	// Real DWD ICON-D2 t_2m forecast (regular lat/lon, simple packing,
	// 16-bit values, bitmap of valid points).
	f, err := grib.Open(loadTestdata(t, "icon-d2_t_2m.grib2"))
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
	if h.Ni != 1215 || h.Nj != 746 {
		t.Fatalf("dims = %dx%d, want 1215x746", h.Ni, h.Nj)
	}

	vals, err := m.DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := len(vals); got != 1215*746 {
		t.Fatalf("decoded len = %d, want %d", got, 1215*746)
	}

	// Sanity on value range: T2m for Central Europe in May should be roughly
	// 240–310 K (with some NaN ocean points). Compute min/max/mean over the
	// non-NaN values.
	var minV, maxV float64 = math.Inf(1), math.Inf(-1)
	var sum float64
	var n int
	for _, v := range vals {
		if math.IsNaN(v) {
			continue
		}
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
		sum += v
		n++
	}
	if n == 0 {
		t.Fatalf("all values NaN")
	}
	mean := sum / float64(n)
	t.Logf("ICON-D2 t_2m: n=%d nan=%d min=%.2f max=%.2f mean=%.2f", n, len(vals)-n, minV, maxV, mean)
	if minV < 200 || maxV > 330 {
		t.Fatalf("T2m range looks wrong: [%.2f .. %.2f] K", minV, maxV)
	}
	if mean < 250 || mean > 305 {
		t.Fatalf("T2m mean looks wrong: %.2f K", mean)
	}

	// Spot-check a known location: Frankfurt (50.11°N, 8.68°E).
	// Just verify it returns a sensible value via ValueAt.
	if v, err := m.ValueAt(50.11, 8.68, tile.Bicubic); err != nil {
		t.Fatalf("ValueAt Frankfurt: %v", err)
	} else if math.IsNaN(v) || v < 240 || v > 320 {
		t.Fatalf("ValueAt Frankfurt = %g, expected 240..320 K", v)
	}

	// Render a 256x256 tile that covers Germany. Web-Mercator tile z=5,x=16,y=10
	// roughly covers central Europe; let's pick z=5 x=17 y=10 (south-central
	// Germany).
	req := grib.TileRequest{
		Tile:   tile.XYZ{Z: 5, X: 17, Y: 10},
		Width:  256,
		Height: 256,
		Sample: tile.Bicubic,
	}
	dst := make([]float32, 256*256)
	if err := m.RenderFloat32(req, dst); err != nil {
		t.Fatalf("render: %v", err)
	}
	// At least 25% of the tile should be inside the ICON domain (the rest
	// will be NaN where the tile escapes Germany).
	var valid int
	for _, v := range dst {
		if !math.IsNaN(float64(v)) {
			valid++
		}
	}
	if valid < 256*256/4 {
		t.Fatalf("tile has too few valid pixels: %d", valid)
	}

	// Verify the LatLon grid bounds are what we expect.
	g, err := m.Grid()
	if err != nil {
		t.Fatalf("grid: %v", err)
	}
	ll, ok := g.(gridpkg.LatLon)
	if !ok {
		t.Fatalf("grid type %T, want LatLon", g)
	}
	if math.Abs(ll.Di-0.02) > 1e-9 || math.Abs(ll.Dj-0.02) > 1e-9 {
		t.Fatalf("Di/Dj = %g/%g, want 0.02", ll.Di, ll.Dj)
	}
}
