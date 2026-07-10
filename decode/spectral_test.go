package decode

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestSpectralSimple(t *testing.T) {
	template := make([]byte, 13)
	binary.BigEndian.PutUint32(template[0:], math.Float32bits(2))
	template[8] = 0
	binary.BigEndian.PutUint32(template[9:], math.Float32bits(1.5))

	got, err := SpectralSimple(template, nil, 4, nil)
	if err != nil {
		t.Fatalf("SpectralSimple: %v", err)
	}
	want := []float64{1.5, 2, 2, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestSpectralSimpleRejectsMalformed(t *testing.T) {
	if _, err := SpectralSimple(make([]byte, 12), nil, 1, nil); err == nil {
		t.Fatal("short template accepted")
	}
	if _, err := SpectralSimple(make([]byte, 13), nil, 0, nil); err == nil {
		t.Fatal("zero coefficients accepted")
	}
}
