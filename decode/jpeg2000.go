package decode

import (
	"fmt"
	"math"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// JPEG2000 decodes Data Representation Template 5.40 (JPEG-2000 codestream).
//
// Template 5.40 layout (template bytes — i.e. starting at section 5 byte 11):
//
//	bytes 0-3   reference value R (IEEE-754 float32 BE)
//	bytes 4-5   binary scale factor E (sign-magnitude int16)
//	bytes 6-7   decimal scale factor D (sign-magnitude int16)
//	byte  8     number of bits per packed value (informational; the
//	            codestream's SIZ marker is authoritative)
//	byte  9     type of original field values (Code Table 5.1)
//	byte  10    type of compression (0 = lossless, 1 = lossy)
//
// Decoded value: Y = (R + X * 2^E) / 10^D
//
// The Section 7 payload is a raw JPEG-2000 codestream (no JP2 container).
// This function delegates to the system libopenjp2 via the cgo-tagged
// jpeg2000Decode helper. On builds without CGo, it returns ErrCgoRequired.
func JPEG2000(template, data []byte, numPoints int, dst []float64) ([]float64, error) {
	if len(template) < 10 {
		return nil, fmt.Errorf("decode: template 5.40 too short (%d bytes)", len(template))
	}
	r := bswap.F32(template, 0)
	e := bswap.I16SM(template, 4)
	d := bswap.I16SM(template, 6)
	nbits := int(template[8])

	if cap(dst) < numPoints {
		dst = make([]float64, numPoints)
	} else {
		dst = dst[:numPoints]
	}

	scaleBin := math.Ldexp(1, int(e))
	scaleDec := math.Pow10(-int(d))
	bias := float64(r) * scaleDec

	if nbits == 0 || numPoints == 0 {
		// nbits=0 conventionally means "every value equals R/10^D"; some
		// encoders emit an empty payload in this case rather than a real
		// codestream. Short-circuit before we hand bytes to libopenjp2.
		v := bias
		for i := range dst {
			dst[i] = v
		}
		return dst, nil
	}

	samples, err := jpeg2000Decode(data, numPoints)
	if err != nil {
		return nil, err
	}
	if len(samples) < numPoints {
		return nil, fmt.Errorf("decode: J2K produced %d samples, expected %d",
			len(samples), numPoints)
	}
	mul := scaleBin * scaleDec
	for i := 0; i < numPoints; i++ {
		dst[i] = bias + float64(samples[i])*mul
	}
	return dst, nil
}
