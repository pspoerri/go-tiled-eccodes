package writer

import (
	"encoding/binary"
	"math"
)

// Grid is the union over Section 3 templates the writer supports — and the
// extension point for user-provided grids. The four methods cover everything
// the writer needs to emit a Section 3 and to reorder Field.Values from
// natural scanning order into the storage layout the decoder will reverse.
//
// Implement this interface in your own package to feed bespoke grids into
// EncodeFile / EncodeMessage. See the LatLon, RotatedLatLon, Mercator,
// Polar, Lambert and Gaussian types in this package for working references.
type Grid interface {
	// TemplateNumber returns the WMO Code Table 3.1 grid definition template
	// number written into Section 3 octets 13-14.
	TemplateNumber() uint16

	// EncodeTemplate returns the template body — the bytes that follow the
	// 14-byte Section 3 prefix. Length depends on the template.
	EncodeTemplate() []byte

	// NumPoints returns the total number of grid points (must equal
	// len(Field.Values) for any field using this grid).
	NumPoints() int

	// NaturalSize returns the (ni, nj) extents in the natural scanning order
	// (W→E columns, N→S rows). The writer iterates 0..ni × 0..nj over the
	// caller's Values slice (laid out row-major in this order) and routes
	// each entry through StorageIndex.
	NaturalSize() (ni, nj int)

	// StorageIndex returns the offset within the packed Section 7 buffer for
	// the natural-order point (i, j). Returns -1 for out-of-bounds.
	StorageIndex(i, j int) int
}

// Scan packs the four scanning-mode bits used by every rectangular grid
// template. The natural / WMO-default layout (i ascending W→E, j ascending
// N→S, row-major, no alternation) corresponds to Scan{IPositive: true,
// Consecutive: true} and encodes to byte 0x00.
type Scan struct {
	IPositive   bool // i increases west→east
	JPositive   bool // j increases south→north
	Consecutive bool // rows are consecutive (false = column-major)
	Alternate   bool // alternate rows reverse direction
}

// NaturalScan returns the conventional W→E, N→S, row-major scanning bits.
func NaturalScan() Scan { return Scan{IPositive: true, Consecutive: true} }

// Byte returns the GRIB2 scanning-mode octet for this Scan.
func (s Scan) Byte() byte {
	var b byte
	if !s.IPositive {
		b |= 0x80
	}
	if s.JPositive {
		b |= 0x40
	}
	if !s.Consecutive {
		b |= 0x20
	}
	if s.Alternate {
		b |= 0x10
	}
	return b
}

// RectIndex implements the storage-offset arithmetic shared by every
// rectangular grid: takes a natural (i, j) and routes it through the
// scanning bits to yield the offset within the packed buffer.
func (s Scan) RectIndex(ni, nj, i, j int) int {
	if i < 0 || i >= ni || j < 0 || j >= nj {
		return -1
	}
	si, sj := i, j
	if !s.IPositive {
		si = ni - 1 - i
	}
	if s.JPositive {
		sj = nj - 1 - j
	}
	if !s.Consecutive {
		return si*nj + sj
	}
	if s.Alternate && (sj&1) == 1 {
		si = ni - 1 - si
	}
	return sj*ni + si
}

// PutI16SM encodes a signed integer as a 16-bit GRIB sign-magnitude field
// (MSB = sign, low 15 bits = magnitude). Exposed for callers writing custom
// grid templates.
func PutI16SM(b []byte, v int16) { putI16SM(b, v) }

// PutI32SM encodes a signed integer as a 32-bit GRIB sign-magnitude field.
func PutI32SM(b []byte, v int32) { putI32SM(b, v) }

// PutAngle writes a degrees value as the sign-magnitude micro-degrees the
// WMO grid templates use for latitudes, longitudes and rotation angles.
func PutAngle(b []byte, deg float64) {
	putI32SM(b, int32(math.Round(deg*1e6)))
}

// EarthShape6 fills in the spherical-earth preamble (template bytes 0..15)
// shared by every projected/lat-lon grid: shape code 6 = sphere of radius
// 6 371 229 m, the GRIB2 default. Bytes 1..15 are scale-factor / scaled-value
// fields that the spherical default ignores; they are left zero.
func EarthShape6(t []byte) {
	t[0] = 6
}

