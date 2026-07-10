package decode

import (
	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// SpectralSimple decodes Data Representation Template 5.50. The real (0,0)
// coefficient is stored separately in the template; the remaining coefficients
// use the same scaling and bit packing as template 5.0.
func SpectralSimple(template, data []byte, numPoints int, dst []float64) ([]float64, error) {
	if len(template) < 13 || numPoints < 1 {
		return nil, ErrBadComplexStream
	}
	rest, err := Simple(template[:10], data, numPoints-1, nil)
	if err != nil {
		return nil, err
	}
	if cap(dst) < numPoints {
		dst = make([]float64, numPoints)
	} else {
		dst = dst[:numPoints]
	}
	dst[0] = float64(bswap.F32(template, 9))
	copy(dst[1:], rest)
	return dst, nil
}
