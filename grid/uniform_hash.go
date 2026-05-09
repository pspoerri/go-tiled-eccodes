package grid

import "math"

// uniformHash is a uniform 2-D lat/lon spatial hash for nearest-neighbor
// queries on an unstructured / icosahedral mesh.
//
// Each model's mesh is fixed and known (identified by its UUID and shipped
// as a horizontal_constants companion file), so the hash is built once per
// mesh and shared across every (variable, timestep) message that references
// it — the same lifetime as the existing tile-index cache.
//
// Layout: CSR. cells[offsets[b] : offsets[b+1]] lists the cell indices of
// bucket b in arbitrary order. The bucket grid is uniform in (lat, lon)
// degrees with nLat × nLon = nLat × 2*nLat cells, sized so that an equatorial
// bucket holds ≈ hashTargetCellsPerBucket cells. Polar buckets are sparser
// by a factor of cos(lat) (their spherical area shrinks toward the poles);
// queries near the pole are handled by reflecting the search neighborhood
// across the pole and shifting longitudes by 180°, so cross-pole neighbors
// are visited at small ring radii instead of requiring a half-globe lon
// sweep.
//
// Distance ranking uses 3-vector chord-squared on the unit sphere, matching
// the pre-existing KD-tree metric. Chord ranking is monotonic with great-
// circle arc length on a sphere of constant radius, so "nearest by chord"
// ≡ "nearest by distance".
type uniformHash struct {
	nLat, nLon       int
	invDLat, invDLon float64 // bucket-stride reciprocals (degrees⁻¹)

	offsets []int32 // length nLat*nLon + 1
	cells   []int32 // length len(lats), in bucket order

	vx, vy, vz []float64 // per-cell unit vectors, indexed by original cell id
}

// hashTargetCellsPerBucket is the average occupancy we aim for in an
// equatorial bucket. Larger values shrink the offsets table at the cost of
// more cells visited per query; smaller values do the opposite. 8 keeps the
// offsets table well under a megabyte even for ICON-Global (≈3 M cells)
// while leaving the inner distance loop short enough to vectorize cleanly.
const hashTargetCellsPerBucket = 8

// newUniformHash builds the 2-D spatial index from per-cell (lat, lon) arrays.
// The caller retains ownership of the inputs; we store our own pre-computed
// unit vectors so subsequent queries don't pay the trig cost.
func newUniformHash(lats, lons []float64) *uniformHash {
	n := len(lats)
	if n == 0 {
		return &uniformHash{
			nLat: 1, nLon: 1,
			invDLat: 1.0 / 180, invDLon: 1.0 / 360,
			offsets: make([]int32, 2),
		}
	}

	// nLon = 2·nLat keeps equatorial buckets square in degrees.
	// hashTargetCellsPerBucket × (nLat × nLon) ≈ N → nLat = √(N / 2K).
	nLat := int(math.Round(math.Sqrt(float64(n) / float64(2*hashTargetCellsPerBucket))))
	if nLat < 4 {
		nLat = 4
	}
	if nLat > 4096 {
		nLat = 4096
	}
	nLon := nLat * 2

	h := &uniformHash{
		nLat:    nLat,
		nLon:    nLon,
		invDLat: float64(nLat) / 180,
		invDLon: float64(nLon) / 360,
		offsets: make([]int32, nLat*nLon+1),
		cells:   make([]int32, n),
		vx:      make([]float64, n),
		vy:      make([]float64, n),
		vz:      make([]float64, n),
	}

	// Pass 1: compute unit vectors and per-cell bucket id, count per-bucket.
	bucket := make([]int32, n)
	for i := 0; i < n; i++ {
		x, y, z := latLonToVec3(lats[i], lons[i])
		h.vx[i] = x
		h.vy[i] = y
		h.vz[i] = z
		b := h.bucketOf(lats[i], lons[i])
		bucket[i] = int32(b)
		h.offsets[b+1]++
	}

	// Cumulative sum: offsets[b] becomes the start index of bucket b.
	for i := 1; i < len(h.offsets); i++ {
		h.offsets[i] += h.offsets[i-1]
	}

	// Pass 2: scatter cells. head[b] tracks the next free slot in bucket b.
	head := make([]int32, nLat*nLon)
	copy(head, h.offsets[:nLat*nLon])
	for i := 0; i < n; i++ {
		b := bucket[i]
		h.cells[head[b]] = int32(i)
		head[b]++
	}
	return h
}

