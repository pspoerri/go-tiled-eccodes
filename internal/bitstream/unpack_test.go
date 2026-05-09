package bitstream

import "testing"

// TestUnpackPaths exercises every special-case width plus the general path.
func TestUnpackPaths(t *testing.T) {
	// 8-bit: dst = src.
	src := []byte{0x01, 0x7f, 0xff, 0x80}
	got := Unpack(src, 8, 4, nil)
	want := []uint32{1, 127, 255, 128}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("8-bit: got[%d] = %d, want %d", i, got[i], want[i])
		}
	}

	// 16-bit big-endian.
	src = []byte{0x01, 0x02, 0xff, 0xfe}
	got = Unpack(src, 16, 2, nil)
	if got[0] != 0x0102 || got[1] != 0xfffe {
		t.Errorf("16-bit: %v, want [0x0102 0xfffe]", got)
	}

	// 24-bit big-endian.
	src = []byte{0xab, 0xcd, 0xef, 0x12, 0x34, 0x56}
	got = Unpack(src, 24, 2, nil)
	if got[0] != 0xabcdef || got[1] != 0x123456 {
		t.Errorf("24-bit: %v", got)
	}

	// 32-bit.
	src = []byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04}
	got = Unpack(src, 32, 2, nil)
	if got[0] != 0xdeadbeef || got[1] != 0x01020304 {
		t.Errorf("32-bit: %v", got)
	}

	// nbits == 0 → all zeros.
	got = Unpack(nil, 0, 5, nil)
	for i, v := range got {
		if v != 0 {
			t.Errorf("0-bit got[%d] = %d, want 0", i, v)
		}
	}

	// n == 0 → empty slice (no read).
	got = Unpack(nil, 12, 0, nil)
	if len(got) != 0 {
		t.Errorf("n=0 returned %d values, want 0", len(got))
	}

	// General path: 12-bit values. Encode 0x123, 0x456, 0x789 as
	// [0x12 0x34 0x56 0x78 0x9?]. We pad the trailing nibble.
	src = []byte{0x12, 0x34, 0x56, 0x78, 0x90}
	got = Unpack(src, 12, 3, nil)
	if got[0] != 0x123 || got[1] != 0x456 || got[2] != 0x789 {
		t.Errorf("12-bit general: %v", got)
	}

	// dst reuse: passing a sized slice avoids the allocation.
	dst := make([]uint32, 0, 4)
	dst = Unpack([]byte{0x11, 0x22}, 8, 2, dst)
	if cap(dst) < 4 || dst[0] != 0x11 || dst[1] != 0x22 {
		t.Errorf("dst reuse failed: %v cap=%d", dst, cap(dst))
	}
}

func TestUnpackInto(t *testing.T) {
	// UnpackInto reinterprets the unsigned packing as int32 (caller layer
	// applies signed semantics).
	src := []byte{0xff, 0xff, 0xff, 0xff}
	out := UnpackInto(src, 16, 2, nil)
	if len(out) != 2 || out[0] != int32(0xffff) || out[1] != int32(0xffff) {
		t.Errorf("UnpackInto 16-bit = %v, want [0xffff 0xffff]", out)
	}

	// 8-bit fast path.
	out = UnpackInto([]byte{0x01, 0x7f, 0xff}, 8, 3, nil)
	if out[0] != 1 || out[1] != 127 || out[2] != 255 {
		t.Errorf("UnpackInto 8-bit = %v", out)
	}

	// 24-bit fast path.
	out = UnpackInto([]byte{0xab, 0xcd, 0xef, 0x12, 0x34, 0x56}, 24, 2, nil)
	if out[0] != int32(0xabcdef) || out[1] != int32(0x123456) {
		t.Errorf("UnpackInto 24-bit = %v", out)
	}

	// 32-bit fast path.
	out = UnpackInto([]byte{0xde, 0xad, 0xbe, 0xef}, 32, 1, nil)
	if uint32(out[0]) != 0xdeadbeef {
		t.Errorf("UnpackInto 32-bit = %v", out)
	}

	// 0-bit and n=0 short-circuits.
	out = UnpackInto(nil, 0, 3, nil)
	for i, v := range out {
		if v != 0 {
			t.Errorf("UnpackInto 0-bit out[%d] = %d, want 0", i, v)
		}
	}
	out = UnpackInto(nil, 12, 0, nil)
	if len(out) != 0 {
		t.Errorf("UnpackInto n=0 returned %d values, want 0", len(out))
	}

	// General path: 12-bit values matching the Unpack reference.
	out = UnpackInto([]byte{0x12, 0x34, 0x56, 0x78, 0x90}, 12, 3, nil)
	if out[0] != 0x123 || out[1] != 0x456 || out[2] != 0x789 {
		t.Errorf("UnpackInto 12-bit general = %v", out)
	}

	// dst reuse path.
	dst := make([]int32, 4)
	out = UnpackInto([]byte{0x00, 0x80}, 8, 2, dst)
	if &out[0] != &dst[0] {
		t.Errorf("UnpackInto did not reuse dst")
	}
	if out[0] != 0 || out[1] != int32(0x80) {
		t.Errorf("UnpackInto values = %v, want [0 128]", out)
	}
}
