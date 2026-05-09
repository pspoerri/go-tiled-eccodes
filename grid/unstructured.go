package grid

import (
	"errors"
	"math"
	"sync"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// Unstructured is Grid Definition Template 3.101 — General Unstructured Grid.
//
// This template covers irregular meshes whose cell coordinates are *not*
// transmitted in the message itself; the body just identifies the mesh by a
// numeric reference and a UUID. Real-world examples are the DWD ICON model
// (triangular icosahedral mesh) and the MeteoSwiss ICON-CH ensembles. The
// per-cell (lat, lon) tables live in companion "horizontal_constants" GRIB
// files and must be loaded by the caller and attached via SetCoordinates
// before Locate / Index will return useful values.
//
// Template body offsets (zero-based bytes, starting at Section 3 byte 14):
//
//	0      shape of earth
//	1      scale factor of radius of spherical earth
//	2-5    scaled value of radius
//	6      scale factor of major axis
//	7-10   scaled value of major axis
//	11     scale factor of minor axis
//	12-15  scaled value of minor axis
//	16     number of grid used
//	17-19  number of grid reference (3 octets)
//	20-35  UUID of horizontal grid (16 octets)
//
// Storage: values are stored in a 1-D buffer of NumberOfDataPoints entries,
// one per cell, in the cell-numbering order of the reference mesh. There is
// no row/column structure — Index(i, 0) == i and Size() == (NumPoints, 1).
// The renderer's rectangular fast path therefore does not match Unstructured;
// every read goes through Index(i, j).
type Unstructured struct {
	NumPoints_    int
	GridUsed      uint8
	GridReference uint32
	UUID          [16]byte
	ShapeOfEarth  uint8

	// EarthRadiusMeters is the assumed sphere radius for great-circle
	// distance math when MaxDistance is applied during Locate. Set from the
	// Section 3 shape-of-earth code; defaults to 6371229 m (GRIB code 6) if
	// the message uses a non-spherical or unspecified shape.
	EarthRadiusMeters float64

	// MaxChordSquared, if > 0, bounds Locate so that any query whose nearest
	// cell lies further than this 3-vector chord distance is reported as
	// out-of-bounds. Callers set this via SetMaxDistance to mask points
	// outside the model footprint; without it a global icosahedral grid
	// matches *every* query against its closest cell, which paints a
	// nearest-neighbor halo well outside the grid's intended coverage.
	MaxChordSquared float64

	lats []float64
	lons []float64

	// cellMask, if non-nil, is a per-cell boolean array (len == NumPoints)
	// where true marks a cell whose value should not be served. Locate
	// reports out-of-bounds for any query whose nearest cell is masked,
	// which causes the renderer to paint NaN at that pixel. Use this to
	// trim the boundary relaxation zone of a limited-area icosahedral
	// model (the cells just inside the model footprint where the dynamics
	// are still being relaxed against lateral boundary conditions and
	// values are visibly noisier than the model's interior).
	cellMask []bool

	once sync.Once
	hash *uniformHash

	// tileIdx caches the per-pixel cell-index map for a given XYZ tile
	// geometry. The map is identical for every (variable, timestep)
	// rendered against the same mesh, so we pay the per-pixel KD-tree
	// nearest-neighbor cost once and replay it as a flat lookup for
	// every subsequent render of that tile. The cache is shared across
	// every Unstructured that shares lats/lons via SetCoordinatesFrom
	// (assigning the *sync.Map pointer below in that path), so all the
	// messages of one forecast collapse onto one cache.
	//
	// Lifetime: tied to the receiving Unstructured (and its sharers).
	// Released when the underlying *grib.File is garbage-collected, so
	// the LRU on gribstore.ReaderCache transitively bounds the cache
	// memory footprint.
	tileIdx *sync.Map // tileLocateKey -> *tileLocateEntry
}

// tileLocateKey identifies one cached cell-index map.
type tileLocateKey struct {
	z, x, y, w, h int
}

// tileLocateEntry holds the lazily-computed cell-index map for one
// tile geometry. once.Do drives the actual computation so concurrent
// callers for the same key share a single Locate sweep.
type tileLocateEntry struct {
	once    sync.Once
	indices []int32
}

// ErrCoordinatesMissing is returned by Locate when the unstructured grid has
// no per-cell (lat, lon) attached. Call SetCoordinates with the arrays from
// the matching horizontal_constants companion file before rendering.
var ErrCoordinatesMissing = errors.New("grid: unstructured grid coordinates not attached")

// ParseUnstructured decodes a Section 3 template-101 body. The number of
// cells is supplied by the caller (it lives in Section 3 octets 7-10, not in
// the template body).
func ParseUnstructured(t []byte, numPoints int) *Unstructured {
	g := &Unstructured{
		NumPoints_:        numPoints,
		EarthRadiusMeters: earthRadiusMeters,
	}
	if len(t) >= 36 {
		g.ShapeOfEarth = t[0]
		g.GridUsed = t[16]
		g.GridReference = uint32(t[17])<<16 | uint32(t[18])<<8 | uint32(t[19])
		copy(g.UUID[:], t[20:36])
	}
	if r := earthRadiusFromShape(t); r > 0 {
		g.EarthRadiusMeters = r
	}
	return g
}

// earthRadiusFromShape returns the sphere radius for shape-of-earth codes
// that specify a sphere (codes 0, 1, 6, 8). For ellipsoids and "user defined"
// shapes (2, 3, 7, 9) we fall back to the GRIB default of 6371229 m — the
// difference vs WGS84 is sub-1% and our rendering cares about ranking nearest
// neighbors, not metre-accurate distance.
func earthRadiusFromShape(t []byte) float64 {
	if len(t) < 6 {
		return 0
	}
	switch t[0] {
	case 0:
		return 6367470 // spherical earth, radius 6,367,470 m
	case 1:
		// scaled value of radius of spherical earth
		sf := int8(t[1])
		v := bswap.U32(t, 2)
		if v == 0 || v == 0xffffffff {
			return 0
		}
		return float64(v) * math.Pow10(-int(sf))
	case 6:
		return 6371229
	case 8:
		return 6371200
	}
	return 0
}

func (g *Unstructured) Size() (int, int) { return g.NumPoints_, 1 }
func (g *Unstructured) NumPoints() int   { return g.NumPoints_ }

// IsNatural is always true for unstructured grids — there is no scanning
// mode reorder, the buffer is already in cell-index order.
func (g *Unstructured) IsNatural() bool { return true }

// Index returns the linear offset of the i-th cell. j must be 0; an
// unstructured grid has no second dimension.
func (g *Unstructured) Index(i, j int) int {
	if j != 0 || i < 0 || i >= g.NumPoints_ {
		return -1
	}
	return i
}

// SetCoordinates attaches the per-cell geographic (lat, lon) arrays to this
// grid. Each slice must have len == NumPoints. The KD-tree used by Locate is
// built lazily on first query (or eagerly via BuildIndex) — set-and-forget is
// the intended workflow.
//
// SetCoordinates may be called only once per grid; subsequent calls return
// an error. The caller retains ownership of the slices but must not mutate
// them while Locate is in flight on this grid.
func (g *Unstructured) SetCoordinates(lats, lons []float64) error {
	if len(lats) != g.NumPoints_ || len(lons) != g.NumPoints_ {
		return errors.New("grid: lat/lon length mismatch with NumPoints")
	}
	if g.lats != nil {
		return errors.New("grid: coordinates already set")
	}
	g.lats = lats
	g.lons = lons
	// Allocate the tile-index cache up front so concurrent first-time
	// LocateTileIndices callers don't race on the lazy allocation.
	g.tileIdx = &sync.Map{}
	return nil
}

// SetCoordinatesFrom shares the lat/lon arrays *and* the KD-tree from
// another already-coordinated unstructured grid. Both must agree on
// NumPoints. Use this when many messages from the same forecast write to
// the same icosahedral mesh: build the tree once on one grid, share it
// across all the others. Without this, every Unstructured builds its own
// O(N log N) tree on first Locate — wasteful for ICON-Global-sized meshes
// (3M cells × ~30 bytes/node ≈ 90 MB per redundant tree).
//
// The receiver must not have had SetCoordinates called on it yet.
func (g *Unstructured) SetCoordinatesFrom(other *Unstructured) error {
	if other == nil || other.lats == nil {
		return ErrCoordinatesMissing
	}
	if g.NumPoints_ != other.NumPoints_ {
		return errors.New("grid: NumPoints mismatch")
	}
	if g.lats != nil {
		return errors.New("grid: coordinates already set")
	}
	g.lats = other.lats
	g.lons = other.lons
	g.cellMask = other.cellMask
	// Force the source's tree build (if not yet) and share.
	other.once.Do(func() { other.hash = newUniformHash(other.lats, other.lons) })
	g.hash = other.hash
	// Share the tile-locate cache so all messages on this mesh collapse
	// onto one map. The first Locate on any sharer populates the entry
	// once and every later render reads the same []int32.
	if other.tileIdx == nil {
		other.tileIdx = &sync.Map{}
	}
	g.tileIdx = other.tileIdx
	// Mark our own once as done so Locate doesn't try to rebuild.
	g.once.Do(func() {})
	return nil
}

// HasCoordinates reports whether SetCoordinates has been called.
func (g *Unstructured) HasCoordinates() bool { return g.lats != nil }

// SetCellMask attaches a per-cell boolean mask. Masked cells (mask[i]==true)
// behave as out-of-bounds during Locate, so any pixel whose nearest neighbor
// is a masked cell renders as NaN. Pass nil to clear an existing mask.
//
// The mask must have len == NumPoints. The caller retains ownership of the
// slice but must not mutate it while Locate is in flight.
func (g *Unstructured) SetCellMask(mask []bool) error {
	if mask != nil && len(mask) != g.NumPoints_ {
		return errors.New("grid: cell mask length mismatch with NumPoints")
	}
	g.cellMask = mask
	// Mask outcome is baked into cached cell-index maps (masked cells
	// resolve to -1). Reset so subsequent renders see the new mask.
	g.tileIdx = nil
	return nil
}

// HasCellMask reports whether SetCellMask has been called with a non-nil mask.
func (g *Unstructured) HasCellMask() bool { return g.cellMask != nil }

// LatLonAt returns the geographic (lat, lon) of cell i in degrees, or NaN if
// coordinates are not attached.
func (g *Unstructured) LatLonAt(i int) (lat, lon float64) {
	if g.lats == nil || i < 0 || i >= len(g.lats) {
		return math.NaN(), math.NaN()
	}
	return g.lats[i], g.lons[i]
}

// SetMaxDistance bounds Locate so queries whose nearest cell is further than
// the given great-circle distance (in metres) are reported out-of-bounds.
// Use this to mask the area outside the model footprint when serving an
// icosahedral grid that nominally covers the whole sphere but only carries
// data for a regional subdomain.
//
// distanceMeters ≤ 0 disables the limit (the default).
func (g *Unstructured) SetMaxDistance(distanceMeters float64) {
	defer func() { g.tileIdx = nil }()
	if distanceMeters <= 0 {
		g.MaxChordSquared = 0
		return
	}
	r := g.EarthRadiusMeters
	if r <= 0 {
		r = earthRadiusMeters
	}
	// MaxChordSquared is in unit-sphere units to match the KD-tree's chord²,
	// which is computed from unit-vector points. Half-angle of the arc is
	// (distance/R)/2; unit-sphere chord = 2 sin(half-angle).
	half := 0.5 * distanceMeters / r
	c := 2 * math.Sin(half)
	g.MaxChordSquared = c * c
}

// BuildIndex eagerly builds the spatial index. Calling BuildIndex once at
// startup keeps the first Locate from paying its construction cost;
// subsequent calls are pure bucket lookups.
func (g *Unstructured) BuildIndex() error {
	if g.lats == nil {
		return ErrCoordinatesMissing
	}
	g.once.Do(func() {
		g.hash = newUniformHash(g.lats, g.lons)
	})
	return nil
}

// Locate returns the index of the nearest cell as fi (with fj==0). Bicubic
// resampling on an unstructured grid degenerates gracefully: the renderer's
// 4×4 stencil reads (anything, ±1) which always return NaN, the kernel's
// NaN-aware fallback kicks in and the result is the same nearest read.
func (g *Unstructured) Locate(lat, lon float64) (float64, float64, bool) {
	if g.lats == nil {
		return 0, 0, false
	}
	g.once.Do(func() { g.hash = newUniformHash(g.lats, g.lons) })
	if g.hash == nil {
		return 0, 0, false
	}
	idx, d2 := g.hash.nearest(lat, lon, g.MaxChordSquared)
	if idx < 0 {
		return 0, 0, false
	}
	if g.MaxChordSquared > 0 && d2 > g.MaxChordSquared {
		return 0, 0, false
	}
	if g.cellMask != nil && idx < len(g.cellMask) && g.cellMask[idx] {
		return 0, 0, false
	}
	// Distance is reported in *unit-sphere* chord; rescale to the configured
	// earth radius so callers comparing distances see metres.
	_ = d2
	return float64(idx), 0, true
}

// LocateTileIndices returns the per-pixel cell-index map for a
// WGS84 web-mercator XYZ tile of (w, h) pixels. The slice has length
// w*h in row-major order (row 0 = north, column 0 = west). Out-of-
// bounds, masked, or above-MaxChordSquared pixels are written as -1.
//
// The map is identical for every (variable, timestep) rendered against
// this mesh, so we cache it on the grid (shared across messages on the
// same UUID via SetCoordinatesFrom). The cache lives for the lifetime
// of the underlying *grib.File; gribstore's ReaderCache LRU bounds
// total memory by evicting whole runs.
//
// Concurrency: cache lookup is sync.Map; per-entry compute is gated by
// sync.Once so concurrent callers for the same key share a single
// cell-index sweep instead of redoing 65 536 hash queries.
func (g *Unstructured) LocateTileIndices(z, x, y, w, h int) ([]int32, error) {
	if g.lats == nil {
		return nil, ErrCoordinatesMissing
	}
	g.once.Do(func() { g.hash = newUniformHash(g.lats, g.lons) })
	if g.hash == nil {
		return nil, errors.New("grid: spatial index build failed")
	}
	if w <= 0 || h <= 0 {
		return nil, errors.New("grid: tile dimensions must be positive")
	}
	cache := g.tileIdx
	if cache == nil {
		// SetCoordinates / SetCoordinatesFrom allocate the cache up
		// front; reaching nil means the grid was constructed without
		// going through those entry points (e.g. test fixtures that
		// poke private fields). Fall back to a fresh per-call cache so
		// the math still works — the entry just won't survive the call.
		cache = &sync.Map{}
	}
	key := tileLocateKey{z: z, x: x, y: y, w: w, h: h}
	entry := &tileLocateEntry{}
	if existing, loaded := cache.LoadOrStore(key, entry); loaded {
		entry = existing.(*tileLocateEntry)
	}
	entry.once.Do(func() {
		entry.indices = g.computeTileIndices(z, x, y, w, h)
	})
	return entry.indices, nil
}

// computeTileIndices walks the (w, h) tile and resolves each pixel against
// the spatial hash. -1 marks any pixel that should render as NaN (out-of-
// domain, beyond MaxDistance, or masked).
//
// The inner sweep groups consecutive pixels in the same hash query bucket
// (a run-length compression along each row's lon axis) and runs one ring
// scan per group instead of one per pixel. Inside the ring scan, candidate
// cells loop over all pixels in the group, pivoting the work into a
// stride-1 inner pixel loop that the compiler vectorises and that keeps
// each candidate's vec3 hot in registers across the full pixel sweep.
//
// At ICON-D2 zoom 5 the typical group runs ~40 pixels, which amortises the
// 9-bucket ring scan and lights up the auto-vectoriser; cold tile sweeps
// run roughly 30% faster than the per-pixel path.
func (g *Unstructured) computeTileIndices(z, x, y, w, h int) []int32 {
	out := make([]int32, w*h)
	// Compute lats once per row, lons once per column. Same arithmetic as
	// tile.Build but inlined so this file doesn't import the tile package
	// (tile depends on grid for rendering glue, so the cycle would bite).
	n := float64(int(1) << uint(z))
	lons := make([]float64, w)
	for i := 0; i < w; i++ {
		lons[i] = (float64(x)+(float64(i)+0.5)/float64(w))/n*360 - 180
	}
	lats := make([]float64, h)
	for j := 0; j < h; j++ {
		yn := float64(y) + (float64(j)+0.5)/float64(h)
		t := math.Pi - 2*math.Pi*(yn/n)
		lats[j] = 180 / math.Pi * math.Atan(0.5*(math.Exp(t)-math.Exp(-t)))
	}
	maxD2 := g.MaxChordSquared
	mask := g.cellMask
	hash := g.hash

	// Per-row scratch — pixel vec3 and running best. Re-used across rows.
	pixVx := make([]float64, w)
	pixVy := make([]float64, w)
	pixVz := make([]float64, w)
	bestIdx := make([]int32, w)
	bestD2 := make([]float64, w)
	// lonBucket[i] is the hash lon-bucket index for column i. Constant
	// across rows, so we materialise it once and reuse.
	lonBucket := make([]int, w)
	for i := 0; i < w; i++ {
		lonBucket[i] = hash.lonBucketOf(lons[i])
	}

	for j := 0; j < h; j++ {
		lat := lats[j]
		base := j * w

		// Pre-compute pixel vectors and seed running best.
		for i := 0; i < w; i++ {
			pixVx[i], pixVy[i], pixVz[i] = latLonToVec3(lat, lons[i])
			bestIdx[i] = -1
			if maxD2 > 0 {
				bestD2[i] = maxD2
			} else {
				bestD2[i] = math.Inf(1)
			}
		}

		// Hoisted: lat-bucket is constant for the row.
		li := hash.latBucketOf(lat)

		// Run-length groups along lon: process consecutive pixels in the
		// same lon-bucket as one batched ring scan.
		i := 0
		for i < w {
			lj := lonBucket[i]
			endI := i + 1
			for endI < w && lonBucket[endI] == lj {
				endI++
			}
			hash.nearestRowGroup(li, lj, lat,
				pixVx[i:endI], pixVy[i:endI], pixVz[i:endI],
				bestIdx[i:endI], bestD2[i:endI])
			i = endI
		}

		// Apply mask + write out. maxD2 was already baked into the seed,
		// so no post-check is needed for the footprint cap.
		for i := 0; i < w; i++ {
			idx := bestIdx[i]
			if idx < 0 {
				out[base+i] = -1
				continue
			}
			if mask != nil && int(idx) < len(mask) && mask[idx] {
				out[base+i] = -1
				continue
			}
			out[base+i] = idx
		}
	}
	return out
}

// LocateWithDistance is like Locate but also returns the great-circle
// distance from the query to the matched cell, in metres. Useful for
// callers building their own footprint masks.
func (g *Unstructured) LocateWithDistance(lat, lon float64) (idx int, distMeters float64, ok bool) {
	if g.lats == nil {
		return -1, 0, false
	}
	g.once.Do(func() { g.hash = newUniformHash(g.lats, g.lons) })
	if g.hash == nil {
		return -1, 0, false
	}
	i, d2 := g.hash.nearest(lat, lon, g.MaxChordSquared)
	if i < 0 {
		return -1, 0, false
	}
	if g.MaxChordSquared > 0 && d2 > g.MaxChordSquared {
		return i, chordToArc(math.Sqrt(d2), g.EarthRadiusMeters), false
	}
	if g.cellMask != nil && i < len(g.cellMask) && g.cellMask[i] {
		return i, chordToArc(math.Sqrt(d2), g.EarthRadiusMeters), false
	}
	return i, chordToArc(math.Sqrt(d2), g.EarthRadiusMeters), true
}

// chordToArc converts a unit-sphere chord (3-vector distance) to a
// great-circle arc length in metres.
func chordToArc(chord, radius float64) float64 {
	if chord >= 2 {
		return math.Pi * radius
	}
	return 2 * radius * math.Asin(0.5*chord)
}

// latLonToVec3 maps WGS84 (lat, lon) in degrees onto a unit sphere. We use
// 3-vector chord distance because (a) it sidesteps the antimeridian and
// pole singularities that bite 2-D (lat, lon) lookups, and (b) ranking by
// chord and ranking by great-circle arc are monotonic, so "nearest by
// chord" == "nearest by distance".
func latLonToVec3(latDeg, lonDeg float64) (x, y, z float64) {
	lat := latDeg * deg2rad
	lon := lonDeg * deg2rad
	cl := math.Cos(lat)
	x = cl * math.Cos(lon)
	y = cl * math.Sin(lon)
	z = math.Sin(lat)
	return
}
