package aec

import (
	"errors"
	"testing"
)

func TestConfigValidation(t *testing.T) {
	good := Config{BitsPerSample: 16, BlockSize: 32, RSI: 128, Flags: DataPreprocess | DataMSB}
	cases := []struct {
		name string
		cfg  Config
		want error
	}{
		{"ok", good, nil},
		{"bps0", Config{BitsPerSample: 0, BlockSize: 32, RSI: 128}, ErrConfig},
		{"bps33", Config{BitsPerSample: 33, BlockSize: 32, RSI: 128}, ErrConfig},
		{"rsi0", Config{BitsPerSample: 16, BlockSize: 32, RSI: 0}, ErrConfig},
		{"rsi4097", Config{BitsPerSample: 16, BlockSize: 32, RSI: 4097}, ErrConfig},
		{"blk0", Config{BitsPerSample: 16, BlockSize: 0, RSI: 128}, ErrConfig},
		{"blkOdd", Config{BitsPerSample: 16, BlockSize: 31, RSI: 128}, ErrConfig},
		{"blk257", Config{BitsPerSample: 16, BlockSize: 258, RSI: 128}, ErrConfig},
		{"restrictedBps6", Config{BitsPerSample: 6, BlockSize: 32, RSI: 128, Flags: RestrictedCodes}, ErrConfig},
		{"restrictedBps4ok", Config{BitsPerSample: 4, BlockSize: 32, RSI: 128, Flags: RestrictedCodes}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := newDecoder(make([]byte, 1<<16), []byte{0, 0, 0, 0}, c.cfg)
			if !errors.Is(err, c.want) && err != c.want {
				t.Fatalf("newDecoder err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestDerivedParams(t *testing.T) {
	cases := []struct {
		bps, idLen, bps2 int
		flags            Flags
	}{
		{8, 3, 1, 0}, {1, 3, 1, 0}, {9, 4, 2, 0}, {16, 4, 2, 0},
		{17, 5, 4, 0}, {24, 5, 4, 0}, {24, 5, 3, Data3Byte}, {32, 5, 4, 0},
		{2, 1, 1, RestrictedCodes}, {4, 2, 1, RestrictedCodes},
	}
	for _, c := range cases {
		d, err := newDecoder(make([]byte, 1<<16), []byte{0}, Config{BitsPerSample: c.bps, BlockSize: 16, RSI: 8, Flags: c.flags})
		if err != nil {
			t.Fatalf("bps=%d: %v", c.bps, err)
		}
		if d.idLen != c.idLen || d.bytesPerSample != c.bps2 {
			t.Fatalf("bps=%d flags=%d: idLen=%d (want %d) bytesPerSample=%d (want %d)",
				c.bps, c.flags, d.idLen, c.idLen, d.bytesPerSample, c.bps2)
		}
	}
}

func TestSETable(t *testing.T) {
	tab := buildSETable()
	if len(tab) != 2*(seTableSize+1) {
		t.Fatalf("len = %d, want %d", len(tab), 2*(seTableSize+1))
	}
	// For every m, the decoded pair must invert the forward triangular map
	// gamma = (d0+d1)(d0+d1+1)/2 + d1.
	for m := 0; m <= seTableSize; m++ {
		total := tab[2*m] // d0 + d1
		ms := tab[2*m+1]  // row base = total*(total+1)/2
		d1 := m - ms
		d0 := total - d1
		if d0 < 0 || d1 < 0 || d1 > total {
			t.Fatalf("m=%d: bad pair d0=%d d1=%d total=%d", m, d0, d1, total)
		}
		gamma := (d0+d1)*(d0+d1+1)/2 + d1
		if gamma != m {
			t.Fatalf("m=%d: forward map gives %d (d0=%d d1=%d)", m, gamma, d0, d1)
		}
	}
	// Spot-check the first rows: m=0 ->(0,0); m=1->total1 base1; m=2->total2 base3.
	if tab[0] != 0 || tab[1] != 0 {
		t.Fatalf("m=0 entry = (%d,%d) want (0,0)", tab[0], tab[1])
	}
	if tab[2*1] != 1 || tab[2*1+1] != 1 {
		t.Fatalf("m=1 entry = (%d,%d) want (1,1)", tab[2], tab[3])
	}
}

// bitWriter writes bits MSB-first, the same order bitReader consumes them.
type bitWriter struct {
	buf []byte
	acc uint64
	cnt int
}

func (w *bitWriter) put(v uint32, n int) {
	w.acc = w.acc<<uint(n) | uint64(v&(1<<uint(n)-1))
	w.cnt += n
	for w.cnt >= 8 {
		w.cnt -= 8
		w.buf = append(w.buf, byte(w.acc>>uint(w.cnt)))
	}
}

func (w *bitWriter) bytes() []byte {
	out := w.buf
	if w.cnt > 0 {
		out = append(out, byte(w.acc<<uint(8-w.cnt))) // pad final byte with zeros
	}
	return out
}

// helper: build a uint32 sample buffer into MSB/LSB bytes the way Decode emits.
func packSamples(samples []uint32, bytesPer int, msb bool) []byte {
	out := make([]byte, len(samples)*bytesPer)
	for i, v := range samples {
		o := i * bytesPer
		switch bytesPer {
		case 1:
			out[o] = byte(v)
		case 2:
			if msb {
				out[o], out[o+1] = byte(v>>8), byte(v)
			} else {
				out[o], out[o+1] = byte(v), byte(v>>8)
			}
		case 4:
			if msb {
				out[o], out[o+1], out[o+2], out[o+3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
			} else {
				out[o], out[o+1], out[o+2], out[o+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
			}
		}
	}
	return out
}

// TestUncompNoPP: one block of 8 uncompressed 8-bit samples, no preprocessing.
// id_len=3, id_max=7 -> uncompressed id is 0b111. Then 8 raw 8-bit samples.
func TestUncompNoPP(t *testing.T) {
	cfg := Config{BitsPerSample: 8, BlockSize: 8, RSI: 2, Flags: 0}
	samples := []uint32{10, 20, 30, 40, 250, 1, 2, 3}
	// Bitstream: id (111) then each sample as 8 bits.
	var bw bitWriter
	bw.put(0b111, 3)
	for _, s := range samples {
		bw.put(s, 8)
	}
	dst := make([]byte, len(samples)) // exactly 8 bytes -> needed=8 samples (one block)
	n, err := Decode(dst, bw.bytes(), cfg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := packSamples(samples, 1, false)
	if n != len(want) || string(dst[:n]) != string(want) {
		t.Fatalf("got %v (n=%d), want %v", dst[:n], n, want)
	}
}

// TestSplitNoPP: id selects k-split. id_len=3, so id in 1..6 -> k=id-1.
// Use id=3 -> k=2. block_size=4, no preprocessing, 8-bit. For each sample:
// high part fs (unary, fs zeros then 1) and 2-bit remainder; sample=(fs<<2)|rem.
func TestSplitNoPP(t *testing.T) {
	cfg := Config{BitsPerSample: 8, BlockSize: 4, RSI: 4, Flags: 0}
	fs := []uint32{0, 1, 2, 0}                               // high parts
	rem := []uint32{1, 2, 3, 0}                              // 2-bit low parts
	want := []uint32{0<<2 | 1, 1<<2 | 2, 2<<2 | 3, 0<<2 | 0} // 1,6,11,0
	var bw bitWriter
	bw.put(3, 3)           // id=3 -> k=2
	for _, f := range fs { // FS: f zeros then a 1
		bw.put(1, int(f)+1)
	}
	for _, r := range rem {
		bw.put(r, 2)
	}
	dst := make([]byte, 4)
	n, err := Decode(dst, bw.bytes(), cfg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := dst[:n]
	for i := range want {
		if uint32(got[i]) != want[i] {
			t.Fatalf("sample %d = %d, want %d", i, got[i], want[i])
		}
	}
}

// TestSplitK0NoPP: id=1 -> k=0 (pure fundamental sequence, no remainder bits).
func TestSplitK0NoPP(t *testing.T) {
	cfg := Config{BitsPerSample: 8, BlockSize: 4, RSI: 4, Flags: 0}
	fs := []uint32{5, 0, 3, 7}
	var bw bitWriter
	bw.put(1, 3) // id=1 -> k=0
	for _, f := range fs {
		bw.put(1, int(f)+1)
	}
	dst := make([]byte, 4)
	n, err := Decode(dst, bw.bytes(), cfg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	for i, f := range fs {
		if uint32(dst[:n][i]) != f {
			t.Fatalf("sample %d = %d, want %d", i, dst[i], f)
		}
	}
}

// TestUncompPPUnsigned: preprocessing on, unsigned 16-bit. First sample is the
// raw reference; subsequent stored values are mapped residuals reversed by the
// predictor. We pick residuals that stay in range so the zig-zag branch applies.
func TestUncompPPUnsigned(t *testing.T) {
	cfg := Config{BitsPerSample: 16, BlockSize: 8, RSI: 2, Flags: DataPreprocess | DataMSB}
	// reference=1000. residuals d: 2->+1, 4->+2, 1->-1, 0->0, 6->+3, 3->-2, 8->+4
	// (even d -> +d/2, odd d -> -(d+1)/2), accumulated onto the predictor.
	stored := []uint32{1000, 2, 4, 1, 0, 6, 3, 8}
	wantSamples := []uint32{1000, 1001, 1003, 1002, 1002, 1005, 1003, 1007}
	var bw bitWriter
	bw.put(0b1111, 4) // id_len=4, id_max=15
	for _, s := range stored {
		bw.put(s, 16)
	}
	dst := make([]byte, len(stored)*2)
	n, err := Decode(dst, bw.bytes(), cfg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := packSamples(wantSamples, 2, true)
	if n != len(want) || string(dst[:n]) != string(want) {
		t.Fatalf("got %v, want %v", dst[:n], want)
	}
}
