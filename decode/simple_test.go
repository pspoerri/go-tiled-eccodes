package decode

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestSimpleRejectsMalformedInput(t *testing.T) {
	if _, err := Simple(nil, nil, 1, nil); err == nil {
		t.Fatal("short template returned nil error")
	}

	template := make([]byte, 10)
	template[8] = 8
	if _, err := Simple(template, nil, 1, nil); err == nil {
		t.Fatal("short payload returned nil error")
	}

	template[8] = 33
	if _, err := Simple(template, make([]byte, 8), 1, nil); err == nil {
		t.Fatal("bits-per-value > 32 returned nil error")
	}
}

func TestSimpleAndBitmap(t *testing.T) {
	template := make([]byte, 10)
	template[8] = 8
	got, err := Simple(template, []byte{1, 2, 3}, 3, nil)
	if err != nil {
		t.Fatalf("Simple: %v", err)
	}
	for i, want := range []float64{1, 2, 3} {
		if got[i] != want {
			t.Errorf("value[%d] = %v, want %v", i, got[i], want)
		}
	}

	full := ApplyBitmap([]byte{0xa0}, got, 4, nil)
	if full[0] != 1 || !math.IsNaN(full[1]) || full[2] != 2 || !math.IsNaN(full[3]) {
		t.Errorf("ApplyBitmap = %v", full)
	}
}

func TestPNGRejectsShortTemplate(t *testing.T) {
	if _, err := PNG(nil, nil, 1, nil); err == nil {
		t.Fatal("short PNG template returned nil error")
	}
}

func TestLogPreprocessedConstant(t *testing.T) {
	template := make([]byte, 13)
	b := float32(2)
	z := float32(math.Log(12))
	binary.BigEndian.PutUint32(template[0:], math.Float32bits(z))
	template[8] = 0
	binary.BigEndian.PutUint32(template[9:], math.Float32bits(b))
	got, err := LogPreprocessed(template, nil, 3, nil)
	if err != nil {
		t.Fatalf("LogPreprocessed: %v", err)
	}
	for i, value := range got {
		if math.Abs(value-10) > 1e-5 {
			t.Errorf("value[%d] = %v, want 10", i, value)
		}
	}
	if _, err := LogPreprocessed(template[:12], nil, 1, nil); err == nil {
		t.Error("short template returned nil error")
	}
}
