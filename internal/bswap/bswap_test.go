package bswap

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestUnsignedReaders(t *testing.T) {
	b := make([]byte, 16)
	binary.BigEndian.PutUint64(b, 0x0123456789abcdef)
	binary.BigEndian.PutUint64(b[8:], 0xfedcba9876543210)

	if got := U8(b, 0); got != 0x01 {
		t.Errorf("U8 = %#x, want 0x01", got)
	}
	if got := U16(b, 0); got != 0x0123 {
		t.Errorf("U16 = %#x, want 0x0123", got)
	}
	if got := U32(b, 0); got != 0x01234567 {
		t.Errorf("U32 = %#x, want 0x01234567", got)
	}
	if got := U64(b, 0); got != 0x0123456789abcdef {
		t.Errorf("U64 = %#x, want 0x0123456789abcdef", got)
	}
	if got := U64(b, 8); got != 0xfedcba9876543210 {
		t.Errorf("U64 @8 = %#x, want 0xfedcba9876543210", got)
	}
}

func TestSignedMagnitude(t *testing.T) {
	// I16SM: positive max
	b := []byte{0x7f, 0xff}
	if got := I16SM(b, 0); got != 0x7fff {
		t.Errorf("I16SM(0x7fff) = %d, want %d", got, 0x7fff)
	}
	// Negative max — sign-magnitude, *not* two's complement.
	b = []byte{0xff, 0xff}
	if got := I16SM(b, 0); got != -0x7fff {
		t.Errorf("I16SM(0xffff) = %d, want -32767", got)
	}
	// Positive zero
	b = []byte{0x00, 0x00}
	if got := I16SM(b, 0); got != 0 {
		t.Errorf("I16SM(0x0000) = %d, want 0", got)
	}
	// Negative zero (sign bit set, magnitude 0)
	b = []byte{0x80, 0x00}
	if got := I16SM(b, 0); got != 0 {
		t.Errorf("I16SM(0x8000) = %d, want 0", got)
	}

	// I32SM
	b = []byte{0x80, 0x00, 0x00, 0x05}
	if got := I32SM(b, 0); got != -5 {
		t.Errorf("I32SM(0x80000005) = %d, want -5", got)
	}
	b = []byte{0x00, 0x00, 0x00, 0x05}
	if got := I32SM(b, 0); got != 5 {
		t.Errorf("I32SM(0x00000005) = %d, want 5", got)
	}
}

func TestFloats(t *testing.T) {
	// F32 round-trip
	b := make([]byte, 8)
	binary.BigEndian.PutUint32(b, math.Float32bits(273.15))
	if got := F32(b, 0); got != 273.15 {
		t.Errorf("F32 = %v, want 273.15", got)
	}

	// F64 round-trip
	binary.BigEndian.PutUint64(b, math.Float64bits(math.Pi))
	if got := F64(b, 0); got != math.Pi {
		t.Errorf("F64 = %v, want π", got)
	}
}
