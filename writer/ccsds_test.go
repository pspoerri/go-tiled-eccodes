//go:build cgo

package writer_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/pspoerri/go-tiled-eccodes/writer"
)

// findSection5Template locates Section 5 in a single-message GRIB2 byte
// stream and returns its template body (the bytes after the 11-byte prefix).
// Mirrors the section walk in the decoder/index.
func findSection5Template(t *testing.T, data []byte) (tmplNum uint16, body []byte) {
	t.Helper()
	if len(data) < 16 || string(data[0:4]) != "GRIB" {
		t.Fatalf("not a GRIB message")
	}
	p := 16 // skip Section 0
	for p+5 <= len(data) {
		if string(data[p:p+4]) == "7777" {
			break
		}
		secLen := int(binary.BigEndian.Uint32(data[p:]))
		if secLen <= 0 || p+secLen > len(data) {
			t.Fatalf("bad section length %d at %d", secLen, p)
		}
		if data[p+4] == 5 {
			tmplNum = binary.BigEndian.Uint16(data[p+9:])
			return tmplNum, data[p+11 : p+secLen]
		}
		p += secLen
	}
	t.Fatalf("no Section 5 found")
	return 0, nil
}

// TestCCSDSRoundTrip writes a smoothly varying field as CCSDS (template 5.42)
// and reads it back through the decoder. 16-bit packing over the field span
// gives a tiny quantization error, so values round-trip within ~0.01.
func TestCCSDSRoundTrip(t *testing.T) {
	g := writer.NewLatLon(16, 16, 51, 6, 0.1, 0.1)
	vals := linearField(16, 16, 273)
	f := baseField(g, vals)
	f.Packing = writer.PackingCCSDS
	f.NumBits = 16

	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	file, msgs := roundTrip(t, data)
	defer file.Close()

	if got := msgs[0].Header().DataTemplate; got != 42 {
		t.Errorf("DataTemplate = %d, want 42", got)
	}
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("DecodeFloat64: %v", err)
	}
	assertValuesClose(t, got, vals, 0.01)
}

// TestCCSDSSection5Defaults asserts the writer emits DWD's CCSDS parameters
// (flags=14, blockSize=32, rsi=128) by default, and a 14-byte template body.
// These are the exact values observed on live DWD ICON-D2 products.
func TestCCSDSSection5Defaults(t *testing.T) {
	g := writer.NewLatLon(8, 8, 51, 6, 0.5, 0.5)
	vals := linearField(8, 8, 280)
	f := baseField(g, vals)
	f.Packing = writer.PackingCCSDS
	f.NumBits = 16

	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	tmplNum, body := findSection5Template(t, data)
	if tmplNum != 42 {
		t.Fatalf("template number = %d, want 42", tmplNum)
	}
	if len(body) != 14 {
		t.Fatalf("template body = %d bytes, want 14", len(body))
	}
	if body[8] != 16 {
		t.Errorf("nbits = %d, want 16", body[8])
	}
	if body[9] != 0 {
		t.Errorf("type of original field values = %d, want 0", body[9])
	}
	if body[10] != 14 {
		t.Errorf("flags = %d, want 14", body[10])
	}
	if body[11] != 32 {
		t.Errorf("blockSize = %d, want 32", body[11])
	}
	if rsi := binary.BigEndian.Uint16(body[12:]); rsi != 128 {
		t.Errorf("rsi = %d, want 128", rsi)
	}
}

// TestCCSDSConstantField exercises the nbits=0 fast path: a constant field has
// no packed samples, so Section 7 is empty and the decoder reconstructs every
// value from the reference alone (no libaec call on either side).
func TestCCSDSConstantField(t *testing.T) {
	g := writer.NewLatLon(4, 3, 51, 6, 1, 1)
	vals := make([]float64, 12)
	for i := range vals {
		vals[i] = 273.15
	}
	f := baseField(g, vals)
	f.Packing = writer.PackingCCSDS
	f.NumBits = 0 // constant-field fast path

	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	_, body := findSection5Template(t, data)
	if body[8] != 0 {
		t.Errorf("nbits = %d, want 0 (constant field)", body[8])
	}
	file, msgs := roundTrip(t, data)
	defer file.Close()
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("DecodeFloat64: %v", err)
	}
	for i, v := range got {
		if math.Abs(v-273.15) > 1e-3 {
			t.Errorf("vals[%d] = %v, want 273.15", i, v)
		}
	}
}

// TestCCSDSWithMissing round-trips a field containing NaNs. The Section 6
// bitmap path is shared with the other packings, but exercise it under CCSDS
// to confirm only valid samples are fed to the AEC encoder.
func TestCCSDSWithMissing(t *testing.T) {
	g := writer.NewLatLon(4, 3, 51, 6, 1, 1)
	vals := []float64{
		1, 2, math.NaN(), 4,
		5, math.NaN(), 7, 8,
		9, 10, 11, math.NaN(),
	}
	f := baseField(g, vals)
	f.Packing = writer.PackingCCSDS
	f.NumBits = 16

	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	file, msgs := roundTrip(t, data)
	defer file.Close()
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("DecodeFloat64: %v", err)
	}
	assertValuesClose(t, got, vals, 0.01)
}

// TestCCSDSCustomParams confirms caller-supplied AEC tuning is honoured and
// still round-trips (the decoder reads the parameters back out of Section 5).
func TestCCSDSCustomParams(t *testing.T) {
	g := writer.NewLatLon(12, 10, 51, 6, 0.2, 0.2)
	vals := linearField(12, 10, 290)
	f := baseField(g, vals)
	f.Packing = writer.PackingCCSDS
	f.NumBits = 16
	f.CCSDSBlockSize = 16
	f.CCSDSRSI = 64

	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	_, body := findSection5Template(t, data)
	if body[11] != 16 {
		t.Errorf("blockSize = %d, want 16", body[11])
	}
	if rsi := binary.BigEndian.Uint16(body[12:]); rsi != 64 {
		t.Errorf("rsi = %d, want 64", rsi)
	}
	file, msgs := roundTrip(t, data)
	defer file.Close()
	got, err := msgs[0].DecodeFloat64(nil)
	if err != nil {
		t.Fatalf("DecodeFloat64: %v", err)
	}
	assertValuesClose(t, got, vals, 0.01)
}
