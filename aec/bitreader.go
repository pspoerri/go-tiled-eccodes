package aec

import "math/bits"

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
	v := uint32((b.acc >> uint(b.cnt-n)) & (^uint64(0) >> uint(64-n)))
	b.cnt -= n
	return v, true
}

// getFS reads a fundamental-sequence value: the number of consecutive 0 bits
// before the next 1 bit. The terminating 1 is consumed. Mirrors fs_ask+fs_drop.
//
// Uses bits.LeadingZeros64 to scan the accumulator window in bulk rather than
// bit by bit, reducing loop iterations for moderate and long FS values.
func (b *bitReader) getFS() (uint32, bool) {
	// Ensure at least 1 bit is available.
	if b.cnt == 0 {
		if b.pos >= len(b.src) {
			return 0, false
		}
		b.acc = b.acc<<8 | uint64(b.src[b.pos])
		b.pos++
		b.cnt = 8
	}
	var fs uint32
	for {
		// Align the cnt valid bits to the top of a 64-bit word and count
		// leading zeros in one instruction.
		window := b.acc << uint(64-b.cnt)
		lz := bits.LeadingZeros64(window)
		if lz < b.cnt {
			// Found the terminating 1 within the current window.
			fs += uint32(lz)
			b.cnt -= lz + 1 // consume leading zeros + terminating 1
			return fs, true
		}
		// All cnt bits are zero; consume them and load one more byte.
		fs += uint32(b.cnt)
		if b.pos >= len(b.src) {
			return 0, false
		}
		b.acc = b.acc<<8 | uint64(b.src[b.pos])
		b.pos++
		b.cnt = 8
	}
}
