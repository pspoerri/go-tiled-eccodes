package grid

import "math"

// rectScan holds the index/scanning math shared by all rectangular projected
// grids (Mercator, Lambert, polar stereographic, regular Gaussian's
// rectangular layout). Each grid embeds this and supplies its own Locate.
type rectScan struct {
	Nx, Ny      int
	IPositive   bool
	JPositive   bool
	Consecutive bool
	Alternate   bool
}

func (r rectScan) Size() (int, int) { return r.Nx, r.Ny }
func (r rectScan) NumPoints() int   { return r.Nx * r.Ny }

func (r rectScan) Index(i, j int) int {
	if i < 0 || i >= r.Nx || j < 0 || j >= r.Ny {
		return -1
	}
	si := i
	sj := j
	if !r.IPositive {
		si = r.Nx - 1 - i
	}
	if r.JPositive {
		sj = r.Ny - 1 - j
	}
	if !r.Consecutive {
		return si*r.Ny + sj
	}
	if r.Alternate && (sj&1) == 1 {
		si = r.Nx - 1 - si
	}
	return sj*r.Nx + si
}

const (
	earthRadiusMeters = 6371229.0 // GRIB shape-of-earth code 6 default
	deg2rad           = math.Pi / 180
	rad2deg           = 180 / math.Pi
)

// parseProjScan reads the scanning-mode byte of a projected grid and the
// (Nx, Ny) integers; the byte offsets vary slightly by template, so callers
// pass them in. Reflects the same conventions as ParseLatLon.
func parseProjScan(scan byte) (iPos, jPos, consec, alt bool) {
	iPos = scan&0x80 == 0
	jPos = scan&0x40 != 0
	consec = scan&0x20 == 0
	alt = scan&0x10 != 0
	return
}
