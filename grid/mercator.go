package grid

import (
	"math"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// Mercator is Grid Definition Template 3.10 — Mercator projection.
//
// Template body offsets (template starts at Section 3 byte 14):
//
//	0      shape of earth
//	1-15   earth radius / axis fields
//	16-19  Nx
//	20-23  Ny
//	24-27  La1 (sign-magnitude, micro-degrees)
//	28-31  Lo1
//	32     resolution and component flags
//	33-36  LaD — latitude at which the Mercator projection intersects the Earth
//	37-40  La2
//	41-44  Lo2
//	45     scanning mode
//	46-49  angle of orientation
//	50-53  Di in 10^-3 m at LaD
//	54-57  Dj in 10^-3 m at LaD
type Mercator struct {
	rectScan
	La1, Lo1 float64
	La2, Lo2 float64
	LaD      float64
	Di, Dj   float64 // metres at LaD

	scaleR  float64 // R*cos(LaD), in metres per radian
	yOrigin float64 // Mercator y of La1 (metres)
}

func ParseMercator(t []byte) Mercator {
	g := Mercator{}
	g.Nx = int(bswap.U32(t, 16))
	g.Ny = int(bswap.U32(t, 20))
	g.La1 = float64(bswap.I32SM(t, 24)) / 1e6
	g.Lo1 = float64(bswap.I32SM(t, 28)) / 1e6
	g.LaD = float64(bswap.I32SM(t, 33)) / 1e6
	g.La2 = float64(bswap.I32SM(t, 37)) / 1e6
	g.Lo2 = float64(bswap.I32SM(t, 41)) / 1e6
	scan := t[45]
	g.IPositive, g.JPositive, g.Consecutive, g.Alternate = parseProjScan(scan)
	g.Di = float64(int32(bswap.U32(t, 50))) * 1e-3
	g.Dj = float64(int32(bswap.U32(t, 54))) * 1e-3

	g.scaleR = earthRadiusMeters * math.Cos(g.LaD*deg2rad)
	g.yOrigin = g.scaleR * math.Log(math.Tan(math.Pi/4+g.La1*deg2rad/2))
	return g
}

// Locate inverts the Mercator projection: y = R·cos(LaD)·ln(tan(π/4 + lat/2)),
// x = R·cos(LaD)·(lon − Lo1) (in radians), then divides by Di/Dj to find the
// grid cell. j increases southward in natural order (so we subtract).
func (g Mercator) Locate(lat, lon float64) (float64, float64, bool) {
	if g.Di == 0 || g.Dj == 0 || g.scaleR == 0 {
		return 0, 0, false
	}
	dlon := wrap180(lon - g.Lo1)
	x := g.scaleR * dlon * deg2rad
	y := g.scaleR * math.Log(math.Tan(math.Pi/4+lat*deg2rad/2))
	fi := x / g.Di
	fj := (g.yOrigin - y) / g.Dj
	if fi < -0.5 || fi > float64(g.Nx)-0.5 {
		return 0, 0, false
	}
	if fj < -0.5 || fj > float64(g.Ny)-0.5 {
		return 0, 0, false
	}
	return fi, fj, true
}

func wrap180(d float64) float64 {
	x := math.Mod(d+180, 360)
	if x < 0 {
		x += 360
	}
	return x - 180
}
