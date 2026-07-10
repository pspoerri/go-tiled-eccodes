package grid

import (
	"math"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// Earth describes the GRIB2 Section 3 reference-system shape in metres.
type Earth struct {
	Shape     uint8
	Radius    float64
	MajorAxis float64
	MinorAxis float64
}

// ParseEarth decodes the 16-octet Earth-shape prefix shared by the supported
// Section 3 templates.
func ParseEarth(t []byte) Earth {
	if len(t) < 16 {
		return Earth{Shape: 6, Radius: 6371229}
	}
	e := Earth{Shape: t[0]}
	switch e.Shape {
	case 0:
		e.Radius = 6367470
	case 1:
		e.Radius = scaledEarthValue(t[1], bswap.U32(t, 2), 1)
	case 2:
		e.MajorAxis, e.MinorAxis = 6378160, 6356775
	case 3:
		e.MajorAxis = scaledEarthValue(t[6], bswap.U32(t, 7), 1000)
		e.MinorAxis = scaledEarthValue(t[11], bswap.U32(t, 12), 1000)
	case 4, 5, 10:
		e.MajorAxis, e.MinorAxis = 6378137, 6356752.314245
	case 6:
		e.Radius = 6371229
	case 7:
		e.MajorAxis = scaledEarthValue(t[6], bswap.U32(t, 7), 1)
		e.MinorAxis = scaledEarthValue(t[11], bswap.U32(t, 12), 1)
	case 8:
		e.Radius = 6371200
	case 9:
		e.MajorAxis, e.MinorAxis = 6377563.396, 6356256.909
	case 11:
		e.Radius = 695990000
	default:
		e.Shape, e.Radius = 6, 6371229
	}
	if e.Radius <= 0 && (e.MajorAxis <= 0 || e.MinorAxis <= 0) {
		e.Shape, e.Radius = 6, 6371229
	}
	return e
}

func scaledEarthValue(scale uint8, value uint32, unit float64) float64 {
	if scale == 255 || value == 0xffffffff {
		return 0
	}
	return float64(value) * math.Pow10(-int(scale)) * unit
}

// EffectiveRadius returns the exact spherical radius or the authalic radius
// of an ellipsoid. The latter preserves surface area in the spherical
// projection formulas used by this tile-oriented package.
func (e Earth) EffectiveRadius() float64 {
	if e.Radius > 0 {
		return e.Radius
	}
	a, b := e.MajorAxis, e.MinorAxis
	if a <= 0 || b <= 0 {
		return 6371229
	}
	if a < b {
		a, b = b, a
	}
	ecc2 := 1 - (b*b)/(a*a)
	if ecc2 <= 0 {
		return a
	}
	ecc := math.Sqrt(ecc2)
	factor := (1 - ecc2) * math.Atanh(ecc) / ecc
	return a * math.Sqrt(0.5*(1+factor))
}
