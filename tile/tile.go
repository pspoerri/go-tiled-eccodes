// Package tile maps WGS84 XYZ web-map tiles to lat/lon grids and renders
// them into typed buffers using a configurable sampler.
//
// XYZ convention: z = zoom level, x = column (0..2^z-1, west→east), y = row
// (0..2^z-1, north→south). This is the "spherical Mercator" / "Google" /
// OSM convention; the standard slippy-map URL tile layout.
package tile

import "math"

// XYZ identifies one slippy-map tile.
type XYZ struct{ Z, X, Y int }

// SampleMode picks the resampler used by the renderer.
type SampleMode uint8

const (
	Nearest SampleMode = iota
	Bicubic
	Mode
)

// Quantize describes the float64 → integer conversion used by integer-typed
// renderers. out = clamp(round((v - Offset) * Scale), Min, Max). NaN inputs
// (missing values) are written as MissingValue.
type Quantize struct {
	Scale, Offset float64
	Min, Max      float64
	MissingValue  int64 // typed sentinel cast on output
}

// Bounds returns the WGS84 (south, west, north, east) extent of an XYZ tile
// in degrees. The returned latitudes are clamped to the spherical-Mercator
// extent (~±85.0511°).
func (t XYZ) Bounds() (south, west, north, east float64) {
	n := float64(int(1) << uint(t.Z))
	west = float64(t.X)/n*360 - 180
	east = float64(t.X+1)/n*360 - 180
	north = mercY(float64(t.Y) / n)
	south = mercY(float64(t.Y+1) / n)
	return
}

// mercY converts a normalised tile y (0..1, north→south) to latitude in deg.
func mercY(yn float64) float64 {
	t := math.Pi - 2*math.Pi*yn
	return 180 / math.Pi * math.Atan(0.5*(math.Exp(t)-math.Exp(-t)))
}

// Pixel returns the WGS84 lat/lon of the pixel at (px, py) within a tile of
// width w and height h. (0,0) is the top-left (north-west) corner.
func (t XYZ) Pixel(px, py, w, h int) (lat, lon float64) {
	south, west, north, east := t.Bounds()
	lon = west + (east-west)*(float64(px)+0.5)/float64(w)
	// Linear in normalised tile y (preserves Mercator within one tile is too
	// expensive per pixel — the visual error is sub-pixel for typical zooms).
	// Trade documented in plan.md.
	yn := (float64(py) + 0.5) / float64(h)
	lat = north + (south-north)*yn
	_ = south
	_ = north
	return
}

// LatLonGrid pre-computes the (lat, lon) of every output pixel for a tile
// once. Renderers iterate over this in their hot loop instead of recomputing
// the projection per pixel.
type LatLonGrid struct {
	Lats []float64
	Lons []float64
	W, H int
}

// Build precomputes the per-pixel lat/lon arrays for an XYZ tile of (w,h).
// Cost: O(w + h) — lons depend only on x, lats only on y.
func Build(t XYZ, w, h int) LatLonGrid {
	south, west, north, east := t.Bounds()
	lons := make([]float64, w)
	for i := 0; i < w; i++ {
		lons[i] = west + (east-west)*(float64(i)+0.5)/float64(w)
	}
	lats := make([]float64, h)
	// Pixel y is in Mercator, so compute lat per-row exactly via mercY of the
	// per-pixel normalised y across the whole tile. That's the right move for
	// a single tile (h=256 evaluations is trivial); the per-pixel lat is then
	// the same for every column.
	n := float64(int(1) << uint(t.Z))
	for j := 0; j < h; j++ {
		yn := float64(t.Y) + (float64(j)+0.5)/float64(h)
		lats[j] = mercY(yn / n)
	}
	_ = south
	_ = north
	_ = lats
	return LatLonGrid{Lats: lats, Lons: lons, W: w, H: h}
}
