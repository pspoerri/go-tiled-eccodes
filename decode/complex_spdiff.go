package decode

import (
	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// ComplexSpatialDifferencing decodes Data Representation Template 5.3 —
// complex packing with spatial differencing.
//
// Template 5.3 extends 5.2 with two extra octets at the end of the template:
//
//	byte 36  order of spatial differencing (1 or 2)
//	byte 37  number of octets required for extra descriptors (typically 2)
//
// Section 7 layout for 5.3:
//
//	octets 1..ed*order   the first `order` original values, sign-magnitude
//	octets next ed       overall minimum (sign-magnitude)
//	then the standard 5.2 stream (refs / widths / lengths / values), but the
//	    encoded values are *differences* and the group lengths sum to
//	    (n - order) rather than n.
//
// Reconstruction:
//
//	order==1:  Y[0]   = V[0]
//	           Y[k>=1] = Y[k-1] + (X[k-1] + overall_min)
//	order==2:  Y[0]   = V[0]
//	           Y[1]   = V[1]
//	           Y[k>=2] = 2*Y[k-1] - Y[k-2] + (X[k-2] + overall_min)
//
// Final physical value uses the same Y = (R + Y * 2^E) / 10^D formula as 5.2.
func ComplexSpatialDifferencing(template, data []byte, nset int, dst []float64) ([]float64, error) {
	h := parseComplexHeader(template)
	order := int(template[36])
	ed := int(template[37])
	if order != 1 && order != 2 {
		return nil, ErrBadComplexStream
	}
	if ed < 1 || ed > 4 {
		return nil, ErrBadComplexStream
	}

	// Read the leading initial-values + overall-min, all as ed-octet
	// sign-magnitude ints, then advance into the 5.2 stream.
	var initial [2]int64
	for i := 0; i < order; i++ {
		initial[i] = readSM(data[i*ed : (i+1)*ed])
	}
	overallMin := readSM(data[order*ed : (order+1)*ed])
	stream := data[(order+1)*ed:]

	// Per eccodes/grib_accessor_class_data_g22order_packing.c, the inner
	// 5.2 stream encodes N (not N-order) values: the first `order` slots are
	// placeholders that are subsequently overridden by the initial values,
	// and the remaining slots hold the (overall_min-shifted) differences.
	work := i64WorkPool.get(nset)
	defer i64WorkPool.put(work)
	diffs, err := decodeComplexInto(h, stream, nset, work)
	if err != nil {
		return nil, err
	}

	full := diffs // reuse the same []int64 buffer in-place
	for i := 0; i < order; i++ {
		full[i] = initial[i]
	}
	if order == 1 {
		for k := 1; k < nset; k++ {
			d := diffs[k]
			if d == missingMarker || d == missingMarker2 {
				full[k] = d
				continue
			}
			full[k] = full[k-1] + d + overallMin
		}
	} else {
		for k := 2; k < nset; k++ {
			d := diffs[k]
			if d == missingMarker || d == missingMarker2 {
				full[k] = d
				continue
			}
			full[k] = 2*full[k-1] - full[k-2] + d + overallMin
		}
	}

	return finalizeValues(h, full, dst, 0, 0), nil
}

// readSM reads a sign-magnitude integer from a 1-, 2-, 3-, or 4-byte field.
func readSM(b []byte) int64 {
	switch len(b) {
	case 1:
		v := int64(b[0])
		if v&0x80 != 0 {
			return -(v & 0x7f)
		}
		return v
	case 2:
		v := int64(bswap.U16(b, 0))
		if v&0x8000 != 0 {
			return -(v & 0x7fff)
		}
		return v
	case 3:
		v := int64(b[0])<<16 | int64(b[1])<<8 | int64(b[2])
		if v&0x800000 != 0 {
			return -(v & 0x7fffff)
		}
		return v
	case 4:
		v := int64(bswap.U32(b, 0))
		if v&0x80000000 != 0 {
			return -(v & 0x7fffffff)
		}
		return v
	}
	return 0
}
