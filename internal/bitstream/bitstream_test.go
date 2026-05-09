package bitstream

import "testing"

// pack writes n values, each nbits wide, big-endian / MSB-first.
func pack(values []uint32, nbits int) []byte {
	totalBits := nbits * len(values)
	totalBytes := (totalBits + 7) / 8
	out := make([]byte, totalBytes)
	bitPos := 0
	for _, v := range values {
		for b := nbits - 1; b >= 0; b-- {
			bit := (v >> uint(b)) & 1
			byteIdx := bitPos >> 3
			bitInByte := 7 - (bitPos & 7)
			out[byteIdx] |= byte(bit << uint(bitInByte))
			bitPos++
		}
	}
	return out
}

func TestUnpackRoundtrip(t *testing.T) {
	for _, nbits := range []int{1, 3, 7, 8, 11, 16, 19, 24, 25, 32} {
		t.Run("", func(t *testing.T) {
			max := uint32(1)<<uint(nbits) - 1
			seed := []uint32{0, 1, 2, 3, max, max - 1, 42}
			vals := make([]uint32, 0, len(seed))
			for _, v := range seed {
				vals = append(vals, v&max)
			}
			packed := pack(vals, nbits)
			got := Unpack(packed, nbits, len(vals), nil)
			if len(got) != len(vals) {
				t.Fatalf("len = %d, want %d", len(got), len(vals))
			}
			for i := range vals {
				if got[i] != vals[i] {
					t.Errorf("nbits=%d [%d]: got %d, want %d", nbits, i, got[i], vals[i])
				}
			}
		})
	}
}

func TestUnpackZeroBits(t *testing.T) {
	got := Unpack(nil, 0, 5, nil)
	for i, v := range got {
		if v != 0 {
			t.Errorf("[%d] = %d, want 0", i, v)
		}
	}
}
