package grid

import (
	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// LatLon is Grid Definition Template 3.0 — regular latitude/longitude.
//
// Template body (zero-based bytes after Section 3 byte 14):
//
//	0     shape of earth
//	1     scale factor of radius of spherical earth
//	2-5   scaled value of radius
//	6     scale factor of major axis
//	7-10  scaled value
//	11    scale factor of minor axis
//	12-15 scaled value
//	16-19 Ni (uint32)
//	20-23 Nj (uint32)
//	24-27 basic angle (uint32)
//	28-31 subdivisions of basic angle (uint32)
//	32-35 La1 (sign-magnitude int32, micro-degrees if basic angle = 0)
//	36-39 Lo1
//	40    resolution and component flags
//	41-44 La2
//	45-48 Lo2
//	49-52 Di
//	53-56 Dj
//	57    scanning mode
type LatLon struct {
	Ni, Nj      int
	La1, Lo1    float64 // first point (degrees)
	La2, Lo2    float64 // last point (degrees)
	Di, Dj      float64 // increments (degrees, always positive in our struct)
	IPositive   bool    // true if i increases with longitude (scanning bit 1)
	JPositive   bool    // true if j increases with latitude (scanning bit 2)
	Consecutive bool    // true if data laid out row-by-row (scanning bit 3 cleared)
	Alternate   bool    // true if alternate rows scan in opposite direction
}

// ParseLatLon decodes a Section 3 template-0 body into a LatLon.
func ParseLatLon(t []byte) LatLon {
	angleScale := angleScale(t)
	g := LatLon{
		Ni: int(bswap.U32(t, 16)),
		Nj: int(bswap.U32(t, 20)),
	}
	g.La1 = float64(bswap.I32SM(t, 32)) / angleScale
	g.Lo1 = float64(bswap.I32SM(t, 36)) / angleScale
	g.La2 = float64(bswap.I32SM(t, 41)) / angleScale
	g.Lo2 = float64(bswap.I32SM(t, 45)) / angleScale
	g.Di = float64(bswap.U32(t, 49)) / angleScale
	g.Dj = float64(bswap.U32(t, 53)) / angleScale

	scan := t[57]
	g.IPositive = scan&0x80 == 0 // bit 1 = 0 -> +i (west to east)
	g.JPositive = scan&0x40 != 0 // bit 2 = 1 -> +j (south to north)
	g.Consecutive = scan&0x20 == 0
	g.Alternate = scan&0x10 != 0
	return g
}

// angleScale returns the divisor that converts the raw Section-3 lat/lon
// integers into degrees. When basic angle = 0 (almost always the case) the
// divisor is 1e6 — i.e. values are in micro-degrees. Otherwise it is
// subdivisions / basic_angle.
func angleScale(t []byte) float64 {
	basic := bswap.U32(t, 24)
	subs := bswap.U32(t, 28)
	if basic == 0 || basic == 0xffffffff {
		return 1e6
	}
	if subs == 0 || subs == 0xffffffff {
		return 1e6
	}
	return float64(subs) / float64(basic)
}

func (g LatLon) Size() (int, int) { return g.Ni, g.Nj }
func (g LatLon) NumPoints() int   { return g.Ni * g.Nj }

// IsNatural is true when the storage order matches "natural" row-major
// scanning (W→E within rows, rows N→S, no alternation). The renderer takes
// a fast in-line path through buf[j*Ni+i] for these — interface dispatch
// through Index() costs ~5 ns per source read otherwise.
func (g LatLon) IsNatural() bool {
	return g.IPositive && !g.JPositive && g.Consecutive && !g.Alternate
}

func (g LatLon) Index(i, j int) int {
	if i < 0 || i >= g.Ni || j < 0 || j >= g.Nj {
		return -1
	}
	// Scanning mode is reflected in how we *interpret* the i/j the caller
	// supplies — we convert from "natural" (W→E, N→S) to storage order.
	si := i
	sj := j
	if !g.IPositive {
		si = g.Ni - 1 - i
	}
	if g.JPositive {
		sj = g.Nj - 1 - j
	}
	if !g.Consecutive {
		// Data laid out column-major.
		return si*g.Nj + sj
	}
	if g.Alternate && (sj&1) == 1 {
		// Alternate rows reverse direction.
		si = g.Ni - 1 - si
	}
	return sj*g.Ni + si
}

// Locate returns fractional (i, j) for a (lat, lon) in WGS84 degrees, where
// (0, 0) is the north-west grid corner in natural scanning order (highest
// latitude, westernmost longitude).
func (g LatLon) Locate(lat, lon float64) (float64, float64, bool) {
	north := g.La1
	south := g.La2
	if north < south {
		north, south = south, north
	}
	if lat > north+1e-9 || lat < south-1e-9 {
		return 0, 0, false
	}

	west := g.Lo1
	east := g.Lo2
	if east < west {
		east += 360
	}
	lonN := wrap360(lon, west)
	if lonN > east+1e-9 {
		return 0, 0, false
	}

	fi := (lonN - west) / g.Di
	fj := (north - lat) / g.Dj
	if fi < 0 {
		fi = 0
	} else if fi > float64(g.Ni-1) {
		fi = float64(g.Ni - 1)
	}
	if fj < 0 {
		fj = 0
	} else if fj > float64(g.Nj-1) {
		fj = float64(g.Nj - 1)
	}
	return fi, fj, true
}

// Bounds returns the (south, west, north, east) extent in degrees.
func (g LatLon) Bounds() (south, west, north, east float64) {
	north = g.La1
	south = g.La2
	if north < south {
		north, south = south, north
	}
	west = g.Lo1
	east = g.Lo2
	if east < west {
		east += 360
	}
	return
}
