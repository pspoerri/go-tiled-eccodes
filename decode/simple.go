// Package decode contains GRIB2 Section 5 / Section 7 payload decoders. Each
// function takes the section bytes (as views into the mmap) plus an optional
// destination buffer and returns physical values as float64.
package decode

import (
	"math"

	"github.com/pspoerri/go-tiled-eccodes/internal/bitstream"
	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// Simple decodes Data Representation Template 5.0 (simple packing).
//
// Template 5.0 layout (template bytes — i.e. starting at section 5 byte 11):
//
//	bytes 0-3   reference value R (IEEE-754 float32, big-endian)
//	bytes 4-5   binary scale factor E (sign-magnitude int16)
//	bytes 6-7   decimal scale factor D (sign-magnitude int16)
//	byte  8     number of bits per value
//	byte  9     type of original field values (Code Table 5.1)
//
// Decoded value: Y = (R + X * 2^E) / 10^D
//
// numPoints is the number of *non-missing* points to unpack from data; when
// a bitmap is present, callers must pass the count of set bits and use
// ApplyBitmap to fan the values out.
func Simple(template, data []byte, numPoints int, dst []float64) ([]float64, error) {
	if len(template) < 10 || numPoints < 0 {
		return nil, ErrBadComplexStream
	}
	r := bswap.F32(template, 0)
	e := bswap.I16SM(template, 4)
	d := bswap.I16SM(template, 6)
	nbits := int(template[8])
	if nbits > 32 ||
		uint64(numPoints)*uint64(nbits) > uint64(len(data))*8 {
		return nil, ErrBadComplexStream
	}

	scaleBin := math.Ldexp(1, int(e)) // 2^e, exact for representable e
	scaleDec := math.Pow10(-int(d))   // 10^-d
	bias := float64(r) * scaleDec

	if cap(dst) < numPoints {
		dst = make([]float64, numPoints)
	} else {
		dst = dst[:numPoints]
	}

	if nbits == 0 {
		// Every value equals the reference (or rather R/10^D after applying D).
		v := float64(r) * scaleDec
		for i := range dst {
			dst[i] = v
		}
		return dst, nil
	}

	// Unpack ints, then apply Y = (R + X*2^E)/10^D in one pass.
	tmp := unpackPool.get(numPoints)
	defer unpackPool.put(tmp)
	tmp = bitstream.Unpack(data, nbits, numPoints, tmp)

	mul := scaleBin * scaleDec
	for i, x := range tmp {
		dst[i] = bias + float64(x)*mul
	}
	return dst, nil
}

// ApplyBitmap maps a packed slice of `nset` decoded non-missing values into a
// `nTotal`-sized output, marking points whose bitmap bit is 0 as NaN. bitmap
// is the raw section-6 bitmap (MSB first), bit i of byte b → point index
// b*8 + (7-i). The output is grown if needed.
//
// When bitmap is nil and nTotal == nset, this is a no-op alias (returns vals
// unchanged or copied into dst).
func ApplyBitmap(bitmap []byte, vals []float64, nTotal int, dst []float64) []float64 {
	if cap(dst) < nTotal {
		dst = make([]float64, nTotal)
	} else {
		dst = dst[:nTotal]
	}
	if bitmap == nil {
		copy(dst, vals)
		return dst
	}
	vi := 0
	for i := 0; i < nTotal; i++ {
		bit := (bitmap[i>>3] >> uint(7-(i&7))) & 1
		if bit == 1 {
			dst[i] = vals[vi]
			vi++
		} else {
			dst[i] = math.NaN()
		}
	}
	return dst
}
