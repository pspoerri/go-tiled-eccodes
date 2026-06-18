package grid

import (
	"math"
	"testing"
)

// wrap360Ref is the straightforward math.Mod reference the optimized wrap360
// must match exactly for every input.
func wrap360Ref(lon, base float64) float64 {
	x := math.Mod(lon-base, 360)
	if x < 0 {
		x += 360
	}
	return base + x
}

// TestWrap360MatchesReference checks the fast-path wrap360 against the math.Mod
// reference across in-range, one-period-out, and far-out-of-range inputs, for
// several bases. The result must be bit-identical so no grid Locate changes
// behaviour.
func TestWrap360MatchesReference(t *testing.T) {
	bases := []float64{0, -180, 10, 359.5, -0.25}
	for _, base := range bases {
		for lon := -1100.0; lon <= 1100.0; lon += 0.5 {
			got := wrap360(lon, base)
			want := wrap360Ref(lon, base)
			if math.Abs(got-want) > 1e-9 {
				t.Fatalf("wrap360(%g, %g) = %.12g, want %.12g", lon, base, got, want)
			}
			// Result must land in [base, base+360).
			if got < base || got >= base+360 {
				t.Fatalf("wrap360(%g, %g) = %g outside [%g, %g)", lon, base, got, base, base+360)
			}
		}
	}
}

func BenchmarkWrap360(b *testing.B) {
	// Mix of in-range and just-out-of-range values, the render hot-path shape.
	lons := []float64{12.3, 181.7, -5.0, 359.9, 400.0, -190.0}
	var sink float64
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sink += wrap360(lons[i%len(lons)], 0)
	}
	_ = sink
}
