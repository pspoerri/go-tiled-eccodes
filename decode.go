package grib

import (
	"math/bits"

	"github.com/pspoerri/go-tiled-eccodes/decode"
)

// DecodeFloat64 returns the message's grid as a contiguous []float64 in the
// message's own storage (scanning) order — the order Section 7 packs the
// values, which follows the scanning-mode bits. For a grid whose scan is not
// natural this is NOT W→E/N→S; use DecodeNatural when you need guaranteed
// natural order. The first call decodes; subsequent calls reuse the cached
// buffer (the cache is invalidated by File.Close).
//
// Caller may pass dst to receive a copy of the cached values; if dst is nil
// or undersized, a new slice is returned. The returned slice is owned by the
// caller — internal mutation of the cache after a call would be a bug.
func (m *Message) DecodeFloat64(dst []float64) ([]float64, error) {
	src, err := m.decodeCached()
	if err != nil {
		return nil, err
	}
	if dst == nil || cap(dst) < len(src) {
		dst = make([]float64, len(src))
	} else {
		dst = dst[:len(src)]
	}
	copy(dst, src)
	return dst, nil
}

// DecodeFloat32 mirrors DecodeFloat64 but returns float32. Conversion happens
// in the same pass that copies out of the cache.
func (m *Message) DecodeFloat32(dst []float32) ([]float32, error) {
	src, err := m.decodeCached()
	if err != nil {
		return nil, err
	}
	if dst == nil || cap(dst) < len(src) {
		dst = make([]float32, len(src))
	} else {
		dst = dst[:len(src)]
	}
	for i, v := range src {
		dst[i] = float32(v)
	}
	return dst, nil
}

// decodeCached lazily decodes the message and caches the resulting []float64
// for reuse by ValueAt and tile renderers.
func (m *Message) decodeCached() ([]float64, error) {
	m.bitmapMu.Lock()
	m.decodeStarted = true
	m.bitmapMu.Unlock()
	m.once.Do(func() {
		m.cached, m.decErr = m.decodeNow()
	})
	return m.cached, m.decErr
}

// decodeNow runs the actual Section 5 → Section 7 decode for the message,
// applying the bitmap (Section 6) if one is present. The returned slice is in
// the message's *storage* (scan) order: values come out of Section 7 in the
// order the scanning-mode bits define and decode does not re-order them.
//
// Why we keep storage order rather than un-scrambling here: the renderer's
// source closure routes (i, j) → buffer offset through Grid.Index, so the
// fast path stays zero-copy. DecodeNatural is the accessor that materialises a
// natural-order (W→E, N→S) copy for callers that want one.
func (m *Message) decodeNow() ([]float64, error) {
	g, err := m.Grid()
	if err != nil {
		return nil, err
	}

	nTotal := g.NumPoints()
	bitmap, nset, err := m.bitmapAndCount(nTotal)
	if err != nil {
		return nil, err
	}

	tmpl := m.S5.Template()
	data := m.S7.Payload()

	var packed []float64
	switch m.S5.TemplateNumber() {
	case 0:
		packed, err = decode.Simple(tmpl, data, nset, nil)
	case 2:
		packed, err = decode.Complex(tmpl, data, nset, nil)
	case 3:
		packed, err = decode.ComplexSpatialDifferencing(tmpl, data, nset, nil)
	case 4:
		packed, err = decode.IEEE(tmpl, data, nset, nil)
	case 40:
		packed, err = decode.JPEG2000(tmpl, data, nset, nil)
	case 41:
		packed, err = decode.PNG(tmpl, data, nset, nil)
	case 42:
		packed, err = decode.CCSDS(tmpl, data, nset, nil)
	case 50:
		packed, err = decode.SpectralSimple(tmpl, data, nset, nil)
	case 61:
		packed, err = decode.LogPreprocessed(tmpl, data, nset, nil)
	default:
		return nil, ErrUnsupportedPacking
	}
	if err != nil {
		return nil, err
	}

	// Output stays in *storage* order. The renderer's source closure routes
	// (i, j) → buffer offset through Grid.Index, which understands scanning
	// modes and (for reduced grids) variable row widths. This keeps the
	// fast path zero-copy and avoids a duplicated buffer for non-natural
	// scanning modes.
	//
	// When no bitmap is present, packed is already the full nTotal-sized
	// storage-order buffer — return it directly rather than allocating an
	// 8 MB copy via ApplyBitmap. This is the common path for files without
	// missing values and saves one large allocation per cold decode.
	if bitmap == nil {
		return packed, nil
	}
	return decode.ApplyBitmap(bitmap, packed, nTotal, nil), nil
}

// bitmapAndCount inspects Section 6 and returns the bitmap byte slice (or nil
// if no bitmap), plus the number of set bits (i.e. the number of values
// actually packed in Section 7). When indicator==255, every point has a
// value and nset == nTotal.
func (m *Message) bitmapAndCount(nTotal int) ([]byte, int, error) {
	switch m.S6.Indicator() {
	case 255:
		return nil, nTotal, nil
	case 0:
		bm := m.S6.Bits()
		if len(bm)*8 < nTotal {
			return nil, 0, ErrTruncated
		}
		nset := 0
		full := nTotal >> 3
		for i := 0; i < full; i++ {
			nset += bits.OnesCount8(bm[i])
		}
		// Last partial byte: only the high nTotal&7 bits are valid.
		if rem := nTotal & 7; rem != 0 {
			mask := byte(0xff) << uint(8-rem)
			nset += bits.OnesCount8(bm[full] & mask)
		}
		return bm[:(nTotal+7)>>3], nset, nil
	case 254:
		// Indicator 254 ("reuse a bitmap previously defined in this
		// message") is normally rewritten by the indexer to point at the
		// earlier Section 6 — see splitMessage in index.go. Reaching this
		// branch means the very first Section 6 of a message had indicator
		// 254 with no preceding bitmap, which is malformed.
		return nil, 0, ErrUnsupportedTemplate
	default:
		if indicator := m.S6.Indicator(); indicator >= 1 && indicator <= 253 {
			m.bitmapMu.Lock()
			bm := m.predefinedBitmap
			m.bitmapMu.Unlock()
			if bm == nil {
				return nil, 0, ErrPredefinedBitmap
			}
			if len(bm) < (nTotal+7)>>3 {
				return nil, 0, ErrTruncated
			}
			return bm[:(nTotal+7)>>3], bitmapPopulation(bm, nTotal), nil
		}
		return nil, 0, ErrUnsupportedTemplate
	}
}

func bitmapPopulation(bitmap []byte, n int) int {
	count := 0
	for i := 0; i < n>>3; i++ {
		count += bits.OnesCount8(bitmap[i])
	}
	if rem := n & 7; rem != 0 {
		mask := byte(0xff) << uint(8-rem)
		count += bits.OnesCount8(bitmap[n>>3] & mask)
	}
	return count
}
