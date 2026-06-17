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
