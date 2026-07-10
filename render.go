package grib

import (
	"math"

	gridpkg "github.com/pspoerri/go-tiled-eccodes/grid"
	"github.com/pspoerri/go-tiled-eccodes/internal/bufpool"
	"github.com/pspoerri/go-tiled-eccodes/sample"
	"github.com/pspoerri/go-tiled-eccodes/tile"
)

// TileRequest fully describes a tile-render call. Width/Height default to 256
// when zero. ModeWindow is half-window radius for SampleMode == Mode (3x3
// window when 1, 5x5 when 2, etc.).
type TileRequest struct {
	Tile       tile.XYZ
	Width      int
	Height     int
	Sample     tile.SampleMode
	ModeWindow int
}

// ValueAt returns the message value at (lat, lon) using the requested
// sampler. The first call decodes the message; subsequent calls hit the
// cached buffer and complete in microseconds. Out-of-range coordinates
// return ErrOutOfBounds.
func (m *Message) ValueAt(lat, lon float64, mode tile.SampleMode) (float64, error) {
	src, err := m.decodeCached()
	if err != nil {
		return 0, err
	}
	g, err := m.Grid()
	if err != nil {
		return 0, err
	}
	fi, fj, ok := g.Locate(lat, lon)
	if !ok {
		return 0, ErrOutOfBounds
	}
	source := makeSource(g, src)
	fx := [1]float64{fi}
	fy := [1]float64{fj}
	out := [1]float64{}
	sample.Resample(toSampleMode(mode), source, fx[:], fy[:], out[:], 1)
	return out[0], nil
}

// RenderFloat64 fills dst (length req.W*req.H) with sampled values from this
// message at the requested WGS84 tile. Working buffers (fx, fy) are pooled.
func (m *Message) RenderFloat64(req TileRequest, dst []float64) error {
	w, h := tileDims(req)
	if len(dst) < w*h {
		return ErrShortBuffer
	}
	if g, err := m.Grid(); err == nil {
		if u, ok := g.(*gridpkg.Unstructured); ok && req.Sample != tile.Mode {
			// Unstructured grids resolve every pixel via 1-NN against the
			// mesh KD-tree. The (cellIdx) result depends only on the tile
			// geometry and the mesh, not on the values, so we cache it
			// once per (z, x, y, w, h) on the grid and replay it as a
			// flat slice lookup for every other (variable, timestep) on
			// the same mesh — the dominant cost of an icosahedral tile
			// render. Bicubic on unstructured already degenerates to
			// nearest, so the fast path is bit-equivalent for both.
			return m.renderUnstructuredTile(req, u, w, h, dst)
		}
	}
	tg := tile.Build(req.Tile, w, h)
	return m.renderLatLon(tg.Lats, tg.Lons, w, h, req.Sample, req.ModeWindow, dst)
}

// renderUnstructuredTile is the cached fast-path for unstructured-grid tiles.
// Skips per-pixel Locate (KD-tree NN) by reusing the cached cell-index map
// for the tile geometry, then does a flat lookup into the decoded buffer.
// Out-of-bounds and masked pixels (idx < 0) become NaN.
func (m *Message) renderUnstructuredTile(req TileRequest, u *gridpkg.Unstructured, w, h int, dst []float64) error {
	src, err := m.decodeCached()
	if err != nil {
		return err
	}
	indices, err := u.LocateTileIndices(req.Tile.Z, req.Tile.X, req.Tile.Y, w, h)
	if err != nil {
		return err
	}
	dst = dst[:w*h]
	for i, idx := range indices {
		if idx < 0 || int(idx) >= len(src) {
			dst[i] = math.NaN()
			continue
		}
		dst[i] = src[idx]
	}
	return nil
}

