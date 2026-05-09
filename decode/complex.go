package decode

import (
	"math"

	"github.com/pspoerri/go-tiled-eccodes/internal/bitstream"
	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
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

func parseComplexHeader(t []byte) complexHeader {
	return complexHeader{
		R:        bswap.F32(t, 0),
		E:        bswap.I16SM(t, 4),
		D:        bswap.I16SM(t, 6),
		NBits:    int(t[8]),
		OrigType: t[9],
		Split:    t[10],
		MVM:      t[11],
		MissingP: bswap.U32(t, 12),
		MissingS: bswap.U32(t, 16),
		NG:       int(bswap.U32(t, 20)),
		RefW:     t[24],
		WBits:    int(t[25]),
		RefL:     bswap.U32(t, 26),
		LIncr:    t[30],
		LastL:    bswap.U32(t, 31),
		LBits:    int(t[35]),
	}
}

// Complex decodes Data Representation Template 5.2 (complex packing). nset
// is the number of values to decode (i.e. number of valid points after a
// bitmap, or the full grid if no bitmap is present).
func Complex(template, data []byte, nset int, dst []float64) ([]float64, error) {
	h := parseComplexHeader(template)
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
	if h.NG <= 0 {
		return nil, ErrBadComplexStream
	}

	// 1. Group references X1[g], each h.NBits wide.
	refs := bitstream.Unpack(data, h.NBits, h.NG, nil)
	refsBytes := bitsToBytes(h.NBits * h.NG)

	// 2. Group widths W[g], each h.WBits wide. Actual width = h.RefW + W[g].
	widths := bitstream.Unpack(data[refsBytes:], h.WBits, h.NG, nil)
	widthsBytes := bitsToBytes(h.WBits * h.NG)

	// 3. Group lengths L[g], each h.LBits wide. Actual length =
	//    h.RefL + L[g]*h.LIncr; except last group uses h.LastL.
	lens := bitstream.Unpack(data[refsBytes+widthsBytes:], h.LBits, h.NG, nil)
	lensBytes := bitsToBytes(h.LBits * h.NG)

	// Cumulative sanity check.
	totalLen := 0
	for g := 0; g < h.NG; g++ {
		actualLen := int(h.RefL) + int(lens[g])*int(h.LIncr)
		if g == h.NG-1 {
			actualLen = int(h.LastL)
		}
		totalLen += actualLen
	}
	if totalLen != nValues {
		return nil, ErrBadComplexStream
	}

	// 4. Group values — decode each group inline. We build a single contiguous
	//    int64 buffer of nValues entries; group g contributes L[g] values
	//    each at width W[g] starting at the right bit offset.
	if cap(dst) < nValues {
		dst = make([]int64, nValues)
	} else {
		dst = dst[:nValues]
	}

	bs := newBitReader(data[refsBytes+widthsBytes+lensBytes:])
	idx := 0
	for g := 0; g < h.NG; g++ {
		W := int(h.RefW) + int(widths[g])
		actualLen := int(h.RefL) + int(lens[g])*int(h.LIncr)
		if g == h.NG-1 {
			actualLen = int(h.LastL)
		}
		ref := int64(refs[g])

		// W==0 means every value in the group equals the reference.
		// Missing-value handling: for mvm>=1, a reference equal to
		// 2^h.NBits-1 means the *entire group* is the missing primary value.
		groupMissing := h.MVM > 0 && refs[g] == (uint32(1)<<uint(h.NBits))-1
		if W == 0 {
			if groupMissing {
				for k := 0; k < actualLen; k++ {
					dst[idx+k] = missingMarker
				}
			} else {
				for k := 0; k < actualLen; k++ {
					dst[idx+k] = ref
				}
			}
			idx += actualLen
			continue
		}

		// Per-group missing thresholds.
		missingPrim := uint32(1)<<uint(W) - 1
		missingSec := missingPrim - 1
		for k := 0; k < actualLen; k++ {
			x := bs.read(W)
			if h.MVM >= 1 && x == missingPrim {
				dst[idx+k] = missingMarker
			} else if h.MVM == 2 && x == missingSec {
				dst[idx+k] = missingMarker2
			} else {
				dst[idx+k] = ref + int64(x)
			}
		}
		idx += actualLen
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
