package bufpool

import "testing"

func TestBucket(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 1},
		{1, 1},
		{2, 2},
		{3, 4},
		{1024, 1024},
		{1025, 2048},
	}
	for _, c := range cases {
		if got := bucket(c.in); got != c.want {
			t.Errorf("bucket(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestF64GetPut(t *testing.T) {
	// Borrowed slice has len == bucket(n) and cap == bucket(n).
	s := P.GetF64(100)
	if cap(s) < 100 || len(s) != cap(s) {
		t.Fatalf("GetF64(100) = len %d cap %d, want both 128", len(s), cap(s))
	}
	for i := range s {
		s[i] = float64(i)
	}
	P.PutF64(s)

	// A second Get of the same bucket should reuse the buffer (sync.Pool
	// guarantees aren't strict, but at minimum the cap should match).
	s2 := P.GetF64(100)
	if cap(s2) != cap(s) {
		t.Errorf("second Get cap = %d, want %d", cap(s2), cap(s))
	}
	P.PutF64(s2)

	// Different bucket → independent pool.
	big := P.GetF64(10_000)
	if cap(big) < 10_000 {
		t.Errorf("GetF64(10000) cap = %d, want ≥ 10000", cap(big))
	}
	P.PutF64(big)

	// Put with cap not matching any bucket is a silent no-op.
	stray := make([]float64, 17)
	P.PutF64(stray)
}

func TestF32GetPut(t *testing.T) {
	s := P.GetF32(100)
	if cap(s) < 100 {
		t.Fatalf("GetF32(100) cap = %d", cap(s))
	}
	P.PutF32(s)
	stray := make([]float32, 19)
	P.PutF32(stray)
}