// renderLatLon is the shared inner loop for tile and region rendering. lats
// has length h (one entry per output row, north → south). lons has length w
// (one entry per output column, west → east). dst must be at least w*h.
func (m *Message) renderLatLon(lats, lons []float64, w, h int, mode tile.SampleMode, modeWindow int, dst []float64) error {
	dst = dst[:w*h]
	src, err := m.decodeCached()
	if err != nil {
		return err
	}
	g, err := m.Grid()
	if err != nil {
		return err
	}
	// Bicubic on an unstructured grid silently degenerates to nearest:
	// Locate returns (cellIdx, 0) so any (anything, ±1) stencil read in
	// sample/sample.go falls outside the grid's only row and triggers
	// the NaN-fallback to the central nearest read. The output is
	// identical to Nearest mode but pays an extra source(i, ±1) NaN-
	// returning call per pixel. Force Nearest so the sampler skips
	// the dead 4×4 stencil setup entirely.
	if _, isUnstructured := g.(*gridpkg.Unstructured); isUnstructured && mode == tile.Bicubic {
		mode = tile.Nearest
	}

	fx := bufpool.P.GetF64(w * h)
	fy := bufpool.P.GetF64(w * h)
	defer bufpool.P.PutF64(fx)
	defer bufpool.P.PutF64(fy)
	fx = fx[:w*h]
	fy = fy[:w*h]

	idx := 0
	for j := 0; j < h; j++ {
		lat := lats[j]
		for i := 0; i < w; i++ {
			fi, fj, ok := g.Locate(lat, lons[i])
			if !ok {
				// Sentinel out-of-bounds: integer coords below zero force
				// the resampler's edge handling to surface NaN.
				fx[idx] = -1
				fy[idx] = -1
			} else {
				fx[idx] = fi
				fy[idx] = fj
			}
			idx++
		}
	}

	source := makeSource(g, src)
	sample.Resample(toSampleMode(mode), source, fx, fy, dst, modeWindow)
	return nil
}

// makeSource returns a sample.Source closure that maps integer source pixel
// (i, j) coordinates into the decoded buffer.
//
// Hot path: for any rectangular grid (LatLon, RotatedLatLon, Mercator,
// Lambert, Polar, regular Gaussian) we capture (Ni, Nj, scanning bits) into
// the closure and inline the address arithmetic. This avoids the interface
// method call to Grid.Index that doubled tile-render latency for ICON-style
// J-positive storage.
//
// Cold path: reduced Gaussian and any future grid that doesn't expose a
// rectangular layout falls back to Grid.Index per source read.
func makeSource(g gridpkg.Grid, buf []float64) sample.Source {
	if r, ok := rectLayout(g); ok && r.consecutive && !r.alternate {
		ni, nj := r.ni, r.nj
		ipos, jpos := r.iPositive, r.jPositive
		switch {
		case ipos && !jpos:
			return func(i, j int) float64 {
				if uint(i) >= uint(ni) || uint(j) >= uint(nj) {
					return math.NaN()
				}
				return buf[j*ni+i]
			}
		case ipos && jpos:
			return func(i, j int) float64 {
				if uint(i) >= uint(ni) || uint(j) >= uint(nj) {
					return math.NaN()
				}
				return buf[(nj-1-j)*ni+i]
			}
		case !ipos && !jpos:
			return func(i, j int) float64 {
				if uint(i) >= uint(ni) || uint(j) >= uint(nj) {
					return math.NaN()
				}
				return buf[j*ni+(ni-1-i)]
			}
		default: // !ipos && jpos
			return func(i, j int) float64 {
				if uint(i) >= uint(ni) || uint(j) >= uint(nj) {
					return math.NaN()
				}
				return buf[(nj-1-j)*ni+(ni-1-i)]
			}
		}
	}
	return func(i, j int) float64 {
		off := g.Index(i, j)
		if off < 0 {
			return math.NaN()
		}
		return buf[off]
	}
}

// rect captures the scanning-mode + dimensions needed by makeSource's fast
// path. Non-rectangular grids return ok=false.
type rect struct {
	ni, nj                                       int
	iPositive, jPositive, consecutive, alternate bool
}

func rectLayout(g gridpkg.Grid) (rect, bool) {
	switch gg := g.(type) {
	case gridpkg.LatLon:
		return rect{gg.Ni, gg.Nj, gg.IPositive, gg.JPositive, gg.Consecutive, gg.Alternate}, true
	case gridpkg.RotatedLatLon:
		return rect{gg.Ni, gg.Nj, gg.IPositive, gg.JPositive, gg.Consecutive, gg.Alternate}, true
	case gridpkg.Mercator:
		return rect{gg.Nx, gg.Ny, gg.IPositive, gg.JPositive, gg.Consecutive, gg.Alternate}, true
	case gridpkg.Lambert:
		return rect{gg.Nx, gg.Ny, gg.IPositive, gg.JPositive, gg.Consecutive, gg.Alternate}, true
	case gridpkg.Polar:
		return rect{gg.Nx, gg.Ny, gg.IPositive, gg.JPositive, gg.Consecutive, gg.Alternate}, true
	case gridpkg.Gaussian:
		if gg.Reduced {
			return rect{}, false
		}
		return rect{gg.Ni, len(gg.Lats), gg.IPositive, gg.JPositive, gg.Consecutive, gg.Alternate}, true
	case gridpkg.RotatedGaussian:
		if gg.Reduced {
			return rect{}, false
		}
		return rect{gg.Ni, len(gg.Lats), gg.IPositive, gg.JPositive, gg.Consecutive, gg.Alternate}, true
	case gridpkg.StretchedGaussian:
		if gg.Reduced {
			return rect{}, false
		}
		return rect{gg.Ni, len(gg.Lats), gg.IPositive, gg.JPositive, gg.Consecutive, gg.Alternate}, true
	case gridpkg.StretchedRotatedGaussian:
		if gg.Reduced {
			return rect{}, false
		}
		return rect{gg.Ni, len(gg.Lats), gg.IPositive, gg.JPositive, gg.Consecutive, gg.Alternate}, true
	}
	return rect{}, false
}

