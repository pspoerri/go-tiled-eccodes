package grid

import (
	"math"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// Gaussian is Grid Definition Template 3.40 — Gaussian latitude/longitude.
// Handles both the regular (Ni present, every row has Ni points) and reduced
// (Ni == 0xffffffff, per-row pl[] table appended) variants.
//
// The latitudes are not equispaced — they are roots of the Legendre
// polynomial P_2N (where N is the number of latitudes between the equator
// and pole). Total number of latitudes = 2N. We compute them by Newton
// iteration on the Legendre polynomial, then sort to descending order.
type Gaussian struct {
	N              int       // number of latitudes between equator and pole
	Lats           []float64 // 2N Gaussian latitudes (descending: north → south)
	Earth          Earth
	Reduced        bool
	Ni             int   // for regular grids, the row width
	PL             []int // for reduced grids, points per row (len = Nj = 2N)
	rowOffsets     []int // prefix sum of PL (len Nj+1)
	storageOffsets []int // prefix sum in encoded scan order
	totalPoints    int
	La1, La2       float64
	Lo1, Lo2, Di   float64
	IPositive      bool
	JPositive      bool
	Consecutive    bool
	Alternate      bool

	// invLatBucket and latToRow form a uniform lat-bucket → row-index hash
	// that replaces sort.Search in Locate. The bucket grid has nb = 4·Nj
	// uniform-degree buckets (nb+1 entries: latToRow[b] is the first row
	// index whose Gaussian latitude sits at or below the upper edge of
	// bucket b, latToRow[nb] = Nj). For a query, latToRow[b]..latToRow[b+1]
	// brackets the answer in O(1) array reads followed by ≤1 refinement
	// step — the closure-hosted binary search of sort.Search was cutting
	// 30% off the cold tile path on regular Gaussian grids.
	invLatBucket float64
	latToRow     []int32
}

// ParseGaussian decodes Section 3 template 40, optionally followed by a
// list-of-numbers (the pl[] table) for the reduced variant.
func ParseGaussian(s3template, optionalList []byte, listOctets int) Gaussian {
	g := Gaussian{}
	scale := angleScale(s3template)
	g.Earth = ParseEarth(s3template)
	rawNi := bswap.U32(s3template, 16)
	nj := int(bswap.U32(s3template, 20))

	g.La1 = float64(bswap.I32SM(s3template, 32)) / scale
	g.Lo1 = float64(bswap.I32SM(s3template, 36)) / scale
	g.La2 = float64(bswap.I32SM(s3template, 41)) / scale
	g.Lo2 = float64(bswap.I32SM(s3template, 45)) / scale
	if rawDi := bswap.U32(s3template, 49); rawDi != 0xffffffff {
		g.Di = float64(rawDi) / scale
	}
	// N (number of parallels between equator and pole) sits at template byte
	// 53 — same slot as Dj in template 3.0, since Gaussian latitudes are
	// non-uniform and Dj is unused.
	g.N = int(bswap.U32(s3template, 53))
	if g.N == 0 {
		g.N = nj / 2
	}

	scan := s3template[57]
	g.IPositive = scan&0x80 == 0
	g.JPositive = scan&0x40 != 0
	g.Consecutive = scan&0x20 == 0
	g.Alternate = scan&0x10 != 0

	fullLatitudes := gaussianLatitudes(g.N)
	g.Lats = gaussianSubset(fullLatitudes, nj, g.La1, g.La2)

	var naturalWidths, storageWidths []int
	if rawNi == 0xffffffff {
		g.Reduced = true
		storageWidths = parsePL(optionalList, nj, listOctets)
		if g.JPositive {
			g.PL = reversedInts(storageWidths)
		} else {
			g.PL = append([]int(nil), storageWidths...)
		}
		naturalWidths = g.PL
	} else {
		g.Ni = int(rawNi)
		naturalWidths = make([]int, nj)
		storageWidths = make([]int, nj)
		for j := 0; j < nj; j++ {
			naturalWidths[j] = g.Ni
			storageWidths[j] = g.Ni
		}
	}

	g.rowOffsets = prefixSums(naturalWidths)
	g.storageOffsets = prefixSums(storageWidths)
	g.totalPoints = g.storageOffsets[len(g.storageOffsets)-1]
	g.buildLatRowHash()
	return g
}

// buildLatRowHash populates the uniform lat-bucket → row-index lookup
// table used by Locate. Costs O(Nj) — Lats is descending so we make a
// single forward pass, advancing the row pointer as bucket edges sweep
// southward.
func (g *Gaussian) buildLatRowHash() {
	nj := len(g.Lats)
	if nj == 0 {
		return
	}
	nb := 4 * nj
	if nb < 16 {
		nb = 16
	}
	g.invLatBucket = float64(nb) / 180
	g.latToRow = make([]int32, nb+1)
	dLat := 180.0 / float64(nb)
	j := 0
	for b := 0; b <= nb; b++ {
		// Upper edge of bucket b in geographic lat (descending sweep).
		edge := 90 - float64(b)*dLat
		for j < nj && g.Lats[j] > edge {
			j++
		}
		g.latToRow[b] = int32(j)
	}
}

// locateRow returns the index of the first row whose Gaussian latitude is
// at or below `lat` — same contract as the previous sort.Search call.
// Falls back to a linear scan for grids constructed without ParseGaussian
// (test fixtures that poke private fields directly).
func (g Gaussian) locateRow(lat float64) int {
	nj := len(g.Lats)
	if g.latToRow == nil {
		j := 0
		for j < nj && g.Lats[j] > lat {
			j++
		}
		return j
	}
	nb := len(g.latToRow) - 1
	b := int((90.0 - lat) * g.invLatBucket)
	if b < 0 {
		b = 0
	} else if b >= nb {
		b = nb - 1
	}
	jStart := int(g.latToRow[b])
	jEnd := int(g.latToRow[b+1])
	if jEnd > nj {
		jEnd = nj
	}
	for j := jStart; j < jEnd; j++ {
		if g.Lats[j] <= lat {
			return j
		}
	}
	return jEnd
}

func parsePL(b []byte, nj int, listOctets int) []int {
	out := make([]int, nj)
	if listOctets <= 0 || len(b) < nj*listOctets {
		return out
	}
	for j := 0; j < nj; j++ {
		off := j * listOctets
		switch listOctets {
		case 1:
			out[j] = int(b[off])
		case 2:
			out[j] = int(bswap.U16(b, off))
		case 3:
			out[j] = int(b[off])<<16 | int(b[off+1])<<8 | int(b[off+2])
		case 4:
			out[j] = int(bswap.U32(b, off))
		}
	}
	return out
}

func (g Gaussian) Size() (int, int) {
	if g.Reduced {
		// For reduced grids, "Ni" is the maximum row width. Callers should
		// route through Index(i,j) and not assume a rectangular layout.
		max := 0
		for _, v := range g.PL {
			if v > max {
				max = v
			}
		}
		return max, len(g.Lats)
	}
	return g.Ni, len(g.Lats)
}

func (g Gaussian) NumPoints() int { return g.totalPoints }

// IsNatural is true when the grid's storage is already in the natural (W→E,
// N→S) row-major layout — so the renderer can skip its scanning-mode
// reorder pass.
func (g Gaussian) IsNatural() bool {
	return g.IPositive && !g.JPositive && g.Consecutive && !g.Alternate && !g.Reduced
}

func (g Gaussian) Index(i, j int) int {
	nj := len(g.Lats)
	if j < 0 || j >= nj || len(g.rowOffsets) != nj+1 || len(g.storageOffsets) != nj+1 {
		return -1
	}
	rowW := g.rowOffsets[j+1] - g.rowOffsets[j]
	if i < 0 || i >= rowW {
		return -1
	}
	if g.Reduced && !g.Consecutive {
		return -1
	}

	si, sj := i, j
	if !g.IPositive {
		si = rowW - 1 - i
	}
	if g.JPositive {
		sj = nj - 1 - j
	}
	if !g.Consecutive {
		if g.Alternate && (si&1) == 1 {
			sj = nj - 1 - sj
		}
		return si*nj + sj
	}
	storageW := g.storageOffsets[sj+1] - g.storageOffsets[sj]
	if g.Alternate && (sj&1) == 1 {
		si = storageW - 1 - si
	}
	if si < 0 || si >= storageW {
		return -1
	}
	return g.storageOffsets[sj] + si
}

// Locate finds the source pixel for a geographic coordinate. For Gaussian
// grids, latitude is non-uniform; locateRow does an O(1) lookup into a
// uniform lat-bucket → row-index table and refines within the bucket's
// row span.
func (g Gaussian) Locate(lat, lon float64) (float64, float64, bool) {
	nj := len(g.Lats)
	if nj == 0 {
		return 0, 0, false
	}
	if lat > g.Lats[0]+1e-9 || lat < g.Lats[nj-1]-1e-9 {
		return 0, 0, false
	}

	jUpper := g.locateRow(lat)
	var fj float64
	switch {
	case jUpper == 0:
		fj = 0
	case jUpper >= nj:
		fj = float64(nj - 1)
	default:
		l1 := g.Lats[jUpper-1]
		l2 := g.Lats[jUpper]
		t := (l1 - lat) / (l1 - l2)
		fj = float64(jUpper-1) + t
	}

	jRound := int(math.Floor(fj + 0.5))
	if jRound < 0 {
		jRound = 0
	}
	if jRound >= nj {
		jRound = nj - 1
	}
	if len(g.rowOffsets) != nj+1 {
		return 0, 0, false
	}
	rowW := g.rowOffsets[jRound+1] - g.rowOffsets[jRound]
	west, east, di, global := g.longitudeLayout(rowW)
	if rowW == 1 {
		return 0, fj, math.Abs(wrap180(lon-west)) < 1e-9
	}
	if di <= 0 {
		return 0, 0, false
	}
	lonN := wrap360(lon, west)
	if !global && lonN > east+1e-9 {
		return 0, 0, false
	}
	fi := (lonN - west) / di
	if global {
		if fi < 0 {
			fi += float64(rowW)
		}
		if fi >= float64(rowW) {
			fi -= float64(rowW)
		}
	} else if fi < -1e-9 || fi > float64(rowW-1)+1e-9 {
		return 0, 0, false
	}
	return fi, fj, true
}

// gaussianLatitudes returns 2N Gaussian latitudes in *descending* order
// (north pole to south pole). Computed via Newton iteration on the Legendre
// polynomial P_{2N}(sin φ) = 0. Cost: O(N²) but tiny — N≤320 in practice.
//
// Reference: Press et al., "Numerical Recipes", §4.5 (Gauss-Legendre).
func gaussianLatitudes(N int) []float64 {
	if N <= 0 {
		return nil
	}
	deg := 180.0 / math.Pi
	M := 2 * N
	out := make([]float64, M)
	// Initial guess: cos(π * (i + 0.75) / (M + 0.5)) for i in 0..M-1
	for i := 0; i < (M+1)/2; i++ {
		z := math.Cos(math.Pi * (float64(i) + 0.75) / (float64(M) + 0.5))
		// Newton iteration on Legendre P_M.
		for iter := 0; iter < 64; iter++ {
			p1, p2 := 1.0, 0.0
			for j := 1; j <= M; j++ {
				p3 := p2
				p2 = p1
				p1 = ((2*float64(j)-1)*z*p2 - (float64(j)-1)*p3) / float64(j)
			}
			pp := float64(M) * (z*p1 - p2) / (z*z - 1)
			z1 := z
			z = z1 - p1/pp
			if math.Abs(z-z1) < 1e-15 {
				break
			}
		}
		out[i] = math.Asin(z) * deg
		out[M-1-i] = -out[i]
	}
	// Above we computed in descending order from the highest |z| (closest to
	// pole) — that is already northern latitudes first. Sanity-check.
	if out[0] < out[1] {
		// Reverse if necessary (defensive — shouldn't happen with our init).
		for i, j := 0, M-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out
}
