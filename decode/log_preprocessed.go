package decode

import (
	"math"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// LogPreprocessed decodes Data Representation Template 5.61: simple-packed
// logarithms Z followed by the inverse preprocessing Y = exp(Z) - B.
func LogPreprocessed(template, data []byte, numPoints int, dst []float64) ([]float64, error) {
	if len(template) < 13 {
		return nil, ErrBadComplexStream
	}
	values, err := Simple(template[:10], data, numPoints, dst)
	if err != nil {
		return nil, err
	}
	b := float64(bswap.F32(template, 9))
	for i, z := range values {
		values[i] = math.Exp(z) - b
	}
	return values, nil
}
