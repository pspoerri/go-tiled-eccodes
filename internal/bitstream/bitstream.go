// Package bitstream is a high-throughput big-endian bit unpacker for GRIB2
// payloads. The "general" path uses a 64-bit accumulator drained left-to-right;
// dedicated fast paths for the most common widths (8, 16, 24, 32 bits per
// value) avoid the inner-loop branches.
//
// All functions assume nbits ∈ [0, 32]. nbits == 0 is a valid GRIB2 case
// meaning "every value equals the reference value"; callers handle that
// before unpacking — Unpack here returns an empty/zero buffer when nbits==0.
package bitstream

// Unpack decodes n big-endian unsigned integers, each nbits wide, from src
// into dst. dst must have len(dst) >= n. Returns dst[:n].
func Unpack(src []byte, nbits int, n int, dst []uint32) []uint32 {
	if cap(dst) < n {
		dst = make([]uint32, n)
	} else {
		dst = dst[:n]
	}
	if n == 0 {
		return dst
	}
	if nbits == 0 {
		for i := range dst {
			dst[i] = 0
		}
		return dst
	}

	switch nbits {
	case 8:
		for i := 0; i < n; i++ {
			dst[i] = uint32(src[i])
		}
		return dst
	case 16:
		// Manual byte ops beat binary.BigEndian.Uint16 here because the
		// compiler can hoist the bounds check out of the loop given a
		// length-bound src slice — the wrapper requires a fresh reslice
		// per iteration which leaves the bound untouched.
		_ = src[2*n-1]
		for i := 0; i < n; i++ {
			j := i * 2
			dst[i] = uint32(src[j])<<8 | uint32(src[j+1])
		}
		return dst
	case 24:
		_ = src[3*n-1]
		for i := 0; i < n; i++ {
			j := i * 3
			dst[i] = uint32(src[j])<<16 | uint32(src[j+1])<<8 | uint32(src[j+2])
		}
		return dst
	case 32:
		_ = src[4*n-1]
		for i := 0; i < n; i++ {
			j := i * 4
			dst[i] = uint32(src[j])<<24 | uint32(src[j+1])<<16 | uint32(src[j+2])<<8 | uint32(src[j+3])
		}
		return dst
	}

	// General case: shift register over a 64-bit accumulator.
	mask := uint64(1)<<uint(nbits) - 1
	var acc uint64
	bits := 0
	si := 0
	for i := 0; i < n; i++ {
		for bits < nbits {
			acc = (acc << 8) | uint64(src[si])
			si++
			bits += 8
		}
		shift := uint(bits - nbits)
		dst[i] = uint32((acc >> shift) & mask)
		bits -= nbits
		// Trim accumulator to the bits we still own to keep the shift stable.
		if bits > 0 {
			acc &= uint64(1)<<uint(bits) - 1
		} else {
			acc = 0
		}
	}
	return dst
}

// UnpackInto decodes into a caller-provided []int32 (some packings interpret
// values as signed via spatial differencing). The packing itself is unsigned,
// so this is bit-pattern equivalent to Unpack + cast — but written directly
// against the int32 destination so we don't allocate a uint32 scratch buffer.
func UnpackInto(src []byte, nbits int, n int, dst []int32) []int32 {
	if cap(dst) < n {
		dst = make([]int32, n)
	} else {
		dst = dst[:n]
	}
	if n == 0 {
		return dst
	}
	if nbits == 0 {
		for i := range dst {
			dst[i] = 0
		}
		return dst
	}

	switch nbits {
	case 8:
		for i := 0; i < n; i++ {
			dst[i] = int32(src[i])
		}
		return dst
	case 16:
		_ = src[2*n-1]
		for i := 0; i < n; i++ {
			j := i * 2
			dst[i] = int32(uint32(src[j])<<8 | uint32(src[j+1]))
		}
		return dst
	case 24:
		_ = src[3*n-1]
		for i := 0; i < n; i++ {
			j := i * 3
			dst[i] = int32(uint32(src[j])<<16 | uint32(src[j+1])<<8 | uint32(src[j+2]))
		}
		return dst
	case 32:
		_ = src[4*n-1]
		for i := 0; i < n; i++ {
			j := i * 4
			dst[i] = int32(uint32(src[j])<<24 | uint32(src[j+1])<<16 | uint32(src[j+2])<<8 | uint32(src[j+3]))
		}
		return dst
	}

	// General case: shift register over a 64-bit accumulator, into int32.
	mask := uint64(1)<<uint(nbits) - 1
	var acc uint64
	bits := 0
	si := 0
	for i := 0; i < n; i++ {
		for bits < nbits {
			acc = (acc << 8) | uint64(src[si])
			si++
			bits += 8
		}
		shift := uint(bits - nbits)
		dst[i] = int32(uint32((acc >> shift) & mask))
		bits -= nbits
		if bits > 0 {
			acc &= uint64(1)<<uint(bits) - 1
		} else {
			acc = 0
		}
	}
	return dst
}
