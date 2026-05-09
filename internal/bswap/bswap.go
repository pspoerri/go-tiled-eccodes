// Package bswap holds big-endian readers + GRIB sign-magnitude conversions.
// All multi-byte fields in GRIB2 are big-endian. Several integer fields
// (latitudes, longitudes, scale factors) use a sign-magnitude representation
// rather than two's complement: the most significant bit is the sign, the
// remaining bits the magnitude.
package bswap

import (
	"encoding/binary"
	"math"
)

func U8(b []byte, off int) uint8   { return b[off] }
func U16(b []byte, off int) uint16 { return binary.BigEndian.Uint16(b[off:]) }
func U32(b []byte, off int) uint32 { return binary.BigEndian.Uint32(b[off:]) }
func U64(b []byte, off int) uint64 { return binary.BigEndian.Uint64(b[off:]) }

// I16SM decodes a 16-bit GRIB sign-magnitude integer.
func I16SM(b []byte, off int) int16 {
	v := U16(b, off)
	if v&0x8000 != 0 {
		return -int16(v & 0x7fff)
	}
	return int16(v)
}

// I32SM decodes a 32-bit GRIB sign-magnitude integer.
func I32SM(b []byte, off int) int32 {
	v := U32(b, off)
	if v&0x80000000 != 0 {
		return -int32(v & 0x7fffffff)
	}
	return int32(v)
}

// F32 reads a big-endian IEEE-754 float32.
func F32(b []byte, off int) float32 {
	return math.Float32frombits(U32(b, off))
}

// F64 reads a big-endian IEEE-754 float64.
func F64(b []byte, off int) float64 {
	return math.Float64frombits(U64(b, off))
}