// bucketOf maps (lat, lon) to a flat bucket index. lon is wrapped to
// [-180, 180); lat is clamped to [-90, 90].
func (h *uniformHash) bucketOf(lat, lon float64) int {
	li := int((90.0 - lat) * h.invDLat)
	if li < 0 {
		li = 0
	} else if li >= h.nLat {
		li = h.nLat - 1
	}
	lo := math.Mod(lon+180, 360)
	if lo < 0 {
		lo += 360
	}
	lj := int(lo * h.invDLon)
	if lj < 0 {
		lj = 0
	} else if lj >= h.nLon {
		lj = h.nLon - 1
	}
	return li*h.nLon + lj
}

// nearest returns the cell index nearest to (qlat, qlon) in 3-vector chord-
// squared distance, plus that distance. maxD2 > 0 caps the search radius:
// the search seeds bestD2 with maxD2 and aborts once the next ring's
// guaranteed-min chord exceeds it. -1 is returned when no cell is closer
// than maxD2.
func (h *uniformHash) nearest(qlat, qlon, maxD2 float64) (int, float64) {
	if len(h.cells) == 0 {
		return -1, 0
	}
	qx, qy, qz := latLonToVec3(qlat, qlon)
	return h.nearestVec(qlat, qlon, qx, qy, qz, maxD2)
}

// nearestVec is the inner kernel; callers that already have the query's
// 3-vector unit-sphere coordinates skip a redundant trig call.
func (h *uniformHash) nearestVec(qlat, qlon, qx, qy, qz, maxD2 float64) (int, float64) {
	bestD2 := math.Inf(1)
	if maxD2 > 0 {
		bestD2 = maxD2
	}
	bestIdx := int32(-1)

	li := int((90.0 - qlat) * h.invDLat)
	if li < 0 {
		li = 0
	} else if li >= h.nLat {
		li = h.nLat - 1
	}
	lo := math.Mod(qlon+180, 360)
	if lo < 0 {
		lo += 360
	}
	lj := int(lo * h.invDLon)
	if lj < 0 {
		lj = 0
	} else if lj >= h.nLon {
		lj = h.nLon - 1
	}

	// Ring termination uses the smaller of the two axis bounds:
	//   lat axis: ring · dLat (in degrees of arc on the sphere)
	//   lon axis: ring · dLon · |cos(qlat)| (lon-shift arc shrinks at high lat)
	// Both reduce to ring · dLat · min(1, |cos(qlat)|) since dLon = dLat.
	dLatDeg := 1.0 / h.invDLat
	cosLat := math.Abs(math.Cos(qlat * deg2rad))
	if cosLat > 1 {
		cosLat = 1
	}

	maxRing := h.nLat
	for ring := 0; ring <= maxRing; ring++ {
		h.scanRing(li, lj, ring, qx, qy, qz, &bestIdx, &bestD2)
		if bestIdx >= 0 && ring >= 1 {
			lbDeg := float64(ring) * dLatDeg * cosLat
			chord := 2 * math.Sin(lbDeg*deg2rad*0.5)
			if chord*chord >= bestD2 {
				break
			}
		}
	}
	return int(bestIdx), bestD2
}

// scanRing visits every bucket whose grid coordinates differ from (li, lj)
// by exactly `ring` (Chebyshev distance). ring=0 is the centre bucket only.
//
// For ti outside [0, nLat) the band is reflected across the pole and the
// lon coordinate shifted by nLon/2: a query at high latitude finds its
// cross-pole neighbours at small ring radii, instead of needing ring =
// nLon/2 to wrap the longitude axis.
func (h *uniformHash) scanRing(li, lj, ring int, qx, qy, qz float64, bestIdx *int32, bestD2 *float64) {
	if ring == 0 {
		h.scanBucket(li, lj, qx, qy, qz, bestIdx, bestD2)
		return
	}
	// Top + bottom edges (full lon span of the ring).
	for _, dli := range [2]int{-ring, ring} {
		ti, lonShift, ok := h.reflectLat(li + dli)
		if !ok {
			continue
		}
		for dlj := -ring; dlj <= ring; dlj++ {
			tj := wrapLon(lj+dlj+lonShift, h.nLon)
			h.scanBucket(ti, tj, qx, qy, qz, bestIdx, bestD2)
		}
	}
	// Left + right edges (corners are already covered by the top/bottom edges).
	for dli := -ring + 1; dli <= ring-1; dli++ {
		ti, lonShift, ok := h.reflectLat(li + dli)
		if !ok {
			continue
		}
		for _, dlj := range [2]int{-ring, ring} {
			tj := wrapLon(lj+dlj+lonShift, h.nLon)
			h.scanBucket(ti, tj, qx, qy, qz, bestIdx, bestD2)
		}
	}
}