// LatLon — Grid Definition Template 3.0.
//
// Construction conventions match the decoder:
//   - La1, Lo1 is the first grid point; La2, Lo2 is the last.
//   - In the natural scanning order (default), La1 is the northernmost row
//     (so La1 > La2) and Lo1 is the westernmost column (Lo1 < Lo2).
//   - Di, Dj are positive degree increments in i and j.
type LatLon struct {
	Ni, Nj   int
	La1, Lo1 float64
	La2, Lo2 float64
	Di, Dj   float64
	Scan     Scan
}

// NewLatLon builds a regular lat/lon grid with natural scanning. La1/Lo1 is
// the NW corner; the constructor derives La2/Lo2 from Di, Dj.
func NewLatLon(ni, nj int, la1, lo1, di, dj float64) LatLon {
	return LatLon{
		Ni: ni, Nj: nj,
		La1: la1, Lo1: lo1,
		La2: la1 - dj*float64(nj-1),
		Lo2: lo1 + di*float64(ni-1),
		Di:  di, Dj: dj,
		Scan: NaturalScan(),
	}
}

func (g LatLon) TemplateNumber() uint16  { return 0 }
func (g LatLon) NumPoints() int          { return g.Ni * g.Nj }
func (g LatLon) NaturalSize() (int, int) { return g.Ni, g.Nj }
func (g LatLon) StorageIndex(i, j int) int {
	return g.Scan.RectIndex(g.Ni, g.Nj, i, j)
}
func (g LatLon) EncodeTemplate() []byte {
	t := make([]byte, 58)
	EarthShape6(t)
	binary.BigEndian.PutUint32(t[16:], uint32(g.Ni))
	binary.BigEndian.PutUint32(t[20:], uint32(g.Nj))
	binary.BigEndian.PutUint32(t[24:], 0)          // basic angle
	binary.BigEndian.PutUint32(t[28:], 0xffffffff) // subdivisions = "missing" → 1e6 µdeg
	PutAngle(t[32:], g.La1)
	PutAngle(t[36:], g.Lo1)
	t[40] = 0x30 // resolution flags: i and j increments are given
	PutAngle(t[41:], g.La2)
	PutAngle(t[45:], g.Lo2)
	// Di/Dj are encoded as unsigned uint32 micro-degrees (template 3.0). The
	// natural scanning convention treats them as magnitudes with direction
	// carried by the scan flags, so guard against accidental negatives here —
	// uint32(neg-float) is implementation-defined and would silently corrupt
	// the encoded grid.
	binary.BigEndian.PutUint32(t[49:], uint32(math.Round(math.Abs(g.Di)*1e6)))
	binary.BigEndian.PutUint32(t[53:], uint32(math.Round(math.Abs(g.Dj)*1e6)))
	t[57] = g.Scan.Byte()
	return t
}

// RotatedLatLon — Grid Definition Template 3.1.
//
// Identical encoding to 3.0 plus the three trailing rotation parameters
// (south-pole lat, south-pole lon, axial rotation angle). Used by ICON-EU,
// ICON-CH1/CH2, COSMO and other regional models that align an orthogonal
// lat/lon grid with the model domain instead of with the Earth's poles.
type RotatedLatLon struct {
	LatLon
	SouthPoleLat float64
	SouthPoleLon float64
	Angle        float64
}

// NewRotatedLatLon builds a rotated lat/lon grid. La1/Lo1 are coordinates in
// the *rotated* frame, exactly as in the GRIB2 spec.
func NewRotatedLatLon(ni, nj int, la1, lo1, di, dj, southPoleLat, southPoleLon float64) RotatedLatLon {
	return RotatedLatLon{
		LatLon:       NewLatLon(ni, nj, la1, lo1, di, dj),
		SouthPoleLat: southPoleLat,
		SouthPoleLon: southPoleLon,
	}
}

func (g RotatedLatLon) TemplateNumber() uint16 { return 1 }
func (g RotatedLatLon) EncodeTemplate() []byte {
	body := g.LatLon.EncodeTemplate()
	out := make([]byte, len(body)+12)
	copy(out, body)
	PutAngle(out[58:], g.SouthPoleLat)
	PutAngle(out[62:], g.SouthPoleLon)
	PutAngle(out[66:], g.Angle)
	return out
}

