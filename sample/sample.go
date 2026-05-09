// Package sample implements resampling kernels driven by the renderer.
//
// Each kernel walks the destination pixels (the tile is tiny — 256² fits in
// L2) and reads the source values it needs through a Source closure. The
// renderer supplies that closure with the decoded grid + Grid.Index, so the
// kernels are independent of the source grid layout.
package sample

import "math"

// Source returns the value at integer source pixel (i, j), or NaN for
// missing/out-of-bounds points. Implementations should clamp at the edges.
type Source func(i, j int) float64

// Mode selects which kernel to run.
type Mode uint8

const (
	Nearest Mode = iota
	Bicubic
	ModeFilter
)

// Resample fills dst with W*H values sampled at the fractional source
// coordinates (fx[k], fy[k]) for pixel k. The Mode picks the kernel.
//
// fx and fy must each have len = W*H (or be width-W, height-H plane-separable
// — see RenderSeparable for the common path).
func Resample(mode Mode, src Source, fx, fy []float64, dst []float64, modeWindow int) {
	switch mode {
	case Nearest:
		nearest(src, fx, fy, dst)
	case Bicubic:
		bicubic(src, fx, fy, dst)
	case ModeFilter:
		modeF(src, fx, fy, dst, modeWindow)
	}
}

func nearest(src Source, fx, fy []float64, dst []float64) {
	for k := range dst {
		i := int(math.Round(fx[k]))
		j := int(math.Round(fy[k]))
		dst[k] = src(i, j)
	}
}

// Catmull-Rom cubic kernel (a = -0.5).
//
//	|t| < 1:    1.5*|t|³ - 2.5*|t|² + 1
//	1 ≤ |t| < 2: -0.5*|t|³ + 2.5*|t|² - 4*|t| + 2
//	|t| ≥ 2:    0
func cr(t float64) float64 {
	at := t
	if at < 0 {
		at = -at
	}
	if at < 1 {
		return ((1.5*at-2.5)*at)*at + 1
	}
	if at < 2 {
		return ((-0.5*at+2.5)*at-4)*at + 2
	}
	return 0
}

func bicubic(src Source, fx, fy []float64, dst []float64) {
	var w [4]float64
	var u [4]float64
	for k := range dst {
		x := fx[k]
		y := fy[k]
		ix := int(math.Floor(x))
		iy := int(math.Floor(y))
		dx := x - float64(ix)
		dy := y - float64(iy)
		w[0] = cr(1 + dx)
		w[1] = cr(dx)
		w[2] = cr(1 - dx)
		w[3] = cr(2 - dx)
		u[0] = cr(1 + dy)
		u[1] = cr(dy)
		u[2] = cr(1 - dy)
		u[3] = cr(2 - dy)

		// Accumulate over the 4x4 stencil. NaN-aware: if any contributing
		// source is NaN, fall back to nearest to avoid poisoning the output.
		var sum, wsum float64
		nanSeen := false
		for jj := 0; jj < 4; jj++ {
			for ii := 0; ii < 4; ii++ {
				v := src(ix-1+ii, iy-1+jj)
				if math.IsNaN(v) {
					nanSeen = true
					break
				}
				ww := w[ii] * u[jj]
				sum += v * ww
				wsum += ww
			}
			if nanSeen {
				break
			}
		}
		if nanSeen || wsum == 0 {
			dst[k] = src(int(math.Round(x)), int(math.Round(y)))
			continue
		}
		dst[k] = sum / wsum
	}
}

// modeF picks the most-frequent value within a (2*half+1)² square window
// centred on the source pixel. For continuous data this is meaningless; for
// categorical fields (cloud type, precipitation type) it preserves class
// boundaries that bicubic would smear.
//
// The histogram is built per-pixel in a small map; the window is intended to
// be small (3x3 or 5x5). Ties are broken by lowest value.
func modeF(src Source, fx, fy []float64, dst []float64, half int) {
	if half < 1 {
		half = 1
	}
	hist := make(map[float64]int, (2*half+1)*(2*half+1))
	for k := range dst {
		ci := int(math.Round(fx[k]))
		cj := int(math.Round(fy[k]))
		for k := range hist {
			delete(hist, k)
		}
		for jj := -half; jj <= half; jj++ {
			for ii := -half; ii <= half; ii++ {
				v := src(ci+ii, cj+jj)
				if math.IsNaN(v) {
					continue
				}
				hist[v]++
			}
		}
		var best float64
		bestN := -1
		for v, n := range hist {
			if n > bestN || (n == bestN && v < best) {
				best = v
				bestN = n
			}
		}
		if bestN < 0 {
			dst[k] = math.NaN()
		} else {
			dst[k] = best
		}
	}
}
