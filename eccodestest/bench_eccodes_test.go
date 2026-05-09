//go:build eccodes && cgo

package eccodestest

import (
	"os"
	"path/filepath"
	"testing"
)

func loadTestdata(tb testing.TB, name string) string {
	tb.Helper()
	// eccodestest lives under go-tiled-eccodes/eccodestest, so testdata is
	// one level up. Same skip-on-missing semantics as the parent package.
	p := filepath.Join("..", "testdata", name)
	if _, err := os.Stat(p); err != nil {
		tb.Skipf("fixture %s not present: %v", name, err)
	}
	return p
}

func benchEccodes(b *testing.B, name string) {
	path := loadTestdata(b, name)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := DecodeValuesEccodes(path); err != nil {
			b.Fatal(err)
		}
	}
}

// One bench per packing template. Names mirror the pure-Go BenchmarkDecode*
// suite in ../bench_test.go for easy side-by-side reading.
func BenchmarkEccodesDecodeICONSimple(b *testing.B) {
	benchEccodes(b, "icon-d2_t_2m.grib2")
}
func BenchmarkEccodesDecodeICONComplex(b *testing.B) {
	benchEccodes(b, "icon-d2_t_2m_complex.grib2")
}
func BenchmarkEccodesDecodeICONSpdiff(b *testing.B) {
	benchEccodes(b, "icon-d2_t_2m_spdiff.grib2")
}
func BenchmarkEccodesDecodeICONJPEG2000(b *testing.B) {
	benchEccodes(b, "icon-d2_t_2m_jpeg.grib2")
}
func BenchmarkEccodesDecodeICONCCSDS(b *testing.B) {
	benchEccodes(b, "icon-d2_t_2m_ccsds.grib2")
}
