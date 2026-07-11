package decode

import (
	"bytes"
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
//	byte  8     bits per value (0..32)
//	byte  9     type of original field values
//
// Section 7 contains a complete PNG image whose pixel values, decoded as
// unsigned integers, are the X values fed into Y = (R + X*2^E) / 10^D.
//
// ecCodes stores values in the smallest PNG container that can hold nbits:
// Gray8, Gray16, RGB8, or RGBA8. Native 1-, 2-, and 4-bit grayscale PNGs are
// also accepted; image/png expands those samples, so their IHDR depth is used
// to recover the original integer value.
func PNG(template, data []byte, nset int, dst []float64) ([]float64, error) {
	if len(template) < 10 || nset < 0 {
		return nil, ErrBadComplexStream
	}
	r := bswap.F32(template, 0)
	e := bswap.I16SM(template, 4)
	d := bswap.I16SM(template, 6)
	nbits := int(template[8])
	if nbits > 32 {
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

	// Constant fields have no coded values and therefore no PNG payload.
	if nbits == 0 {
		for i := range dst {
			dst[i] = bias
		}
		return dst, nil
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	pngDepth, colorType, ok := pngFormat(data)
	if !ok {
		return nil, ErrBadComplexStream
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if uint64(w)*uint64(h) != uint64(nset) {
		return nil, ErrBadComplexStream
	}

	switch {
	case nbits <= 8:
		if colorType != 0 || pngDepth > 8 || pngDepth < nbits {
			return nil, ErrBadComplexStream
		}
		shift := uint(8 - pngDepth)
		for j := 0; j < h; j++ {
			for i := 0; i < w; i++ {
				c := img.At(bounds.Min.X+i, bounds.Min.Y+j)
				p := color.GrayModel.Convert(c).(color.Gray).Y
				x := uint32(p) >> shift
				dst[j*w+i] = bias + float64(x)*mul
			}
		}
	case nbits <= 16:
		if colorType != 0 || pngDepth != 16 {
			return nil, ErrBadComplexStream
		}
		for j := 0; j < h; j++ {
			for i := 0; i < w; i++ {
				c := img.At(bounds.Min.X+i, bounds.Min.Y+j)
				p := color.Gray16Model.Convert(c).(color.Gray16).Y
				dst[j*w+i] = bias + float64(p)*mul
			}
		}
	case nbits <= 24:
		if pngDepth != 8 || (colorType != 2 && colorType != 6) {
			return nil, ErrBadComplexStream
		}
		for j := 0; j < h; j++ {
			for i := 0; i < w; i++ {
				c := img.At(bounds.Min.X+i, bounds.Min.Y+j)
				p := color.NRGBAModel.Convert(c).(color.NRGBA)
				x := uint32(p.R)<<16 | uint32(p.G)<<8 | uint32(p.B)
				dst[j*w+i] = bias + float64(x)*mul
			}
		}
	case nbits <= 32:
		if pngDepth != 8 || colorType != 6 {
			return nil, ErrBadComplexStream
		}
		for j := 0; j < h; j++ {
			for i := 0; i < w; i++ {
				c := img.At(bounds.Min.X+i, bounds.Min.Y+j)
				p := color.NRGBAModel.Convert(c).(color.NRGBA)
				x := uint32(p.R)<<24 | uint32(p.G)<<16 | uint32(p.B)<<8 | uint32(p.A)
				dst[j*w+i] = bias + float64(x)*mul
			}
		}
	}
	return dst, nil
}

func pngFormat(data []byte) (bitDepth, colorType int, ok bool) {
	if len(data) < 26 ||
		!bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")) ||
		!bytes.Equal(data[12:16], []byte("IHDR")) {
		return 0, 0, false
	}
	return int(data[24]), int(data[25]), true
}
