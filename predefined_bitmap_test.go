package grib_test

import (
	"errors"
	"math"
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
	"github.com/pspoerri/go-tiled-eccodes/writer"
)

func TestPredefinedBitmapRegistration(t *testing.T) {
	g := writer.NewLatLon(4, 2, 1, 0, 1, 1)
	want := []float64{1, math.NaN(), 3, 4, 5, 6, math.NaN(), 8}
	data, err := writer.Single(minimalField(g, want))
	if err != nil {
		t.Fatalf("Single: %v", err)
	}

	indexed, err := grib.FromBytes(data)
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}
	bitmap := append([]byte(nil), indexed.Messages()[0].S6.Bits()...)
	indexed.Messages()[0].S6.Raw[5] = 7
	_ = indexed.Close()

	missing, err := grib.FromBytes(data)
	if err != nil {
		t.Fatalf("FromBytes missing: %v", err)
	}
	if _, err := missing.Messages()[0].DecodeFloat64(nil); !errors.Is(err, grib.ErrPredefinedBitmap) {
		t.Fatalf("decode without registration = %v, want ErrPredefinedBitmap", err)
	}
	_ = missing.Close()

	file, err := grib.FromBytes(data)
	if err != nil {
		t.Fatalf("FromBytes registered: %v", err)
	}
	defer file.Close()
	if _, err := file.SetPredefinedBitmap(7, nil); !errors.Is(err, grib.ErrTruncated) {
		t.Fatalf("short bitmap = %v, want ErrTruncated", err)
	}
	matched, err := file.SetPredefinedBitmap(7, bitmap)
	if err != nil {
		t.Fatalf("SetPredefinedBitmap: %v", err)
	}
	if matched != 1 {
		t.Fatalf("matched = %d, want 1", matched)
	}
	got, err := file.Messages()[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("DecodeFloat64: %v", err)
	}
	for i := range want {
		if math.IsNaN(want[i]) {
			if !math.IsNaN(got[i]) {
				t.Errorf("got[%d] = %v, want NaN", i, got[i])
			}
		} else if math.Abs(got[i]-want[i]) > 0.01 {
			t.Errorf("got[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if err := file.Messages()[0].SetPredefinedBitmap(bitmap); !errors.Is(err, grib.ErrDecodeStarted) {
		t.Fatalf("late registration = %v, want ErrDecodeStarted", err)
	}
}
