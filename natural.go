package grib

import (
	"math"

	"github.com/pspoerri/go-tiled-eccodes/grid"
)

// RegularLatLon is a normalized regular latitude/longitude grid definition
// (GRIB Grid Definition Template 3.0) with the WMO scanning-mode bits already
// resolved into signed steps. It describes the field in natural order — row 0
// is the northernmost row, column 0 the westernmost — so it pairs 1:1 with
// DecodeNaturalFloat32/DecodeNaturalFloat64: the value at index row*Nx+col sits
// at the grid point
//
//	lat = Lat0 + row*DLat,  lon = Lon0 + col*DLon.
//
// Lat0/Lon0 are the coordinates of natural grid point (0,0): the northernmost
// grid-point latitude and the westernmost grid-point longitude — node centers,
// not cell edges. Because the orientation is normalized, DLat is always
// negative (north→south, magnitude Dj) and DLon always positive (west→east,
// magnitude Di), regardless of how the source message was scanned.
type RegularLatLon struct {
	Nx, Ny     int
	Lat0, Lon0 float64 // northwest grid point, degrees
	DLat, DLon float64 // signed step; DLat < 0 (N→S), DLon > 0 (W→E)
}

// RegularLatLon reports the message as a normalized regular lat/lon grid-def.
// ok is false for any non-template-3.0 grid (rotated, Gaussian, projected,
// unstructured) or if Section 3 fails to parse. The scan bits are consumed
// here, so callers never touch them; the result pairs with DecodeNatural*.
func (m *Message) RegularLatLon() (RegularLatLon, bool) {
	g, err := m.Grid()
	if err != nil {
		return RegularLatLon{}, false
	}
	ll, ok := g.(grid.LatLon)
	if !ok {
		return RegularLatLon{}, false
	}
	r := RegularLatLon{
		Nx:   ll.Ni,
		Ny:   ll.Nj,
		Lat0: ll.La1,
		Lon0: ll.Lo1,
		DLat: -ll.Dj, // natural order scans N→S
		DLon: ll.Di,  // ...and W→E
	}
	// La1/Lo1 are the message's first grid point, which is the NW corner only
	// for the usual N→S, W→E scan. Walk to the NW corner for the other scans.
	if ll.JPositive { // j increases with latitude: La1 is the south point
		r.Lat0 = ll.La1 + float64(ll.Nj-1)*ll.Dj
	}
	if !ll.IPositive { // i decreases with longitude: Lo1 is the east point
		r.Lon0 = ll.Lo1 - float64(ll.Ni-1)*ll.Di
	}
	return r, true
}

// DecodeNaturalFloat32 decodes the message into dst in guaranteed natural
// order: west→east within each row, rows north→south, regardless of the
// message's WMO scanning mode. It pairs 1:1 with RegularLatLon —
// dst[row*Nx+col] is the value at grid point (Lat0+row*DLat, Lon0+col*DLon).
//
// This differs from DecodeFloat32, which returns the message's own storage
// (scan) order: for any non-natural scan the two outputs are permutations of
// each other. dst is grown when nil or too small, otherwise reused; the
// returned slice aliases it.
func (m *Message) DecodeNaturalFloat32(dst []float32) ([]float32, error) {
	return decodeNatural(m, dst)
}

// DecodeNaturalFloat64 is DecodeNaturalFloat32 without the float32 narrowing —
// it fills dst with the full-precision values in natural order. See
// DecodeNaturalFloat32 for the ordering contract.
func (m *Message) DecodeNaturalFloat64(dst []float64) ([]float64, error) {
	return decodeNatural(m, dst)
}

type realFloat interface{ ~float32 | ~float64 }

// decodeNatural is the shared core behind DecodeNaturalFloat32/Float64. It
// reorders the storage-order decode buffer into natural order with three tiers,
// fastest first:
//
//  1. Storage order already is natural (natural lat/lon & Gaussian,
//     unstructured) — a straight convert-copy.
//  2. Rectangular, consecutive, non-alternating scan — the reorder is a
//     separable row-flip and/or per-row column reverse, so values are moved a
//     contiguous row at a time instead of through a per-point Index call.
//  3. Anything exotic (alternating rows, reduced Gaussian) — fall back to the
//     scan-aware Grid.Index per point; missing cells become NaN.
func decodeNatural[T realFloat](m *Message, dst []T) ([]T, error) {
	raw, err := m.decodeCached() // storage order, []float64
	if err != nil {
		return nil, err
	}
	g, err := m.Grid()
	if err != nil {
		return nil, err
	}
	ni, nj := g.Size()
	n := ni * nj
	if cap(dst) < n {
		dst = make([]T, n)
	} else {
		dst = dst[:n]
	}

	// Tier 1: storage order == natural order. The IsNatural types guarantee
	// len(raw) == ni*nj, so a flat copy fills dst exactly.
	if nat, ok := g.(interface{ IsNatural() bool }); ok && nat.IsNatural() {
		for i, v := range raw {
			dst[i] = T(v)
		}
		return dst, nil
	}

	// Tier 2: rectangular scan we can un-scramble row by row. jPositive flips
	// the row order (S→N storage); !iPositive reverses columns within a row.
	if r, ok := rectLayout(g); ok && r.consecutive && !r.alternate && r.ni*r.nj == len(raw) {
		ni, nj := r.ni, r.nj
		for j := 0; j < nj; j++ {
			sj := j
			if r.jPositive {
				sj = nj - 1 - j
			}
			srow := raw[sj*ni : sj*ni+ni]
			drow := dst[j*ni : j*ni+ni]
			if r.iPositive {
				for i, v := range srow {
					drow[i] = T(v)
				}
			} else {
				for i, v := range srow {
					drow[ni-1-i] = T(v)
				}
			}
		}
		return dst, nil
	}

	// Tier 3: route each natural (i,j) back to its storage offset.
	for j := 0; j < nj; j++ {
		row := j * ni
		for i := 0; i < ni; i++ {
			off := g.Index(i, j)
			if off < 0 || off >= len(raw) {
				dst[row+i] = T(math.NaN())
				continue
			}
			dst[row+i] = T(raw[off])
		}
	}
	return dst, nil
}
