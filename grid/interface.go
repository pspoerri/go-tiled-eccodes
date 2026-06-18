// Package grid contains GRIB2 Section 3 grid-definition implementations.
//
// Conventions:
//   - i is the column index (along longitude / x), j is the row (along
//     latitude / y). Both are zero-based, in the *natural* WMO scanning order
//     (i increases west→east, j increases north→south).
//   - Index(i,j) returns the offset of point (i,j) inside the decoded
//     []float64 buffer. It accounts for the GRIB scanning-mode bits so callers
//     never need to.
//   - Locate(lat, lon) returns the floating-point fractional source pixel
//     coordinates (i, j) of the geographic point — bicubic resamplers consume
//     the fractional part directly.
package grid

import "math"

// Grid is the abstraction the renderer uses to translate WGS84 coordinates
// into source-buffer indices. All grid types implement this interface.
type Grid interface {
	// Size returns the number of columns and rows. For irregular grids
	// (reduced Gaussian) Ni is the maximum row width; callers must use
	// Index(i,j) and not compute offsets directly.
	Size() (ni, nj int)

	// NumPoints returns the total number of decoded values. Equals Ni*Nj for
	// regular grids; for reduced grids it equals sum(pl[]).
	NumPoints() int

	// Index returns the linear offset of point (i,j) in the decoded buffer.
	// Returns -1 if (i,j) is out of bounds.
	Index(i, j int) int

	// Locate maps a WGS84 (lat, lon) coordinate to fractional source-pixel
	// coordinates (i, j). lon is normalised to the grid's range internally.
	// Returns ok=false if the point lies outside the grid extents.
	Locate(lat, lon float64) (i, j float64, ok bool)
}

// wrap360 normalises lon into [base, base+360).
//
// In the tile-render hot path lon is at most one period out of range (it comes
// from a pixel's spherical-Mercator longitude against the grid's base), so the
// common case is resolved with one comparison and at most one ±360 add —
// avoiding math.Mod, which dominated the per-pixel coordinate cost. math.Mod is
// used only as the fallback for longitudes more than one period out of range.
func wrap360(lon, base float64) float64 {
	x := lon - base
	if x >= 0 {
		if x < 360 {
			return lon // already in [base, base+360)
		}
		if x < 720 {
			return lon - 360
		}
	} else if x >= -360 {
		return lon + 360
	}
	// Far out of range: full reduction.
	x = math.Mod(x, 360)
	if x < 0 {
		x += 360
	}
	return base + x
}