// Region describes a rectangular WGS84 sampling window for RenderRegion.
//
// The output buffer is laid out row-major (row 0 = north, column 0 = west)
// with Width columns and Height rows. Each cell is centred at the midpoint
// of its (lat, lon) sub-rectangle:
//
//	lon[i] = West  + (East  - West)  * (i + 0.5) / Width
//	lat[j] = North + (South - North) * (j + 0.5) / Height
//
// This is a Plate-Carrée style sample grid, suitable for "sample the grid
// at a regular lat/lon spacing" use cases (data tile / GeoJSON grid /
// contour extraction). For Mercator-style XYZ tiles, use TileRequest +
// RenderFloat64/32 instead — those preserve the Mercator y curvature
// within each tile.
//
// All four extents are in degrees. East may be ≤ West to cross the
// antimeridian, in which case the sampler internally normalises lon into
// [West, West+360).
type Region struct {
	South, West   float64
	North, East   float64
	Width, Height int
	Sample        tile.SampleMode
	ModeWindow    int
}

// RenderRegionFloat64 fills dst (length r.Width*r.Height) with sampled
// values across an arbitrary WGS84 bounding box. Suitable for the
// sample-on-a-bbox use cases that don't fit the XYZ tile path: GeoJSON
// data grids, contour-extraction backing buffers, animation tile chunks.
func (m *Message) RenderRegionFloat64(r Region, dst []float64) error {
	w, h := regionDims(r)
	if len(dst) < w*h {
		return ErrShortBuffer
	}
	lats, lons := regionLatLons(r, w, h)
	return m.renderLatLon(lats, lons, w, h, r.Sample, r.ModeWindow, dst)
}

// RenderRegionFloat32 mirrors RenderRegionFloat64 but writes float32.
func (m *Message) RenderRegionFloat32(r Region, dst []float32) error {
	w, h := regionDims(r)
	if len(dst) < w*h {
		return ErrShortBuffer
	}
	tmp := bufpool.P.GetF64(w * h)
	defer bufpool.P.PutF64(tmp)
	tmp = tmp[:w*h]
	if err := m.RenderRegionFloat64(r, tmp); err != nil {
		return err
	}
	for i, v := range tmp {
		dst[i] = float32(v)
	}
	return nil
}

func regionDims(r Region) (int, int) {
	w, h := r.Width, r.Height
	if w <= 0 {
		w = 256
	}
	if h <= 0 {
		h = 256
	}
	return w, h
}

func regionLatLons(r Region, w, h int) ([]float64, []float64) {
	lons := make([]float64, w)
	lonSpan := r.East - r.West
	if lonSpan <= 0 {
		lonSpan += 360
	}
	for i := 0; i < w; i++ {
		lons[i] = r.West + lonSpan*(float64(i)+0.5)/float64(w)
	}
	lats := make([]float64, h)
	for j := 0; j < h; j++ {
		lats[j] = r.North + (r.South-r.North)*(float64(j)+0.5)/float64(h)
	}
	return lats, lons
}

// RenderFloat32 mirrors RenderFloat64 but writes float32. The intermediate
// float64 scratch buffer is pooled.
func (m *Message) RenderFloat32(req TileRequest, dst []float32) error {
	w, h := tileDims(req)
	if len(dst) < w*h {
		return ErrShortBuffer
	}
	tmp := bufpool.P.GetF64(w * h)
	defer bufpool.P.PutF64(tmp)
	tmp = tmp[:w*h]
	if err := m.RenderFloat64(req, tmp); err != nil {
		return err
	}
	for i, v := range tmp {
		dst[i] = float32(v)
	}
	return nil
}