// Mercator — Grid Definition Template 3.10.
type Mercator struct {
	Nx, Ny   int
	La1, Lo1 float64
	La2, Lo2 float64
	LaD      float64 // latitude where Di, Dj apply
	Di, Dj   float64 // metres at LaD
	Scan     Scan
}

func NewMercator(nx, ny int, la1, lo1, la2, lo2, laD, di, dj float64) Mercator {
	return Mercator{
		Nx: nx, Ny: ny,
		La1: la1, Lo1: lo1,
		La2: la2, Lo2: lo2,
		LaD: laD,
		Di:  di, Dj: dj,
		Scan: NaturalScan(),
	}
}

func (g Mercator) TemplateNumber() uint16  { return 10 }
func (g Mercator) NumPoints() int          { return g.Nx * g.Ny }
func (g Mercator) NaturalSize() (int, int) { return g.Nx, g.Ny }
func (g Mercator) StorageIndex(i, j int) int {
	return g.Scan.RectIndex(g.Nx, g.Ny, i, j)
}
func (g Mercator) EncodeTemplate() []byte {
	t := make([]byte, 58)
	EarthShape6(t)
	binary.BigEndian.PutUint32(t[16:], uint32(g.Nx))
	binary.BigEndian.PutUint32(t[20:], uint32(g.Ny))
	PutAngle(t[24:], g.La1)
	PutAngle(t[28:], g.Lo1)
	t[32] = 0x30
	PutAngle(t[33:], g.LaD)
	PutAngle(t[37:], g.La2)
	PutAngle(t[41:], g.Lo2)
	t[45] = g.Scan.Byte()
	binary.BigEndian.PutUint32(t[46:], 0) // angle of orientation
	binary.BigEndian.PutUint32(t[50:], uint32(int32(math.Round(g.Di*1e3))))
	binary.BigEndian.PutUint32(t[54:], uint32(int32(math.Round(g.Dj*1e3))))
	return t
}

// Polar — Grid Definition Template 3.20 (polar stereographic).
type Polar struct {
	Nx, Ny   int
	La1, Lo1 float64
	LaD, LoV float64
	Dx, Dy   float64 // metres at LaD
	NorthHem bool
	Scan     Scan
}

func NewPolar(nx, ny int, la1, lo1, laD, loV, dx, dy float64, northHem bool) Polar {
	return Polar{
		Nx: nx, Ny: ny,
		La1: la1, Lo1: lo1,
		LaD: laD, LoV: loV,
		Dx: dx, Dy: dy,
		NorthHem: northHem,
		Scan:     NaturalScan(),
	}
}

func (g Polar) TemplateNumber() uint16  { return 20 }
func (g Polar) NumPoints() int          { return g.Nx * g.Ny }
func (g Polar) NaturalSize() (int, int) { return g.Nx, g.Ny }
func (g Polar) StorageIndex(i, j int) int {
	return g.Scan.RectIndex(g.Nx, g.Ny, i, j)
}
func (g Polar) EncodeTemplate() []byte {
	t := make([]byte, 51)
	EarthShape6(t)
	binary.BigEndian.PutUint32(t[16:], uint32(g.Nx))
	binary.BigEndian.PutUint32(t[20:], uint32(g.Ny))
	PutAngle(t[24:], g.La1)
	PutAngle(t[28:], g.Lo1)
	t[32] = 0x30
	PutAngle(t[33:], g.LaD)
	PutAngle(t[37:], g.LoV)
	binary.BigEndian.PutUint32(t[41:], uint32(int32(math.Round(g.Dx*1e3))))
	binary.BigEndian.PutUint32(t[45:], uint32(int32(math.Round(g.Dy*1e3))))
	if g.NorthHem {
		t[49] = 0x00
	} else {
		t[49] = 0x80
	}
	t[50] = g.Scan.Byte()
	return t
}

// Lambert — Grid Definition Template 3.30 (Lambert conformal conic).
type Lambert struct {
	Nx, Ny         int
	La1, Lo1       float64
	LaD, LoV       float64
	Dx, Dy         float64 // metres at LaD
	Latin1, Latin2 float64
	SouthPoleLat   float64
	SouthPoleLon   float64
	Scan           Scan
}

