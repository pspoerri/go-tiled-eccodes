package grid

import (
	"math"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// Lambert is Grid Definition Template 3.30 — Lambert conformal conic.
//
// Template body offsets (starting at Section 3 byte 14):
//
//	0      shape of earth
//	1-15   earth radius / axis fields
//	16-19  Nx
//	20-23  Ny
//	24-27  La1
//	28-31  Lo1
//	32     resolution and component flags
//	33-36  LaD (latitude where Dx, Dy are specified)
//	37-40  LoV (orientation longitude)
//	41-44  Dx in 10^-3 m at LaD
//	45-48  Dy in 10^-3 m at LaD
//	49     projection centre flag
//	50     scanning mode
//	51-54  Latin1 (first standard parallel)
//	55-58  Latin2 (second standard parallel)
//	59-62  latitude of southern pole of projection (rarely non-zero)
//	63-66  longitude of southern pole of projection
type Lambert struct {
	rectScan
	La1, Lo1 float64
	LaD, LoV float64
	Dx, Dy   float64 // metres
	Latin1   float64
	Latin2   float64

	n, F             float64
	xOrigin, yOrigin float64
}

func ParseLambert(t []byte) Lambert {
	g := Lambert{}
	g.Nx = int(bswap.U32(t, 16))
	g.Ny = int(bswap.U32(t, 20))
	g.La1 = float64(bswap.I32SM(t, 24)) / 1e6
	g.Lo1 = float64(bswap.I32SM(t, 28)) / 1e6
	g.LaD = float64(bswap.I32SM(t, 33)) / 1e6
	g.LoV = float64(bswap.I32SM(t, 37)) / 1e6
	g.Dx = float64(int32(bswap.U32(t, 41))) * 1e-3
	g.Dy = float64(int32(bswap.U32(t, 45))) * 1e-3
	scan := t[50]
	g.IPositive, g.JPositive, g.Consecutive, g.Alternate = parseProjScan(scan)
	g.Latin1 = float64(bswap.I32SM(t, 51)) / 1e6
	g.Latin2 = float64(bswap.I32SM(t, 55)) / 1e6

	// Cone constants (spherical earth approximation). When Latin1 == Latin2
	// the cone degenerates to a tangent cone and n = sin(Latin1).
	phi1 := g.Latin1 * deg2rad
	phi2 := g.Latin2 * deg2rad
	if math.Abs(phi1-phi2) < 1e-12 {
		g.n = math.Sin(phi1)
	} else {
		g.n = math.Log(math.Cos(phi1)/math.Cos(phi2)) /
			math.Log(math.Tan(math.Pi/4+phi2/2)/math.Tan(math.Pi/4+phi1/2))
	}
	g.F = math.Cos(phi1) * math.Pow(math.Tan(math.Pi/4+phi1/2), g.n) / g.n

	g.xOrigin, g.yOrigin = g.project(g.La1, g.Lo1)
	return g
}

// project returns Lambert (x, y) in metres for a (lat, lon).
func (g Lambert) project(lat, lon float64) (x, y float64) {
	phi := lat * deg2rad
	rho := earthRadiusMeters * g.F / math.Pow(math.Tan(math.Pi/4+phi/2), g.n)
	theta := g.n * (lon - g.LoV) * deg2rad
	x = rho * math.Sin(theta)
	y = -rho * math.Cos(theta)
	return
}

func (g Lambert) Locate(lat, lon float64) (float64, float64, bool) {
	if g.Dx == 0 || g.Dy == 0 || g.n == 0 {
		return 0, 0, false
	}
	x, y := g.project(lat, lon)
	fi := (x - g.xOrigin) / g.Dx
	fj := (g.yOrigin - y) / g.Dy
	if fi < -0.5 || fi > float64(g.Nx)-0.5 {
		return 0, 0, false
	}
	if fj < -0.5 || fj > float64(g.Ny)-0.5 {
		return 0, 0, false
	}
	return fi, fj, true
}