// reflectLat folds an out-of-range lat-bucket index back into [0, nLat) by
// reflecting across the nearer pole, returning the lon shift that the
// reflection introduces (nLon/2 if reflected, 0 otherwise). Returns ok=false
// when the reflection itself lands outside the grid (only happens at search
// radii past nLat, far beyond any plausible nearest-neighbor).
func (h *uniformHash) reflectLat(ti int) (int, int, bool) {
	if ti >= 0 && ti < h.nLat {
		return ti, 0, true
	}
	if ti < 0 {
		r := -ti - 1
		if r >= h.nLat {
			return 0, 0, false
		}
		return r, h.nLon / 2, true
	}
	r := 2*h.nLat - 1 - ti
	if r < 0 {
		return 0, 0, false
	}
	return r, h.nLon / 2, true
}

// wrapLon normalises a possibly-negative or overflowing lon-bucket index
// into [0, n).
func wrapLon(j, n int) int {
	j %= n
	if j < 0 {
		j += n
	}
	return j
}

// scanBucket walks every cell in (li, lj), updating the running best.
func (h *uniformHash) scanBucket(li, lj int, qx, qy, qz float64, bestIdx *int32, bestD2 *float64) {
	b := li*h.nLon + lj
	start := int(h.offsets[b])
	end := int(h.offsets[b+1])
	for i := start; i < end; i++ {
		c := h.cells[i]
		dx := h.vx[c] - qx
		dy := h.vy[c] - qy
		dz := h.vz[c] - qz
		d2 := dx*dx + dy*dy + dz*dz
		if d2 < *bestD2 {
			*bestD2 = d2
			*bestIdx = c
		}
	}
}

// nearestRowGroup is the spatial-batch variant of nearestVec. It processes
// a contiguous run of pixels (a "group") that all share the same query
// bucket (li, lj) — the natural unit produced when a tile row is
// run-length-compressed by lon-bucket. Per-row constants (cosLat, dLatDeg)
// and the ring scan itself are amortised once across the whole group
// instead of once per pixel.
//
// The inner kernel pivots the loops: for each candidate cell we update
// every pixel's running best. The pixel arrays are stride-1 accesses,
// which the compiler vectorises cleanly on arm64; the candidate's vec3
// stays hot in registers across the pixel sweep.
//
// bestIdx and bestD2 are in-out; the caller seeds bestD2 with math.Inf(1)
// (or maxD2 for footprint-capped rendering) and bestIdx with -1.
func (h *uniformHash) nearestRowGroup(
	li, lj int,
	qlat float64,
	pixVx, pixVy, pixVz []float64,
	bestIdx []int32,
	bestD2 []float64,
) {
	if len(pixVx) == 0 {
		return
	}
	cosLat := math.Abs(math.Cos(qlat * deg2rad))
	if cosLat > 1 {
		cosLat = 1
	}
	dLatDeg := 1.0 / h.invDLat

	maxRing := h.nLat
	for ring := 0; ring <= maxRing; ring++ {
		h.scanRingGroup(li, lj, ring, pixVx, pixVy, pixVz, bestIdx, bestD2)
		if ring >= 1 {
			lbDeg := float64(ring) * dLatDeg * cosLat
			chord := 2 * math.Sin(lbDeg*deg2rad*0.5)
			chord2 := chord * chord
			// "This pixel may still improve" reduces to chord2 < bestD2[i]:
			// chord2 is the lower bound on any future ring's d2, so once
			// it exceeds bestD2 there is nothing closer to find.
			//
			// The previous version also kept the search alive whenever
			// bestIdx[i] < 0 ("we haven't found anything yet, don't give
			// up"), which was correct only in the unbounded-search case
			// where bestD2[i] stays math.Inf(1). For a footprint-capped
			// search (MaxChordSquared > 0, so bestD2[i] is seeded with
			// maxD2), an out-of-footprint pixel never decreases bestD2
			// below maxD2, so the bestIdx < 0 clause forced the ring
			// scan to expand to maxRing on every such pixel — a 256² tile
			// at iconch1's nLat≈250 burnt billions of bucket comparisons
			// per tile, hanging Switzerland-zoom renders for minutes.
			//
			// Dropping that clause is safe in both regimes:
			//   - Bounded (maxD2 > 0): an unmatched pixel's bestD2 stays
			//     at maxD2, and chord2 < maxD2 keeps the loop alive
			//     exactly until the ring's chord2 >= maxD2 — which is the
			//     real "no cell can be closer" condition.
			//   - Unbounded (maxD2 = 0): bestD2[i] is math.Inf(1) for
			//     unmatched pixels, so chord2 < +Inf is always true and
			//     the loop runs to maxRing as before. The eventual
			//     maxRing cap in this branch is the existing safety net
			//     and is unchanged by this fix.
			allDone := true
			for _, b := range bestD2 {
				if chord2 < b {
					allDone = false
					break
				}
			}
			if allDone {
				break
			}
		}
	}
}

