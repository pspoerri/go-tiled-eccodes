package sample

import (
	"math"
	"testing"
)

// linearSrc returns f(i,j) = i + 10*j on a 10x10 grid. Bicubic should
// reproduce the linear field exactly (within rounding) since Catmull-Rom is
// linear-preserving.
func linearSrc() Source {
	return func(i, j int) float64 {
		if i < 0 || i > 9 || j < 0 || j > 9 {
			return math.NaN()
		}
		return float64(i) + 10*float64(j)
	}
}

func TestNearest(t *testing.T) {
	src := linearSrc()
	fx := []float64{1.4, 1.6}
	fy := []float64{2.4, 2.6}
	dst := make([]float64, 2)
	Resample(Nearest, src, fx, fy, dst, 0)
	if dst[0] != 1+10*2 {
		t.Errorf("nearest 1.4/2.4 = %g, want 21", dst[0])
	}
	if dst[1] != 2+10*3 {
		t.Errorf("nearest 1.6/2.6 = %g, want 32", dst[1])
	}
}

func TestBicubicPreservesLinear(t *testing.T) {
	src := linearSrc()
	fx := []float64{4.3, 4.5, 4.7}
	fy := []float64{3.1, 3.5, 3.9}
	dst := make([]float64, len(fx))
	Resample(Bicubic, src, fx, fy, dst, 0)
	for i := range fx {
		want := fx[i] + 10*fy[i]
		if math.Abs(dst[i]-want) > 1e-6 {
			t.Errorf("[%d] got %g want %g (Catmull-Rom should reproduce linear)", i, dst[i], want)
		}
	}
}

func TestModeFilter(t *testing.T) {
	// Source: 9 cells in a 3x3, with 5 of them = 7, others 1, 2, 3, 4.
	src := func(i, j int) float64 {
		if i < 0 || i > 2 || j < 0 || j > 2 {
			return math.NaN()
		}
		grid := [3][3]float64{
			{7, 7, 7},
			{1, 7, 2},
			{3, 7, 4},
		}
		return grid[j][i]
	}
	fx := []float64{1}
	fy := []float64{1}
	dst := make([]float64, 1)
	Resample(ModeFilter, src, fx, fy, dst, 1) // 3x3 window
	if dst[0] != 7 {
		t.Errorf("mode = %g, want 7", dst[0])
	}
}
