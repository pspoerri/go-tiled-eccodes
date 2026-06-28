package grib_test

import (
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
	"github.com/pspoerri/go-tiled-eccodes/tile"
)

// BenchmarkDecodeICON decodes the full 1215x746 ICON-D2 t_2m field, including
// the bitmap fan-out. This is the cold-cache path: each iteration reopens the
// file so sync.Once is reset.
func BenchmarkDecodeICONCold(b *testing.B) {
	path := loadTestdata(b, "icon-d2_t_2m.grib2")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, err := grib.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		_, err = f.Messages()[0].DecodeFloat64(nil)
		if err != nil {
			b.Fatal(err)
		}
		f.Close()
	}
}

func BenchmarkDecodeICONComplex(b *testing.B) {
	path := loadTestdata(b, "icon-d2_t_2m_complex.grib2")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, _ := grib.Open(path)
		_, err := f.Messages()[0].DecodeFloat64(nil)
		if err != nil {
			b.Fatal(err)
		}
		f.Close()
	}
}

func BenchmarkDecodeICONSpdiff(b *testing.B) {
	path := loadTestdata(b, "icon-d2_t_2m_spdiff.grib2")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, _ := grib.Open(path)
		_, err := f.Messages()[0].DecodeFloat64(nil)
		if err != nil {
			b.Fatal(err)
		}
		f.Close()
	}
}

func BenchmarkDecodeICONJPEG2000(b *testing.B) {
	path := loadTestdata(b, "icon-d2_t_2m_jpeg.grib2")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, _ := grib.Open(path)
		_, err := f.Messages()[0].DecodeFloat64(nil)
		if err != nil {
			b.Fatal(err)
		}
		f.Close()
	}
}

func BenchmarkDecodeICONCCSDS(b *testing.B) {
	path := loadTestdata(b, "icon-d2_t_2m_ccsds.grib2")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, _ := grib.Open(path)
		_, err := f.Messages()[0].DecodeFloat64(nil)
		if err != nil {
			b.Fatal(err)
		}
		f.Close()
	}
}

func BenchmarkRenderTile256BicubicWarm(b *testing.B) {
	path := loadTestdata(b, "icon-d2_t_2m.grib2")
	f, err := grib.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	m := f.Messages()[0]
	// warm the cache once
	if _, err := m.DecodeFloat64(nil); err != nil {
		b.Fatal(err)
	}
	dst := make([]float32, 256*256)
	req := grib.TileRequest{
		Tile:   tile.XYZ{Z: 5, X: 17, Y: 10},
		Width:  256,
		Height: 256,
		Sample: tile.Bicubic,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := m.RenderFloat32(req, dst); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecodeNaturalWarm measures the natural-order reorder over the warm
// cache on the 1215x746 icon-d2 grid (S→N scan, so every read goes through the
// Index reorder path). BenchmarkDecodeFloat32Warm is the straight-copy baseline
// over the same cache — the delta is the cost of un-scrambling the scan.
func BenchmarkDecodeNaturalWarm(b *testing.B) {
	path := loadTestdata(b, "icon-d2_t_2m.grib2")
	f, err := grib.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	m := f.Messages()[0]
	if _, err := m.DecodeFloat64(nil); err != nil { // warm the cache
		b.Fatal(err)
	}
	dst := make([]float32, 1215*746)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := m.DecodeNaturalFloat32(dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeFloat32Warm(b *testing.B) {
	path := loadTestdata(b, "icon-d2_t_2m.grib2")
	f, err := grib.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	m := f.Messages()[0]
	if _, err := m.DecodeFloat64(nil); err != nil { // warm the cache
		b.Fatal(err)
	}
	dst := make([]float32, 1215*746)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := m.DecodeFloat32(dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValueAtWarm(b *testing.B) {
	path := loadTestdata(b, "icon-d2_t_2m.grib2")
	f, err := grib.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	m := f.Messages()[0]
	if _, err := m.DecodeFloat64(nil); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := m.ValueAt(50.11, 8.68, tile.Bicubic); err != nil {
			b.Fatal(err)
		}
	}
}
