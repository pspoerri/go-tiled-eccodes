package grid

import (
	"math"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// RotatedLatLon is Grid Definition Template 3.1 — rotated latitude/longitude.
//
// Identical to 3.0 plus three trailing fields (sign-magnitude int32, encoded
// the same way as La/Lo):
//
//	bytes 58-61  latitude of southern pole (rotated frame)
//	bytes 62-65  longitude of southern pole
//	bytes 66-69  angle of rotation about the new polar axis
//
// The grid is defined in the rotated frame: La1/Lo1, Di/Dj are rotated
// coordinates. To map a geographic (lat, lon) into the grid we first rotate
// the input into the rotated frame, then index as if it were a regular
// lat/lon.
type RotatedLatLon struct {
	LatLon
	SouthPoleLat float64 // rotated frame's south pole, in geographic deg
	SouthPoleLon float64
	Angle        float64 // additional rotation about the rotated polar axis
}

// ParseRotatedLatLon decodes a Section 3 template-1 body.
func ParseRotatedLatLon(t []byte) RotatedLatLon {
	g := RotatedLatLon{
		LatLon: ParseLatLon(t),
	}
	scale := angleScale(t)
	g.SouthPoleLat = float64(bswap.I32SM(t, 58)) / scale
	g.SouthPoleLon = float64(bswap.I32SM(t, 62)) / scale
	g.Angle = float64(bswap.I32SM(t, 66)) / scale
	return g
}

// Locate transforms geographic (lat, lon) into the rotated frame and then
// uses the embedded LatLon's logic. The transformation follows the WMO
// convention: the rotation is parametrised by the geographic position of the
// rotated frame's south pole.
//
// Reference (DWD/COSMO formulation):
//
//	sin(rlat) = -sin(latP)*sin(lat) - cos(latP)*cos(lat)*cos(lon - lonP)
//	tan(rlon) =  cos(lat)*sin(lon - lonP)
//	            ─────────────────────────────────────────
//	             -cos(latP)*sin(lat) + sin(latP)*cos(lat)*cos(lon - lonP)
//	rlon = atan2(num, den) + angle
//
// where (latP, lonP) is the rotated south pole.
func (g RotatedLatLon) Locate(lat, lon float64) (float64, float64, bool) {
	const deg2rad = math.Pi / 180
	latR, lonR := g.geoToRotated(lat, lon)
	_ = deg2rad
	return g.LatLon.Locate(latR, lonR)
}

func (g RotatedLatLon) geoToRotated(lat, lon float64) (rlat, rlon float64) {
	const deg2rad = math.Pi / 180
	const rad2deg = 180 / math.Pi
	latP := g.SouthPoleLat * deg2rad
	lonP := g.SouthPoleLon * deg2rad
	phi := lat * deg2rad
	dlam := (lon - g.SouthPoleLon) * deg2rad
	sinLatR := -math.Sin(latP)*math.Sin(phi) - math.Cos(latP)*math.Cos(phi)*math.Cos(dlam)
	num := math.Cos(phi) * math.Sin(dlam)
	den := -math.Cos(latP)*math.Sin(phi) + math.Sin(latP)*math.Cos(phi)*math.Cos(dlam)
	rlat = math.Asin(clamp1(sinLatR)) * rad2deg
	rlon = math.Atan2(num, den)*rad2deg + g.Angle
	_ = lonP
	return
}

func clamp1(x float64) float64 {
	if x > 1 {
		return 1
	}
	if x < -1 {
		return -1
	}
	return x
}
