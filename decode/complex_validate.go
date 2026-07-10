package decode

import (
	"github.com/pspoerri/go-tiled-eccodes/internal/bitstream"
	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

type complexMeta struct {
	refs    []uint32
	widths  []int
	lengths []int
	values  []byte
}

func parseComplexHeader(t []byte) (complexHeader, error) {
	if len(t) < 36 {
		return complexHeader{}, ErrBadComplexStream
	}
	rawNG := bswap.U32(t, 20)
	if uint64(rawNG) > uint64(maxInt()) {
		return complexHeader{}, ErrBadComplexStream
	}
	h := complexHeader{
		R:        bswap.F32(t, 0),
		E:        bswap.I16SM(t, 4),
		D:        bswap.I16SM(t, 6),
		NBits:    int(t[8]),
		OrigType: t[9],
		Split:    t[10],
		MVM:      t[11],
		MissingP: bswap.U32(t, 12),
		MissingS: bswap.U32(t, 16),
		NG:       int(rawNG),
		RefW:     t[24],
		WBits:    int(t[25]),
		RefL:     bswap.U32(t, 26),
		LIncr:    t[30],
		LastL:    bswap.U32(t, 31),
		LBits:    int(t[35]),
	}
	if h.NBits > 32 || h.WBits > 32 || h.LBits > 32 || h.MVM > 2 || h.RefW > 32 {
		return complexHeader{}, ErrBadComplexStream
	}
	return h, nil
}

func parseComplexMeta(h complexHeader, data []byte, nValues int) (complexMeta, error) {
	if nValues < 0 || h.NG <= 0 || (nValues > 0 && h.NG > nValues) || (nValues == 0 && h.NG > 1) {
		return complexMeta{}, ErrBadComplexStream
	}
	off := 0
	refsBytes, ok := packedBytes(h.NBits, h.NG)
	if !ok || refsBytes > len(data)-off {
		return complexMeta{}, ErrBadComplexStream
	}
	refs := bitstream.Unpack(data[off:off+refsBytes], h.NBits, h.NG, nil)
	off += refsBytes

	widthBytes, ok := packedBytes(h.WBits, h.NG)
	if !ok || widthBytes > len(data)-off {
		return complexMeta{}, ErrBadComplexStream
	}
	packedWidths := bitstream.Unpack(data[off:off+widthBytes], h.WBits, h.NG, nil)
	off += widthBytes

	lengthBytes, ok := packedBytes(h.LBits, h.NG)
	if !ok || lengthBytes > len(data)-off {
		return complexMeta{}, ErrBadComplexStream
	}
	packedLengths := bitstream.Unpack(data[off:off+lengthBytes], h.LBits, h.NG, nil)
	off += lengthBytes

	widths := make([]int, h.NG)
	lengths := make([]int, h.NG)
	var totalLength, valueBits uint64
	for group := 0; group < h.NG; group++ {
		width := uint64(h.RefW) + uint64(packedWidths[group])
		if width > 32 {
			return complexMeta{}, ErrBadComplexStream
		}
		length := uint64(h.RefL) + uint64(packedLengths[group])*uint64(h.LIncr)
		if group == h.NG-1 {
			length = uint64(h.LastL)
		}
		if length > uint64(maxInt()) {
			return complexMeta{}, ErrBadComplexStream
		}
		totalLength += length
		if totalLength > uint64(nValues) {
			return complexMeta{}, ErrBadComplexStream
		}
		if length != 0 && width > (^uint64(0)-valueBits)/length {
			return complexMeta{}, ErrBadComplexStream
		}
		valueBits += length * width
		widths[group] = int(width)
		lengths[group] = int(length)
	}
	if totalLength != uint64(nValues) || valueBits > uint64(len(data)-off)*8 {
		return complexMeta{}, ErrBadComplexStream
	}
	return complexMeta{refs: refs, widths: widths, lengths: lengths, values: data[off:]}, nil
}

func packedBytes(width, count int) (int, bool) {
	if width < 0 || width > 32 || count < 0 {
		return 0, false
	}
	bits := uint64(width) * uint64(count)
	bytes := (bits + 7) >> 3
	if bytes > uint64(maxInt()) {
		return 0, false
	}
	return int(bytes), true
}

func maxInt() int { return int(^uint(0) >> 1) }

func allOnes(width int) uint32 {
	if width <= 0 {
		return 0
	}
	if width >= 32 {
		return ^uint32(0)
	}
	return uint32(1)<<uint(width) - 1
}
