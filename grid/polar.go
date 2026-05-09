package grid

import (
	"math"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// Polar is Grid Definition Template 3.20 — polar stereographic projection.
//
// Template body offsets (template starts at Section 3 byte 14):
//
//	0      shape of earth
//	1-15   earth radius / axis fields
//	16-19  Nx
//	20-23  Ny
//	24-27  La1
//	28-31  Lo1
//	32     resolution and component flags
//	33-36  LaD — latitude at which Dx, Dy are specified (typically ±60°)
//	37-40  LoV — orientation longitude (the meridian going to the bottom of map)
//	41-44  Dx in 10^-3 m at LaD
//	45-48  Dy in 10^-3 m at LaD
//	49     projection centre flag (bit 1 = 0 → north pole, =1 → south pole)
//	50     scanning mode
type Polar struct {
	rectScan
	La1, Lo1     float64
	LaD, LoV     float64
	Dx, Dy       float64 // metres at LaD
	NorthHem     bool    // true if north-polar projection
	xOrigin      float64
	yOrigin      float64
	scaleFromLaD float64 // 1 + sin(|LaD|)
}

func ParsePolar(t []byte) Polar {
	g := Polar{}
	g.Nx = int(bswap.U32(t, 16))
	g.Ny = int(bswap.U32(t, 20))
	g.La1 = float64(bswap.I32SM(t, 24)) / 1e6
	g.Lo1 = float64(bswap.I32SM(t, 28)) / 1e6
	g.LaD = float64(bswap.I32SM(t, 33)) / 1e6
	g.LoV = float64(bswap.I32SM(t, 37)) / 1e6
	g.Dx = float64(int32(bswap.U32(t, 41))) * 1e-3
	g.Dy = float64(int32(bswap.U32(t, 45))) * 1e-3
	pcf := t[49]
	g.NorthHem = pcf&0x80 == 0
	scan := t[50]
	g.IPositive, g.JPositive, g.Consecutive, g.Alternate = parseProjScan(scan)

	// Stereographic scale: a point's distance from the pole is
	// r(lat) = 2*R*K*tan((π/2 - |lat|)/2), with K such that Dx is the metric
	// step at LaD. K = (1 + sin(|LaD|))/2 → r(lat) = R*(1 + sin|LaD|)*tan((π/2 - |lat|)/2)
	g.scaleFromLaD = (1 + math.Sin(math.Abs(g.LaD)*deg2rad)) * earthRadiusMeters
	x0, y0 := g.project(g.La1, g.Lo1)
	g.xOrigin, g.yOrigin = x0, y0
	return g
}

// project returns the (x, y) projected metres of a (lat, lon) point relative
// to the projection origin (pole), with x along longitude LoV and y oriented
// such that increasing j in the natural scanning order goes "down" (away from
// the pole towards LoV).
func (g Polar) project(lat, lon float64) (x, y float64) {
	sign := 1.0
	if !g.NorthHem {
		sign = -1
	}
	r := g.scaleFromLaD * math.Tan((math.Pi/4)-sign*lat*deg2rad/2)
	dlon := (lon - g.LoV) * deg2rad
	x = r * math.Sin(dlon)
	y = -sign * r * math.Cos(dlon)
	return
}

func (g Polar) Locate(lat, lon float64) (float64, float64, bool) {
	if g.Dx == 0 || g.Dy == 0 {
		return 0, 0, false
	}
	if g.NorthHem && lat < -10 || !g.NorthHem && lat > 10 {
		return 0, 0, false
	}
	x, y := g.project(lat, lon)
	fi := (x - g.xOrigin) / g.Dx
	fj := (y - g.yOrigin) / g.Dy
	if fi < -0.5 || fi > float64(g.Nx)-0.5 {
		return 0, 0, false
	}
	if fj < -0.5 || fj > float64(g.Ny)-0.5 {
		return 0, 0, false
	}
	return fi, fj, true
}
