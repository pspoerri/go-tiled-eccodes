package writer

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// TestPutI16SM round-trips a few values through putI16SM and verifies the
// sign-magnitude encoding (most-significant bit = sign, remainder magnitude).
func TestPutI16SM(t *testing.T) {
	cases := []struct {
		v    int16
		want []byte
	}{
		{0, []byte{0x00, 0x00}},
		{1, []byte{0x00, 0x01}},
		{-1, []byte{0x80, 0x01}},
		{32767, []byte{0x7f, 0xff}},
		{-32767, []byte{0xff, 0xff}},
	}
	for _, c := range cases {
		b := make([]byte, 2)
		putI16SM(b, c.v)
		if !bytes.Equal(b, c.want) {
			t.Errorf("putI16SM(%d) = %x, want %x", c.v, b, c.want)
		}
	}
}

func TestPutI32SM(t *testing.T) {
	cases := []struct {
		v    int32
		want []byte
	}{
		{0, []byte{0x00, 0x00, 0x00, 0x00}},
		{42, []byte{0x00, 0x00, 0x00, 0x2a}},
		{-42, []byte{0x80, 0x00, 0x00, 0x2a}},
		{0x7fffffff, []byte{0x7f, 0xff, 0xff, 0xff}},
		{-0x7fffffff, []byte{0xff, 0xff, 0xff, 0xff}},
	}
	for _, c := range cases {
		b := make([]byte, 4)
		putI32SM(b, c.v)
		if !bytes.Equal(b, c.want) {
			t.Errorf("putI32SM(%d) = %x, want %x", c.v, b, c.want)
		}
	}
}

// TestEncodeMessageEntryPoint covers the exported wrapper, which is just
// a delegate to encodeMessage but currently uncovered.
func TestEncodeMessageEntryPoint(t *testing.T) {
	g := NewLatLon(2, 2, 1, 0, 1, 1)
	f := Field{
		ReferenceTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Grid:          g,
		Values:        []float64{1, 2, 3, 4},
		NumBits:       8,
	}
	out, err := EncodeMessage([]Field{f})
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("GRIB")) {
		t.Errorf("output does not start with GRIB magic")
	}
	totalLen := binary.BigEndian.Uint64(out[8:16])
	if int(totalLen) != len(out) {
		t.Errorf("S0 totalLen = %d, len(out) = %d", totalLen, len(out))
	}
	if string(out[len(out)-4:]) != "7777" {
		t.Errorf("output does not end with 7777 trailer")
	}

	// Empty fields list returns an explicit error rather than panicking.
	if _, err := EncodeMessage(nil); err == nil {
		t.Errorf("EncodeMessage(nil) returned nil error, want non-nil")
	}
}

// TestPutSMExports covers the exported wrappers PutI16SM / PutI32SM, which
// the writer package re-exports for callers writing custom grid templates.
func TestPutSMExports(t *testing.T) {
	b := make([]byte, 4)
	PutI16SM(b, -7)
	if !bytes.Equal(b[:2], []byte{0x80, 0x07}) {
		t.Errorf("PutI16SM(-7) = %x, want 80 07", b[:2])
	}
	PutI32SM(b, -7)
	if !bytes.Equal(b, []byte{0x80, 0x00, 0x00, 0x07}) {
		t.Errorf("PutI32SM(-7) = %x, want 80 00 00 07", b)
	}
}

// TestPutAngleVariants exercises both signed branches of PutAngle (the
// existing tests only cover one direction).
func TestPutAngleVariants(t *testing.T) {
	b := make([]byte, 4)
	PutAngle(b, -45.5)
	// -45.5° at micro-degree resolution = 45_500_000 with sign bit set.
	want := uint32(0x80000000 | 45_500_000)
	if got := binary.BigEndian.Uint32(b); got != want {
		t.Errorf("PutAngle(-45.5) = %#x, want %#x", got, want)
	}

	PutAngle(b, 45.5)
	want = 45_500_000
	if got := binary.BigEndian.Uint32(b); got != want {
		t.Errorf("PutAngle(45.5) = %#x, want %#x", got, want)
	}
}