func (h *uniformHash) scanRingGroup(li, lj, ring int, pixVx, pixVy, pixVz []float64, bestIdx []int32, bestD2 []float64) {
	if ring == 0 {
		h.scanBucketGroup(li, lj, pixVx, pixVy, pixVz, bestIdx, bestD2)
		return
	}
	for _, dli := range [2]int{-ring, ring} {
		ti, lonShift, ok := h.reflectLat(li + dli)
		if !ok {
			continue
		}
		for dlj := -ring; dlj <= ring; dlj++ {
			tj := wrapLon(lj+dlj+lonShift, h.nLon)
			h.scanBucketGroup(ti, tj, pixVx, pixVy, pixVz, bestIdx, bestD2)
		}
	}
	for dli := -ring + 1; dli <= ring-1; dli++ {
		ti, lonShift, ok := h.reflectLat(li + dli)
		if !ok {
			continue
		}
		for _, dlj := range [2]int{-ring, ring} {
			tj := wrapLon(lj+dlj+lonShift, h.nLon)
			h.scanBucketGroup(ti, tj, pixVx, pixVy, pixVz, bestIdx, bestD2)
		}
	}
}

func (h *uniformHash) scanBucketGroup(li, lj int, pixVx, pixVy, pixVz []float64, bestIdx []int32, bestD2 []float64) {
	b := li*h.nLon + lj
	start := int(h.offsets[b])
	end := int(h.offsets[b+1])
	for i := start; i < end; i++ {
		c := h.cells[i]
		cx := h.vx[c]
		cy := h.vy[c]
		cz := h.vz[c]
		for p := range pixVx {
			dx := cx - pixVx[p]
			dy := cy - pixVy[p]
			dz := cz - pixVz[p]
			d2 := dx*dx + dy*dy + dz*dz
			if d2 < bestD2[p] {
				bestD2[p] = d2
				bestIdx[p] = c
			}
		}
	}
}

// lonBucketOf returns the lon-bucket index for `lon` in degrees. Wraps lon
// into [-180, 180) and clamps the result to [0, nLon).
func (h *uniformHash) lonBucketOf(lon float64) int {
	lo := math.Mod(lon+180, 360)
	if lo < 0 {
		lo += 360
	}
	lj := int(lo * h.invDLon)
	if lj < 0 {
		lj = 0
	} else if lj >= h.nLon {
		lj = h.nLon - 1
	}
	return lj
}

// latBucketOf returns the lat-bucket index for `lat` in degrees. Clamps the
// result to [0, nLat).
func (h *uniformHash) latBucketOf(lat float64) int {
	li := int((90.0 - lat) * h.invDLat)
	if li < 0 {
		li = 0
	} else if li >= h.nLat {
		li = h.nLat - 1
	}
	return li
}
