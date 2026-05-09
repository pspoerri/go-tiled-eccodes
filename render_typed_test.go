package grib_test

import (
	"math"
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
	"github.com/pspoerri/go-tiled-eccodes/tile"
	"github.com/pspoerri/go-tiled-eccodes/writer"
)

// renderTestMessage builds an in-memory GRIB file with a small, predictable
// regular lat/lon field. Returns the open file (caller must Close) and its
// single message.
func renderTestMessage(t *testing.T) (*grib.File, *grib.Message) {
	t.Helper()
	g := writer.NewLatLon(8, 8, 4, 0, 1, 1)
	vals := make([]float64, 64)
	for i := range vals {
		vals[i] = 273.15 + float64(i)/10
	}
	f := minimalField(g, vals)
	f.Packing = writer.PackingIEEE
	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	file, err := grib.FromBytes(data)
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}
	return file, file.Messages()[0]
}

// regionRequest covers the 4° × 4° box centered on the synthetic field's
// origin. Plate-Carrée sampling at the same density gives a 1:1 mapping
// from output cell to source cell at the centre of the region.
func regionRequest() grib.Region {
	return grib.Region{
		South: 0, West: 0, North: 4, East: 4,
		Width: 4, Height: 4,
		Sample: tile.Bicubic,
	}
}

// TestRenderInt typed integer renderers go through the same float64 source
// pipeline; the test fixture is a small region that sits inside the grid so
// every output cell holds a finite value.
func TestRenderTypedInt(t *testing.T) {
	file, m := renderTestMessage(t)
	defer file.Close()

	tl := grib.TileRequest{Tile: tile.XYZ{Z: 4, X: 8, Y: 7}, Width: 4, Height: 4, Sample: tile.Nearest}

	q := tile.Quantize{Scale: 10, Offset: 273, Min: -100, Max: 100, MissingValue: -1}
	int8Buf := make([]int8, 16)
	if err := m.RenderInt8(tl, q, int8Buf); err != nil {
		t.Errorf("RenderInt8: %v", err)
	}
	int16Buf := make([]int16, 16)
	if err := m.RenderInt16(tl, q, int16Buf); err != nil {
		t.Errorf("RenderInt16: %v", err)
	}
	int32Buf := make([]int32, 16)
	if err := m.RenderInt32(tl, q, int32Buf); err != nil {
		t.Errorf("RenderInt32: %v", err)
	}
	int64Buf := make([]int64, 16)
	if err := m.RenderInt64(tl, q, int64Buf); err != nil {
		t.Errorf("RenderInt64: %v", err)
	}

	// Same value should appear (modulo type width) across all integer
	// renderers — the quantize formula is identical.
	if int16(int8Buf[0]) != int16Buf[0] && math.Abs(float64(int16Buf[0])-float64(int8Buf[0])) > 100 {
		t.Errorf("int8 vs int16 mismatch: %d vs %d", int8Buf[0], int16Buf[0])
	}
}

func TestRenderTypedUint(t *testing.T) {
	file, m := renderTestMessage(t)
	defer file.Close()

	tl := grib.TileRequest{Tile: tile.XYZ{Z: 4, X: 8, Y: 7}, Width: 4, Height: 4, Sample: tile.Nearest}

	q := tile.Quantize{Scale: 10, Offset: 273, Min: 0, Max: 1000, MissingValue: 0}
	u8 := make([]uint8, 16)
	if err := m.RenderUint8(tl, q, u8); err != nil {
		t.Errorf("RenderUint8: %v", err)
	}
	u16 := make([]uint16, 16)
	if err := m.RenderUint16(tl, q, u16); err != nil {
		t.Errorf("RenderUint16: %v", err)
	}
	u32 := make([]uint32, 16)
	if err := m.RenderUint32(tl, q, u32); err != nil {
		t.Errorf("RenderUint32: %v", err)
	}
	u64 := make([]uint64, 16)
	if err := m.RenderUint64(tl, q, u64); err != nil {
		t.Errorf("RenderUint64: %v", err)
	}
}

// TestRenderShortBuffer covers the early ErrShortBuffer return that every
// typed renderer shares.
func TestRenderShortBuffer(t *testing.T) {
	file, m := renderTestMessage(t)
	defer file.Close()
	tl := grib.TileRequest{Tile: tile.XYZ{Z: 4, X: 8, Y: 7}, Width: 4, Height: 4}
	q := tile.Quantize{Scale: 1}
	if err := m.RenderInt8(tl, q, make([]int8, 1)); err == nil {
		t.Errorf("RenderInt8 short buffer: nil error")
	}
	if err := m.RenderUint16(tl, q, make([]uint16, 1)); err == nil {
		t.Errorf("RenderUint16 short buffer: nil error")
	}
}

// TestRenderQuantizeNaN ensures NaN inputs (out-of-bounds region) produce
// the configured MissingValue sentinel in the integer output.
func TestRenderQuantizeNaN(t *testing.T) {
	file, m := renderTestMessage(t)
	defer file.Close()

	// Tile that lies entirely outside our 8×8 lat/lon block (west of 0°).
	// Every output cell should miss the grid → NaN → MissingValue.
	tl := grib.TileRequest{Tile: tile.XYZ{Z: 4, X: 0, Y: 0}, Width: 4, Height: 4}
	q := tile.Quantize{Scale: 1, MissingValue: 42}
	dst := make([]int8, 16)
	if err := m.RenderInt8(tl, q, dst); err != nil {
		t.Fatalf("RenderInt8: %v", err)
	}
	for i, v := range dst {
		if v != 42 {
			t.Errorf("dst[%d] = %d, want MissingValue 42", i, v)
		}
	}
}
