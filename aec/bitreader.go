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
// When all buffered bits are zero, up to 7 bytes are loaded at once to
// amortise refill overhead for long zero-run FS values.
func (b *bitReader) getFS() (uint32, bool) {
	src := b.src
	pos := b.pos
	acc := b.acc
	cnt := b.cnt
	// Ensure at least 1 bit is available.
	if cnt == 0 {
		if pos >= len(src) {
			return 0, false
		}
		acc = acc<<8 | uint64(src[pos])
		pos++
		cnt = 8
	}
	var fs uint32
	for {
		// Align the cnt valid bits to the top of a 64-bit word and count
		// leading zeros in one instruction.
		window := acc << uint(64-cnt)
		lz := bits.LeadingZeros64(window)
		if lz < cnt {
			// Found the terminating 1 within the current window.
			fs += uint32(lz)
			b.acc, b.cnt, b.pos = acc, cnt-lz-1, pos
			return fs, true
		}
		// All cnt bits are zero; consume them and bulk-load more bytes.
		fs += uint32(cnt)
		cnt = 0
		avail := len(src) - pos
		if avail == 0 {
			b.acc, b.cnt, b.pos = acc, 0, pos
			return 0, false
		}
		// Load up to 7 bytes to fill the accumulator to ~56 bits.
		room := 7 // (56 - 0) >> 3
		if avail < room {
			room = avail
		}
		switch room {
		case 1:
			acc = uint64(src[pos])
			cnt = 8
		case 2:
			acc = uint64(src[pos])<<8 | uint64(src[pos+1])
			cnt = 16
		case 3:
			acc = uint64(src[pos])<<16 | uint64(src[pos+1])<<8 | uint64(src[pos+2])
			cnt = 24
		case 4:
			acc = uint64(src[pos])<<24 | uint64(src[pos+1])<<16 | uint64(src[pos+2])<<8 | uint64(src[pos+3])
			cnt = 32
		case 5:
			acc = uint64(src[pos])<<32 | uint64(src[pos+1])<<24 | uint64(src[pos+2])<<16 | uint64(src[pos+3])<<8 | uint64(src[pos+4])
			cnt = 40
		case 6:
			acc = uint64(src[pos])<<40 | uint64(src[pos+1])<<32 | uint64(src[pos+2])<<24 | uint64(src[pos+3])<<16 | uint64(src[pos+4])<<8 | uint64(src[pos+5])
			cnt = 48
		default: // 7
			acc = uint64(src[pos])<<48 | uint64(src[pos+1])<<40 | uint64(src[pos+2])<<32 | uint64(src[pos+3])<<24 | uint64(src[pos+4])<<16 | uint64(src[pos+5])<<8 | uint64(src[pos+6])
			cnt = 56
		}
		pos += room
	}
}
