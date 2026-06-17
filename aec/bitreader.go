package aec

import "math"

// bitReader pulls bits MSB-first from src. Valid (unconsumed) bits occupy the
// low `cnt` bits of acc, the oldest unconsumed bit being the most significant
// of those. This mirrors libaec's bits_ask/bits_get/bits_drop accumulator.
type bitReader struct {
	src []byte
	pos int    // index of next byte to load
	acc uint64 // bit accumulator; meaningful bits are acc[cnt-1 .. 0]
	cnt int    // number of valid bits in acc (stays < 56)
}

// ask ensures at least n (<=32) bits are buffered, loading bytes big-endian.
// Returns false if src is exhausted first.
func (b *bitReader) ask(n int) bool {
	for b.cnt < n {
		if b.pos >= len(b.src) {
			return false
		}
		b.acc = b.acc<<8 | uint64(b.src[b.pos])
		b.pos++
		b.cnt += 8
	}
	return true
}

// getBits reads the next n MSB-first bits (n in 0..32). The first bit read is
// the most significant of the result.
func (b *bitReader) getBits(n int) (uint32, bool) {
	if n == 0 {
		return 0, true
	}
	if !b.ask(n) {
		return 0, false
	}
	v := uint32((b.acc >> uint(b.cnt-n)) & (math.MaxUint64 >> uint(64-n)))
	b.cnt -= n
	return v, true
}

// getFS reads a fundamental-sequence value: the number of consecutive 0 bits
// before the next 1 bit. The terminating 1 is consumed. Mirrors fs_ask+fs_drop.
func (b *bitReader) getFS() (uint32, bool) {
	var fs uint32
	if !b.ask(1) {
		return 0, false
	}
	for b.acc&(uint64(1)<<uint(b.cnt-1)) == 0 {
		if b.cnt == 1 {
			if b.pos >= len(b.src) {
				return 0, false
			}
			b.acc = b.acc<<8 | uint64(b.src[b.pos])
			b.pos++
			b.cnt += 8
		}
		fs++
		b.cnt--
	}
	b.cnt-- // drop the terminating 1
	return fs, true
}