func NewLambert(nx, ny int, la1, lo1, laD, loV, dx, dy, latin1, latin2 float64) Lambert {
	return Lambert{
		Nx: nx, Ny: ny,
		La1: la1, Lo1: lo1,
		LaD: laD, LoV: loV,
		Dx: dx, Dy: dy,
		Latin1: latin1, Latin2: latin2,
		Scan: NaturalScan(),
	}
}

func (g Lambert) TemplateNumber() uint16  { return 30 }
func (g Lambert) NumPoints() int          { return g.Nx * g.Ny }
func (g Lambert) NaturalSize() (int, int) { return g.Nx, g.Ny }
func (g Lambert) StorageIndex(i, j int) int {
	return g.Scan.RectIndex(g.Nx, g.Ny, i, j)
}
func (g Lambert) EncodeTemplate() []byte {
	t := make([]byte, 67)
	EarthShape6(t)
	binary.BigEndian.PutUint32(t[16:], uint32(g.Nx))
	binary.BigEndian.PutUint32(t[20:], uint32(g.Ny))
	PutAngle(t[24:], g.La1)
	PutAngle(t[28:], g.Lo1)
	t[32] = 0x30
	PutAngle(t[33:], g.LaD)
	PutAngle(t[37:], g.LoV)
	binary.BigEndian.PutUint32(t[41:], uint32(int32(math.Round(g.Dx*1e3))))
	binary.BigEndian.PutUint32(t[45:], uint32(int32(math.Round(g.Dy*1e3))))
	t[49] = 0
	t[50] = g.Scan.Byte()
	PutAngle(t[51:], g.Latin1)
	PutAngle(t[55:], g.Latin2)
	PutAngle(t[59:], g.SouthPoleLat)
	PutAngle(t[63:], g.SouthPoleLon)
	return t
}

// Gaussian — Grid Definition Template 3.40, regular variant only.
//
// The decoder computes Gaussian latitudes from N (number of parallels between
// equator and pole), so the writer only encodes N and the longitude span;
// latitudes are not stored explicitly. The reduced variant requires a per-row
// pl[] table appended after the template — out of scope for this writer.
type Gaussian struct {
	Ni       int
	N        int     // number of latitudes between equator and pole; Nj = 2*N
	Lo1, Lo2 float64 // longitude span, degrees
	Scan     Scan
}

func NewGaussian(ni, n int, lo1, lo2 float64) Gaussian {
	return Gaussian{
		Ni: ni, N: n,
		Lo1: lo1, Lo2: lo2,
		Scan: NaturalScan(),
	}
}

func (g Gaussian) TemplateNumber() uint16  { return 40 }
func (g Gaussian) NumPoints() int          { return g.Ni * 2 * g.N }
func (g Gaussian) NaturalSize() (int, int) { return g.Ni, 2 * g.N }
func (g Gaussian) StorageIndex(i, j int) int {
	return g.Scan.RectIndex(g.Ni, 2*g.N, i, j)
}
func (g Gaussian) EncodeTemplate() []byte {
	t := make([]byte, 58)
	EarthShape6(t)
	binary.BigEndian.PutUint32(t[16:], uint32(g.Ni))
	binary.BigEndian.PutUint32(t[20:], uint32(2*g.N))
	binary.BigEndian.PutUint32(t[24:], 0)
	binary.BigEndian.PutUint32(t[28:], 0xffffffff)
	approx := 90.0 - 90.0/float64(2*g.N)
	PutAngle(t[32:], approx)
	PutAngle(t[36:], g.Lo1)
	t[40] = 0x30
	PutAngle(t[41:], -approx)
	PutAngle(t[45:], g.Lo2)
	// Same unsigned-magnitude guard as LatLon — Di is encoded as uint32 even
	// though the natural span (Lo2-Lo1) can be negative for unusual scans.
	binary.BigEndian.PutUint32(t[49:], uint32(math.Round(math.Abs(g.Lo2-g.Lo1)/float64(g.Ni-1)*1e6)))
	binary.BigEndian.PutUint32(t[53:], uint32(g.N))
	t[57] = g.Scan.Byte()
	return t
}