// RenderInt8 quantizes the resampled float64 values via q.
func (m *Message) RenderInt8(req TileRequest, q tile.Quantize, dst []int8) error {
	return renderInt[int8](m, req, q, dst, math.MinInt8, math.MaxInt8)
}
func (m *Message) RenderInt16(req TileRequest, q tile.Quantize, dst []int16) error {
	return renderInt[int16](m, req, q, dst, math.MinInt16, math.MaxInt16)
}
func (m *Message) RenderInt32(req TileRequest, q tile.Quantize, dst []int32) error {
	return renderInt[int32](m, req, q, dst, math.MinInt32, math.MaxInt32)
}
func (m *Message) RenderInt64(req TileRequest, q tile.Quantize, dst []int64) error {
	return renderInt[int64](m, req, q, dst, math.MinInt64, math.MaxInt64)
}
func (m *Message) RenderUint8(req TileRequest, q tile.Quantize, dst []uint8) error {
	return renderUint[uint8](m, req, q, dst, math.MaxUint8)
}
func (m *Message) RenderUint16(req TileRequest, q tile.Quantize, dst []uint16) error {
	return renderUint[uint16](m, req, q, dst, math.MaxUint16)
}
func (m *Message) RenderUint32(req TileRequest, q tile.Quantize, dst []uint32) error {
	return renderUint[uint32](m, req, q, dst, math.MaxUint32)
}
func (m *Message) RenderUint64(req TileRequest, q tile.Quantize, dst []uint64) error {
	return renderUint[uint64](m, req, q, dst, math.MaxUint64)
}

type signedInt interface {
	~int8 | ~int16 | ~int32 | ~int64
}
type unsignedInt interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}

func renderInt[T signedInt](m *Message, req TileRequest, q tile.Quantize, dst []T, tmin, tmax int64) error {
	w, h := tileDims(req)
	if len(dst) < w*h {
		return ErrShortBuffer
	}
	tmp := bufpool.P.GetF64(w * h)
	defer bufpool.P.PutF64(tmp)
	tmp = tmp[:w*h]
	if err := m.RenderFloat64(req, tmp); err != nil {
		return err
	}
	lo, hi := q.Min, q.Max
	if lo == 0 && hi == 0 {
		lo, hi = float64(tmin), float64(tmax)
	}
	miss := T(q.MissingValue)
	// float64 cannot exactly represent math.MaxInt64 / math.MinInt64; the
	// nearest representable values are ±2^63, which overflow on int64 cast.
	// Saturate at the type bounds before the conversion.
	for i, v := range tmp {
		if math.IsNaN(v) {
			dst[i] = miss
			continue
		}
		x := math.Round((v - q.Offset) * q.Scale)
		if x < lo {
			x = lo
		} else if x > hi {
			x = hi
		}
		if x >= float64(tmax) {
			dst[i] = T(tmax)
			continue
		}
		if x <= float64(tmin) {
			dst[i] = T(tmin)
			continue
		}
		dst[i] = T(int64(x))
	}
	return nil
}

func renderUint[T unsignedInt](m *Message, req TileRequest, q tile.Quantize, dst []T, tmax uint64) error {
	w, h := tileDims(req)
	if len(dst) < w*h {
		return ErrShortBuffer
	}
	tmp := bufpool.P.GetF64(w * h)
	defer bufpool.P.PutF64(tmp)
	tmp = tmp[:w*h]
	if err := m.RenderFloat64(req, tmp); err != nil {
		return err
	}
	lo, hi := q.Min, q.Max
	if lo == 0 && hi == 0 {
		hi = float64(tmax)
	}
	miss := T(q.MissingValue)
	// float64 cannot exactly represent math.MaxUint64; rounding bumps it to
	// 2^64, and uint64(2^64) is implementation-defined. Saturate at the type
	// bound before the conversion, and at zero on the low side (uint cast of
	// a negative float is also implementation-defined).
	for i, v := range tmp {
		if math.IsNaN(v) {
			dst[i] = miss
			continue
		}
		x := math.Round((v - q.Offset) * q.Scale)
		if x < lo {
			x = lo
		} else if x > hi {
			x = hi
		}
		if x >= float64(tmax) {
			dst[i] = T(tmax)
			continue
		}
		if x <= 0 {
			dst[i] = 0
			continue
		}
		dst[i] = T(uint64(x))
	}
	return nil
}

func tileDims(req TileRequest) (int, int) {
	w, h := req.Width, req.Height
	if w <= 0 {
		w = 256
	}
	if h <= 0 {
		h = 256
	}
	return w, h
}

func toSampleMode(m tile.SampleMode) sample.Mode {
	switch m {
	case tile.Bicubic:
		return sample.Bicubic
	case tile.Mode:
		return sample.ModeFilter
	default:
		return sample.Nearest
	}
}
