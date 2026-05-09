package grib_test

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	grib "github.com/pspoerri/go-tiled-eccodes"
	"github.com/pspoerri/go-tiled-eccodes/tile"
	"github.com/pspoerri/go-tiled-eccodes/writer"
)

// Regression: exercise the full reader/writer pipeline end-to-end. The
// writer package's own tests use grib.FromBytes; this file goes through
// grib.Open so the mmap I/O path is also covered. It encodes a small
// multi-message file with mixed projections and bundled fields, writes it
// to disk, reopens it, and checks values, headers, ValueAt, and tile +
// region renders. A regression in either direction (writer or reader)
// will trip this test.

func TestWriterReaderRoundTrip(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "synthetic.grib2")

	tref := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)

	// Three groups, each one a physical message:
	//  1. LatLon (ICON-Global-style), bundled with two variables.
	//  2. RotatedLatLon (ICON-CH-style), single field, T+1h.
	//  3. LatLon again, bitmap with NaN holes, T+2h.
	llGlobal := writer.NewLatLon(60, 31, 75, -30, 1, 1)
	tempGlobal := linear(llGlobal.Ni, llGlobal.Nj, 270)
	humGlobal := linear(llGlobal.Ni, llGlobal.Nj, 50)

	llRot := writer.NewRotatedLatLon(40, 30, 2, -2, 0.05, 0.05, -43, 10)
	tempRot := linear(llRot.Ni, llRot.Nj, 280)

	llHoles := writer.NewLatLon(8, 6, 50, 0, 1, 1)
	tempHoles := linear(llHoles.Ni, llHoles.Nj, 290)
	tempHoles[5] = math.NaN()
	tempHoles[20] = math.NaN()

	mk := func(g writer.Grid, vals []float64, t0 time.Time, hours int, cat, num uint8) writer.Field {
		return writer.Field{
			Discipline:              0,
			Centre:                  78,
			ReferenceTime:           t0,
			ParameterCategory:       cat,
			ParameterNumber:         num,
			UnitOfTimeRange:         1,
			ForecastTime:            int32(hours),
			TypeOfFirstFixedSurface: 103,
			ScaledValueFirstSurface: 2,
			Grid:                    g,
			Values:                  vals,
			NumBits:                 16,
		}
	}

	groups := [][]writer.Field{
		{
			mk(llGlobal, tempGlobal, tref, 0, 0, 0),
			mk(llGlobal, humGlobal, tref, 0, 1, 1),
		},
		{mk(llRot, tempRot, tref, 1, 0, 0)},
		{mk(llHoles, tempHoles, tref, 2, 0, 0)},
	}
	data, err := writer.EncodeFile(groups)
	if err != nil {
		t.Fatalf("EncodeFile: %v", err)
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Reopen via the mmap path.
	f, err := grib.Open(tmp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	msgs := f.Messages()
	if len(msgs) != 4 {
		t.Fatalf("messages = %d, want 4 (2 bundled + 1 + 1)", len(msgs))
	}

	// --- msg 0: bundled temperature, LatLon ---
	h := msgs[0].Header()
	if h.GridTemplate != 0 || h.Ni != 60 || h.Nj != 31 ||
		h.ParameterCategory != 0 || h.ParameterNumber != 0 ||
		!h.ReferenceTime.Equal(tref) {
		t.Errorf("msg0 header wrong: %+v", h)
	}
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("msg0 decode: %v", err)
	}
	checkValuesClose(t, "msg0", got, tempGlobal, 0.01)

	// --- msg 1: bundled humidity, same grid + ref time, different param ---
	h = msgs[1].Header()
	if h.GridTemplate != 0 || h.ParameterCategory != 1 || h.ParameterNumber != 1 ||
		!h.ReferenceTime.Equal(tref) {
		t.Errorf("msg1 header wrong: %+v", h)
	}

	// --- msg 2: rotated lat/lon, T+1h ---
	h = msgs[2].Header()
	if h.GridTemplate != 1 || int(h.ForecastTime) != 1 {
		t.Errorf("msg2 header wrong: %+v", h)
	}

	// --- msg 3: bitmap with holes, T+2h ---
	h = msgs[3].Header()
	if int(h.ForecastTime) != 2 {
		t.Errorf("msg3 forecast = %d, want 2", h.ForecastTime)
	}
	got, err = msgs[3].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("msg3 decode: %v", err)
	}
	if !math.IsNaN(got[5]) || !math.IsNaN(got[20]) {
		t.Errorf("msg3 bitmap holes not preserved: got[5]=%v got[20]=%v", got[5], got[20])
	}
	checkValuesClose(t, "msg3", got, tempHoles, 0.02)

	// --- ValueAt: spot-check msg 0 inside its global domain ---
	v, err := msgs[0].ValueAt(60, 0, tile.Bicubic)
	if err != nil || math.IsNaN(v) || v < 260 || v > 290 {
		t.Errorf("ValueAt(60,0) on msg0 = %v err=%v, want in 260..290", v, err)
	}

	// --- Tile render: a Web-Mercator tile that overlaps msg 0's domain ---
	req := grib.TileRequest{
		Tile:   tile.XYZ{Z: 3, X: 4, Y: 2},
		Width:  64,
		Height: 64,
		Sample: tile.Bicubic,
	}
	dst32 := make([]float32, 64*64)
	if err := msgs[0].RenderFloat32(req, dst32); err != nil {
		t.Fatalf("RenderFloat32: %v", err)
	}
	var valid int
	for _, v := range dst32 {
		if !math.IsNaN(float64(v)) {
			valid++
		}
	}
	if valid == 0 {
		t.Errorf("RenderFloat32 produced zero valid pixels — domain mismatch?")
	}

	// --- Region render: a bbox cleanly inside msg 0's domain ---
	// llGlobal spans 45°N..75°N × -30°E..29°E (60×31 cells, 1° step).
	region := grib.Region{
		South: 50, West: -20, North: 70, East: 20,
		Width: 32, Height: 24,
		Sample: tile.Bicubic,
	}
	dst64 := make([]float64, region.Width*region.Height)
	if err := msgs[0].RenderRegionFloat64(region, dst64); err != nil {
		t.Fatalf("RenderRegionFloat64: %v", err)
	}
	for i, v := range dst64 {
		if math.IsNaN(v) || v < 240 || v > 320 {
			t.Errorf("region[%d] = %v out of expected range", i, v)
			break
		}
	}
}

func linear(ni, nj int, base float64) []float64 {
	out := make([]float64, ni*nj)
	for j := 0; j < nj; j++ {
		for i := 0; i < ni; i++ {
			out[j*ni+i] = base + float64(j)*0.1 + float64(i)*0.01
		}
	}
	return out
}

func checkValuesClose(t *testing.T, label string, got, want []float64, tol float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: len mismatch: got %d, want %d", label, len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		switch {
		case math.IsNaN(w) && math.IsNaN(g):
			continue
		case math.IsNaN(w) != math.IsNaN(g):
			t.Fatalf("%s: vals[%d] = %v, want %v (NaN mismatch)", label, i, g, w)
		case math.Abs(g-w) > tol:
			t.Fatalf("%s: vals[%d] = %v, want %v (diff %v > %v)", label, i, g, w, math.Abs(g-w), tol)
		}
	}
}
