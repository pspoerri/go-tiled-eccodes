package decode

import (
	"encoding/binary"
	"math"
)

// IEEE decodes Data Representation Template 5.4 — IEEE-754 floating point.
//
// Template 5.4 layout (template body):
//
//	byte 0   precision: 1 = 32-bit, 2 = 64-bit
//
// Section 7 contains nset values stored back-to-back as big-endian floats.
func IEEE(template, data []byte, nset int, dst []float64) ([]float64, error) {
	if len(template) < 1 {
		return nil, ErrBadComplexStream
	}
	if cap(dst) < nset {
		dst = make([]float64, nset)
	} else {
		dst = dst[:nset]
	}
	switch template[0] {
	case 1: // 32-bit
		if len(data) < nset*4 {
			return nil, ErrBadComplexStream
		}
		for i := 0; i < nset; i++ {
			bits := binary.BigEndian.Uint32(data[i*4:])
			dst[i] = float64(math.Float32frombits(bits))
		}
	case 2: // 64-bit
		if len(data) < nset*8 {
			return nil, ErrBadComplexStream
		}
		for i := 0; i < nset; i++ {
			bits := binary.BigEndian.Uint64(data[i*8:])
			dst[i] = math.Float64frombits(bits)
		}
	default:
		return nil, ErrBadComplexStream
	}
	return dst, nil
}
