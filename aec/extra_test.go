package aec

import (
	"errors"
	"testing"
)

// TestSampleBytesAndDecodedSize locks in the storage-width rule that callers
// rely on to size dst, so it can't silently drift from the decoder's own logic.
func TestSampleBytesAndDecodedSize(t *testing.T) {
	cases := []struct {
		bps   int
		flags Flags
		want  int
	}{
		{1, 0, 1}, {8, 0, 1},
		{9, 0, 2}, {16, 0, 2},
		{17, 0, 4}, {24, 0, 4}, {32, 0, 4},
		{17, Data3Byte, 3}, {24, Data3Byte, 3},
		{25, Data3Byte, 4}, // 3-byte only applies to 17..24
	}
	for _, c := range cases {
		cfg := Config{BitsPerSample: c.bps, Flags: c.flags}
		if got := SampleBytes(cfg); got != c.want {
			t.Fatalf("SampleBytes(bps=%d flags=%d) = %d, want %d", c.bps, c.flags, got, c.want)
		}
		if got := DecodedSize(100, cfg); got != 100*c.want {
			t.Fatalf("DecodedSize(100, bps=%d) = %d, want %d", c.bps, got, 100*c.want)
		}
	}
}

// TestDecodeErrShortInput: an uncompressed block whose payload is truncated
// must return ErrShortInput, not a panic or silent short read.
func TestDecodeErrShortInput(t *testing.T) {
	// id_len=3, idMax=7 -> 0b111 selects uncompressed; then 8x 8-bit samples
	// are expected, but the stream ends right after the id byte.
	cfg := Config{BitsPerSample: 8, BlockSize: 8, RSI: 4}
	src := []byte{0xE0} // 111 00000 — id=7, then EOF
	dst := make([]byte, 8)
	n, err := Decode(dst, src, cfg)
	if !errors.Is(err, ErrShortInput) {
		t.Fatalf("err = %v (n=%d), want ErrShortInput", err, n)
	}
}

// TestDecodeErrDataSecondExtension: a second-extension gamma above the table
// bound (SE_TABLE_SIZE=90) is a malformed stream and must return ErrData.
func TestDecodeErrDataSecondExtension(t *testing.T) {
	cfg := Config{BitsPerSample: 8, BlockSize: 8, RSI: 4} // no preprocess
	var bw bitWriter
	bw.put(0, 3) // id=0 -> low entropy
	bw.put(1, 1) // sub-id=1 -> second extension
	// First gamma = 91 (> seTableSize): 91 zero bits then a terminating 1.
	bw.put(0, 30)
	bw.put(0, 30)
	bw.put(0, 31)
	bw.put(1, 1)
	dst := make([]byte, 8)
	n, err := Decode(dst, bw.bytes(), cfg)
	if !errors.Is(err, ErrData) {
		t.Fatalf("err = %v (n=%d), want ErrData", err, n)
	}
}

// TestDecode3ByteEndianness exercises the 24-bit (Data3Byte) output path in
// both byte orders — the case the GRIB sweep only covered with DataMSB set.
func TestDecode3ByteEndianness(t *testing.T) {
	samples := []uint32{0x123456, 0xABCDEF, 0x000001, 0xFFFFFF, 0x800000, 0x00FF00, 0x010203, 0x0F0F0F}
	for _, msb := range []bool{true, false} {
		flags := Data3Byte
		if msb {
			flags |= DataMSB
		}
		cfg := Config{BitsPerSample: 24, BlockSize: 8, RSI: 4, Flags: flags}
		// id_len=5, idMax=31 -> uncompressed; then 8x 24-bit raw samples.
		var bw bitWriter
		bw.put(31, 5)
		for _, s := range samples {
			bw.put(s, 24)
		}
		dst := make([]byte, len(samples)*3)
		n, err := Decode(dst, bw.bytes(), cfg)
		if err != nil {
			t.Fatalf("msb=%v decode: %v", msb, err)
		}
		if n != len(dst) {
			t.Fatalf("msb=%v wrote %d bytes, want %d", msb, n, len(dst))
		}
		for i, s := range samples {
			o := i * 3
			var b0, b1, b2 byte
			if msb {
				b0, b1, b2 = byte(s>>16), byte(s>>8), byte(s)
			} else {
				b0, b1, b2 = byte(s), byte(s>>8), byte(s>>16)
			}
			if dst[o] != b0 || dst[o+1] != b1 || dst[o+2] != b2 {
				t.Fatalf("msb=%v sample %d = [%02x %02x %02x], want [%02x %02x %02x]",
					msb, i, dst[o], dst[o+1], dst[o+2], b0, b1, b2)
			}
		}
	}
}
