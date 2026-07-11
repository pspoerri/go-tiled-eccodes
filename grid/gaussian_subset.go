package grid

import "math"

func gaussianSubset(full []float64, nj int, la1, la2 float64) []float64 {
	if nj <= 0 || len(full) == 0 {
		return nil
	}
	if nj >= len(full) {
		out := make([]float64, len(full))
		copy(out, full)
		return out
	}

	north, south := la1, la2
	if north < south {
		north, south = south, north
	}
	start := nearestGaussianRow(full, north)
	end := nearestGaussianRow(full, south)
	if start > end {
		start, end = end, start
	}
	if end-start+1 != nj {
		// Encoded endpoints are rounded to the Section 3 angular precision.
		// Anchor at the northern endpoint and trust the independently encoded Nj.
		if start+nj > len(full) {
			start = len(full) - nj
		}
		if start < 0 {
			start = 0
		}
		end = start + nj - 1
	}
	out := make([]float64, nj)
	copy(out, full[start:end+1])
	return out
}

func nearestGaussianRow(lats []float64, lat float64) int {
	best := 0
	bestDistance := math.Inf(1)
	for i, candidate := range lats {
		d := math.Abs(candidate - lat)
		if d < bestDistance {
			best, bestDistance = i, d
		}
	}
	return best
}

func reversedInts(values []int) []int {
	out := make([]int, len(values))
	for i, value := range values {
		out[len(values)-1-i] = value
	}
	return out
}

func prefixSums(widths []int) []int {
	out := make([]int, len(widths)+1)
	for i, width := range widths {
		out[i+1] = out[i] + width
	}
	return out
}

func (g Gaussian) maximumRowWidth() int {
	if !g.Reduced {
		return g.Ni
	}
	maxWidth := 0
	for _, width := range g.PL {
		if width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
}

// longitudeLayout resolves a natural west-to-east row layout. Reduced global
// grids omit Di, while regional Gaussian grids use Lo1/Lo2 and sometimes Di.
func (g Gaussian) longitudeLayout(rowWidth int) (west, east, di float64, global bool) {
	if rowWidth <= 0 {
		return 0, 0, 0, false
	}
	west, east = g.Lo1, g.Lo2
	if !g.IPositive {
		west, east = east, west
	}
	for east < west {
		east += 360
	}
	span := east - west

	if g.Di > 0 {
		di = g.Di
		global = math.Abs(di*float64(rowWidth)-360) < math.Max(1e-6, di*0.1)
		if global {
			east = west + 360 - di
		}
		return
	}

	if rowWidth == 1 {
		return west, west, 0, false
	}
	maxWidth := g.maximumRowWidth()
	if maxWidth > 1 {
		nominal := 360 / float64(maxWidth)
		tolerance := math.Max(1e-6, nominal*0.1)
		if math.Abs(span+nominal-360) < tolerance {
			di = 360 / float64(rowWidth)
			east = west + 360 - di
			return west, east, di, true
		}
	}
	di = span / float64(rowWidth-1)
	return west, east, di, false
}
