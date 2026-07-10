package grid

import (
	"math"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// RotatedGaussian is Grid Definition Template 3.41.
type RotatedGaussian struct {
	Gaussian
	SouthPoleLat float64
	SouthPoleLon float64
	Angle        float64
}

func ParseRotatedGaussian(template, optionalList []byte, listOctets int) RotatedGaussian {
	g := RotatedGaussian{Gaussian: ParseGaussian(template[:58], optionalList, listOctets)}
	scale := angleScale(template)
	g.SouthPoleLat = float64(bswap.I32SM(template, 58)) / scale
	g.SouthPoleLon = float64(bswap.U32(template, 62)) / scale
	g.Angle = float64(bswap.F32(template, 66))
	return g
}

func (g RotatedGaussian) Locate(lat, lon float64) (float64, float64, bool) {
	rotatedLat, rotatedLon := g.geoToRotated(lat, lon)
	return g.Gaussian.Locate(rotatedLat, rotatedLon)
}

func (g RotatedGaussian) geoToRotated(lat, lon float64) (float64, float64) {
	const deg2rad = math.Pi / 180
	const rad2deg = 180 / math.Pi
	latP := g.SouthPoleLat * deg2rad
	phi := lat * deg2rad
	dlam := (lon - g.SouthPoleLon) * deg2rad
	sinLatR := -math.Sin(latP)*math.Sin(phi) - math.Cos(latP)*math.Cos(phi)*math.Cos(dlam)
	numerator := math.Cos(phi) * math.Sin(dlam)
	denominator := -math.Cos(latP)*math.Sin(phi) + math.Sin(latP)*math.Cos(phi)*math.Cos(dlam)
	rotatedLat := math.Asin(clamp1(sinLatR)) * rad2deg
	rotatedLon := math.Atan2(numerator, denominator)*rad2deg + g.Angle
	return rotatedLat, rotatedLon
}
