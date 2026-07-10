package decode

import (
	"math"
)

// complexHeader holds the fields shared by templates 5.2 and 5.3.
type complexHeader struct {
	R        float32
	E        int16
	D        int16
	NBits    int // bits per group reference (R_bits)
	OrigType uint8
	Split    uint8
	MVM      uint8 // missing value management: 0/1/2
	MissingP uint32
	MissingS uint32
	NG       int
	RefW     uint8 // reference for group widths
	WBits    int   // bits per group width
	RefL     uint32
	LIncr    uint8  // length increment
	LastL    uint32 // true length of last group
	LBits    int    // bits per scaled group length
}

// Complex decodes Data Representation Template 5.2 (complex packing). nset
// is the number of values to decode (i.e. number of valid points after a
// bitmap, or the full grid if no bitmap is present).
func Complex(template, data []byte, nset int, dst []float64) ([]float64, error) {
	h, err := parseComplexHeader(template)
	if err != nil || nset < 0 {
		return nil, ErrBadComplexStream
	}
	work := i64WorkPool.get(nset)
	defer i64WorkPool.put(work)
	values, err := decodeComplexInto(h, data, nset, work)
	if err != nil {
		return nil, err
	}
	return finalizeValues(h, values, dst, 0, 0), nil
}

// decodeComplexInto runs the four-stream complex unpacker and returns the raw
// (X1[g] + X2[k]) integers as int64s. The returned slice is owned by the
// caller and may alias dst.
func decodeComplexInto(h complexHeader, data []byte, nValues int, dst []int64) ([]int64, error) {
	meta, err := parseComplexMeta(h, data, nValues)
	if err != nil {
		return nil, err
	}
	if cap(dst) < nValues {
		dst = make([]int64, nValues)
	} else {
		dst = dst[:nValues]
	}

	bs := newBitReader(meta.values)
	idx := 0
	for group := 0; group < h.NG; group++ {
		width := meta.widths[group]
		length := meta.lengths[group]
		ref := int64(meta.refs[group])
		groupMissing := h.MVM > 0 && meta.refs[group] == allOnes(h.NBits)

		if width == 0 {
			value := ref
			if groupMissing {
				value = missingMarker
			}
			for k := 0; k < length; k++ {
				dst[idx+k] = value
			}
			idx += length
			continue
		}

		missingPrimary := allOnes(width)
		missingSecondary := missingPrimary - 1
		for k := 0; k < length; k++ {
			x := bs.read(width)
			switch {
			case h.MVM >= 1 && x == missingPrimary:
				dst[idx+k] = missingMarker
			case h.MVM == 2 && x == missingSecondary:
				dst[idx+k] = missingMarker2
			default:
				dst[idx+k] = ref + int64(x)
			}
		}
		idx += length
	}
	return dst, nil
}

// missingMarker is an int64 sentinel placed in the integer working buffer to
// flag "missing primary"; finalizeValues turns it into NaN. We use values
// that can't appear via legal complex-packed integers.
const (
	missingMarker  = int64(-(1 << 60))
	missingMarker2 = int64(-(1 << 60)) + 1
)

// finalizeValues applies Y = (R + (X + minOff) * 2^E) / 10^D to every value,
// translating missing markers into NaN. minOff is 0 for 5.2 and the spatial
// differencing overall-minimum for 5.3.
func finalizeValues(h complexHeader, x []int64, dst []float64, minOff int64, _ int) []float64 {
	n := len(x)
	if cap(dst) < n {
		dst = make([]float64, n)
	} else {
		dst = dst[:n]
	}
	scaleBin := math.Ldexp(1, int(h.E))
	scaleDec := math.Pow10(-int(h.D))
	mul := scaleBin * scaleDec
	bias := float64(h.R) * scaleDec
	for i, v := range x {
		if v == missingMarker || v == missingMarker2 {
			dst[i] = math.NaN()
			continue
		}
		dst[i] = bias + float64(v+minOff)*mul
	}
	return dst
}

// bitsToBytes returns the number of whole bytes consumed by `bits` bits when
// padded to a byte boundary at the end of the stream. Streams within a
// Section 7 payload are byte-aligned per WMO convention — group references,
// widths, lengths, and values each start on a fresh byte.
func bitsToBytes(bits int) int { return (bits + 7) >> 3 }

// bitReader is a continuous big-endian MSB-first bit reader used for the
// "group values" stream (the only stream where widths vary per call).
type bitReader struct {
	src  []byte
	acc  uint64
	bits int
	pos  int
}

func newBitReader(src []byte) *bitReader { return &bitReader{src: src} }

// read pulls n bits (n ≤ 32) MSB-first from the underlying byte stream.
// Refill strategy: prefer a single 32-bit big-endian load when at least 4
// bytes remain and the accumulator has room — that's roughly 4× the work
// per cache line vs the 1-byte path and is the hot loop for complex
// packings (template 5.2 / 5.3).
func (b *bitReader) read(n int) uint32 {
	// Fast 32-bit refill while we still have headroom in the 64-bit accumulator.
	for b.bits < n && b.bits <= 32 && b.pos+4 <= len(b.src) {
		s := b.src[b.pos:]
		w := uint32(s[0])<<24 | uint32(s[1])<<16 | uint32(s[2])<<8 | uint32(s[3])
		b.acc = (b.acc << 32) | uint64(w)
		b.pos += 4
		b.bits += 32
	}
	// Tail: byte-at-a-time for the last <4 bytes of the stream.
	for b.bits < n && b.pos < len(b.src) {
		b.acc = (b.acc << 8) | uint64(b.src[b.pos])
		b.pos++
		b.bits += 8
	}
	if n == 0 {
		return 0
	}
	shift := uint(b.bits - n)
	mask := uint64(1)<<uint(n) - 1
	v := uint32((b.acc >> shift) & mask)
	b.bits -= n
	if b.bits > 0 {
		b.acc &= uint64(1)<<uint(b.bits) - 1
	} else {
		b.acc = 0
	}
	return v
}
