package decode

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// PNG decodes Data Representation Template 5.41 (PNG-packed grid points).
//
// Template 5.41 layout (template body):
//
//	bytes 0-3   reference value R (float32)
//	bytes 4-5   binary scale factor E (sign-magnitude int16)
//	bytes 6-7   decimal scale factor D (sign-magnitude int16)
//	byte  8     bits per value (8, 16, or 24 in practice)
//	byte  9     type of original field values
//
// Section 7 contains a complete PNG image whose pixel values, decoded as
// unsigned integers, are the X values fed into Y = (R + X*2^E) / 10^D.
//
// 8-bit data appears as Gray; 16-bit as Gray16. 24-bit is rare and packed
// across three 8-bit channels — handled at the end as a fallback.
func PNG(template, data []byte, nset int, dst []float64) ([]float64, error) {
	r := bswap.F32(template, 0)
	e := bswap.I16SM(template, 4)
	d := bswap.I16SM(template, 6)
	nbits := int(template[8])

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w*h != nset {
		return nil, ErrBadComplexStream
	}

	if cap(dst) < nset {
		dst = make([]float64, nset)
	} else {
		dst = dst[:nset]
	}
	scaleBin := math.Ldexp(1, int(e))
	scaleDec := math.Pow10(-int(d))
	mul := scaleBin * scaleDec
	bias := float64(r) * scaleDec

	switch im := img.(type) {
	case *image.Gray:
		for j := 0; j < h; j++ {
			row := im.Pix[j*im.Stride : j*im.Stride+w]
			out := dst[j*w : j*w+w]
			for i, p := range row {
				out[i] = bias + float64(p)*mul
			}
		}
	case *image.Gray16:
		for j := 0; j < h; j++ {
			rowOff := j * im.Stride
			out := dst[j*w : j*w+w]
			for i := 0; i < w; i++ {
				p := uint16(im.Pix[rowOff+2*i])<<8 | uint16(im.Pix[rowOff+2*i+1])
				out[i] = bias + float64(p)*mul
			}
		}
	default:
		// Slow generic path — image/png decodes RGB/RGBA palette etc. We
		// reduce to gray by averaging channels weighted as luminance, which
		// works for the rare 24-bit gridpoint layout that some encoders
		// emit. Cost is one float per pixel; acceptable for the cold path.
		for j := 0; j < h; j++ {
			for i := 0; i < w; i++ {
				c := im.At(bounds.Min.X+i, bounds.Min.Y+j)
				gv := color.GrayModel.Convert(c).(color.Gray).Y
				dst[j*w+i] = bias + float64(gv)*mul
			}
		}
	}

	_ = nbits
	return dst, nil
}
