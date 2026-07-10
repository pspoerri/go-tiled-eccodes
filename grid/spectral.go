package grid

import "github.com/pspoerri/go-tiled-eccodes/internal/bswap"

// Spectral describes Grid Definition Template 3.50 (spherical harmonic
// coefficients). It permits coefficient decoding, but Locate always returns
// false because synthesis onto a geographic grid is outside the Grid contract.
type Spectral struct {
	J, K, M      uint32
	SpectralType uint8
	SpectralMode uint8
	NumValues    int
}

func ParseSpectral(template []byte, numValues int) Spectral {
	return Spectral{
		J:            bswap.U32(template, 0),
		K:            bswap.U32(template, 4),
		M:            bswap.U32(template, 8),
		SpectralType: template[12],
		SpectralMode: template[13],
		NumValues:    numValues,
	}
}

func (g Spectral) Size() (int, int) { return g.NumValues, 1 }
func (g Spectral) NumPoints() int   { return g.NumValues }

func (g Spectral) Index(i, j int) int {
	if j != 0 || i < 0 || i >= g.NumValues {
		return -1
	}
	return i
}

func (g Spectral) Locate(_, _ float64) (float64, float64, bool) {
	return 0, 0, false
}
