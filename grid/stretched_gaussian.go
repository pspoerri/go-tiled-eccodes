package grid

import (
	"math"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// StretchedGaussian is Grid Definition Template 3.42.
type StretchedGaussian struct {
	Gaussian
	StretchPoleLat float64
	StretchPoleLon float64
	StretchFactor  float64
}

func ParseStretchedGaussian(template, optionalList []byte, listOctets int) StretchedGaussian {
	scale := angleScale(template)
	return StretchedGaussian{
		Gaussian:       ParseGaussian(template[:58], optionalList, listOctets),
		StretchPoleLat: float64(bswap.I32SM(template, 58)) / scale,
		StretchPoleLon: float64(bswap.I32SM(template, 62)) / scale,
		StretchFactor:  float64(bswap.U32(template, 66)) / 1e6,
	}
}

func (g StretchedGaussian) Locate(lat, lon float64) (float64, float64, bool) {
	lat, lon, ok := stretchCoordinates(lat, lon, g.StretchPoleLat, g.StretchPoleLon, g.StretchFactor)
	if !ok {
		return 0, 0, false
	}
	return g.Gaussian.Locate(lat, lon)
}

// StretchedRotatedGaussian is Grid Definition Template 3.43. Rotation is
// applied first; the stretching pole is expressed in that model coordinate
// system.
type StretchedRotatedGaussian struct {
	RotatedGaussian
	StretchPoleLat float64
	StretchPoleLon float64
	StretchFactor  float64
}

func ParseStretchedRotatedGaussian(template, optionalList []byte, listOctets int) StretchedRotatedGaussian {
	scale := angleScale(template)
	return StretchedRotatedGaussian{
		RotatedGaussian: ParseRotatedGaussian(template[:70], optionalList, listOctets),
		StretchPoleLat:  float64(bswap.I32SM(template, 70)) / scale,
		StretchPoleLon:  float64(bswap.I32SM(template, 74)) / scale,
		StretchFactor:   float64(bswap.U32(template, 78)) / 1e6,
	}
}

func (g StretchedRotatedGaussian) Locate(lat, lon float64) (float64, float64, bool) {
	lat, lon = g.RotatedGaussian.geoToRotated(lat, lon)
	lat, lon, ok := stretchCoordinates(lat, lon, g.StretchPoleLat, g.StretchPoleLon, g.StretchFactor)
	if !ok {
		return 0, 0, false
	}
	return g.Gaussian.Locate(lat, lon)
}

// stretchCoordinates rotates the stretching pole to 90N and applies the WMO
// latitude transform. C=1 and pole=(90,0) is the identity mapping.
func stretchCoordinates(lat, lon, poleLat, poleLon, c float64) (float64, float64, bool) {
	if c <= 0 || math.IsNaN(c) {
		return 0, 0, false
	}
	const deg2rad = math.Pi / 180
	const rad2deg = 180 / math.Pi
	phi := lat * deg2rad
	pole := poleLat * deg2rad
	dlon := (lon - poleLon) * deg2rad
	sinX := math.Sin(phi)*math.Sin(pole) + math.Cos(phi)*math.Cos(pole)*math.Cos(dlon)
	x := math.Asin(clamp1(sinX))
	y := math.Atan2(
		math.Cos(phi)*math.Sin(dlon),
		math.Sin(pole)*math.Cos(phi)*math.Cos(dlon)-math.Cos(pole)*math.Sin(phi),
	)
	c2 := c * c
	sinX1 := ((1 - c2) + (1+c2)*math.Sin(x)) / ((1 + c2) + (1-c2)*math.Sin(x))
	return math.Asin(clamp1(sinX1)) * rad2deg, y * rad2deg, true
}
