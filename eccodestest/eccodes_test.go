//go:build eccodes && cgo

package eccodestest

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pspoerri/go-tiled-eccodes/writer"
)

func readOrFail(t *testing.T, path string) []EccodesMessage {
	t.Helper()
	msgs, err := ReadEccodes(path)
	if err != nil {
		t.Fatalf("ReadEccodes: %v", err)
	}
	return msgs
}

func TestEccodesValidatesLatLon(t *testing.T) {
	g := writer.NewLatLon(8, 5, 51, 6, 0.5, 0.4)
	vals := genValues(g.Ni, g.Nj, 273)
	f := writer.Field{
		Discipline:              0,
		Centre:                  78,
		ReferenceTime:           time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC),
		ParameterCategory:       0,
		ParameterNumber:         0,
		UnitOfTimeRange:         1,
		ForecastTime:            6,
		TypeOfFirstFixedSurface: 103,
		ScaledValueFirstSurface: 2,
		Grid:                    g,
		Values:                  vals,
		NumBits:                 16,
	}
	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	path := writeTemp(t, data)

	msgs := readOrFail(t, path)
	if len(msgs) != 1 {
		t.Fatalf("eccodes saw %d messages, want 1", len(msgs))
	}
	m := msgs[0]
	if m.Edition != 2 {
		t.Errorf("edition = %d, want 2", m.Edition)
	}
	if m.GridDefNumber != 0 {
		t.Errorf("gridDefinitionTemplateNumber = %d, want 0", m.GridDefNumber)
	}
	if m.Ni != int64(g.Ni) || m.Nj != int64(g.Nj) {
		t.Errorf("dims = %dx%d, want %dx%d", m.Ni, m.Nj, g.Ni, g.Nj)
	}
	if m.Year != 2026 || m.Month != 5 || m.Day != 8 || m.Hour != 12 {
		t.Errorf("ref time = %d-%02d-%02d %02d, want 2026-05-08 12", m.Year, m.Month, m.Day, m.Hour)
	}
	if m.ForecastTime != 6 {
		t.Errorf("forecastTime = %d, want 6", m.ForecastTime)
	}
	if m.BitsPerValue != 16 {
		t.Errorf("bitsPerValue = %d, want 16", m.BitsPerValue)
	}
	if m.NumValues != int64(len(vals)) {
		t.Fatalf("numberOfDataPoints = %d, want %d", m.NumValues, len(vals))
	}
	for i, v := range vals {
		if math.Abs(m.Values[i]-v) > 0.01 {
			t.Fatalf("values[%d] = %v, want %v (eccodes disagreed with writer)", i, m.Values[i], v)
		}
	}
}

func TestEccodesValidatesMultiMessage(t *testing.T) {
	g := writer.NewLatLon(6, 4, 50, 8, 1, 1)
	t0 := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
	var fields []writer.Field
	for h := 0; h < 3; h++ {
		f := writer.Field{
			Discipline:              0,
			Centre:                  78,
			ReferenceTime:           t0.Add(time.Duration(h) * time.Hour),
			ParameterCategory:       0,
			ParameterNumber:         0,
			UnitOfTimeRange:         1,
			ForecastTime:            int32(h),
			TypeOfFirstFixedSurface: 103,
			ScaledValueFirstSurface: 2,
			Grid:                    g,
			Values:                  genValues(g.Ni, g.Nj, 270+float64(h)),
			NumBits:                 16,
		}
		fields = append(fields, f)
	}
	data, err := writer.Series(fields)
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	path := writeTemp(t, data)

	msgs := readOrFail(t, path)
	if len(msgs) != 3 {
		t.Fatalf("eccodes saw %d messages, want 3", len(msgs))
	}
	for i, m := range msgs {
		if m.Hour != int64(i) {
			t.Errorf("msg %d: hour = %d, want %d", i, m.Hour, i)
		}
		if m.ForecastTime != int64(i) {
			t.Errorf("msg %d: forecastTime = %d, want %d", i, m.ForecastTime, i)
		}
	}
}

func TestEccodesValidatesProjections(t *testing.T) {
	cases := []struct {
		name   string
		grid   writer.Grid
		gdtNum int64
	}{
		{
			name:   "rotated_latlon_iconch",
			grid:   writer.NewRotatedLatLon(20, 15, 2, -2, 0.05, 0.05, -43, 10),
			gdtNum: 1,
		},
		{
			name:   "mercator",
			grid:   writer.NewMercator(20, 15, 60, -10, 30, 30, 45, 15000, 15000),
			gdtNum: 10,
		},
		{
			name:   "polar_stereographic",
			grid:   writer.NewPolar(25, 25, 60, -135, 60, 250, 25000, 25000, true),
			gdtNum: 20,
		},
		{
			name:   "lambert_conformal",
			grid:   writer.NewLambert(30, 25, 21.138, 237.28, 38.5, 262.5, 3000, 3000, 38.5, 38.5),
			gdtNum: 30,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ni, nj := tc.grid.NaturalSize()
			vals := genValues(ni, nj, 280)
			f := writer.Field{
				Discipline:              0,
				Centre:                  78,
				ReferenceTime:           time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC),
				ParameterCategory:       0,
				ParameterNumber:         0,
				UnitOfTimeRange:         1,
				ForecastTime:            0,
				TypeOfFirstFixedSurface: 103,
				ScaledValueFirstSurface: 2,
				Grid:                    tc.grid,
				Values:                  vals,
				NumBits:                 16,
			}
			data, err := writer.Single(f)
			if err != nil {
				t.Fatalf("Single: %v", err)
			}
			path := writeTemp(t, data)

			msgs := readOrFail(t, path)
			if len(msgs) != 1 {
				t.Fatalf("eccodes saw %d messages, want 1", len(msgs))
			}
			m := msgs[0]
			if m.GridDefNumber != tc.gdtNum {
				t.Errorf("gridDefinitionTemplateNumber = %d, want %d", m.GridDefNumber, tc.gdtNum)
			}
			if m.NumValues != int64(ni*nj) {
				t.Errorf("numberOfDataPoints = %d, want %d", m.NumValues, ni*nj)
			}
			for i, v := range vals {
				if math.Abs(m.Values[i]-v) > 0.05 {
					t.Fatalf("values[%d] = %v, want %v", i, m.Values[i], v)
				}
			}
		})
	}
}

func TestEccodesValidatesBitmap(t *testing.T) {
	g := writer.NewLatLon(4, 3, 51, 6, 1, 1)
	vals := []float64{
		1, 2, math.NaN(), 4,
		5, math.NaN(), 7, 8,
		9, 10, 11, math.NaN(),
	}
	f := writer.Field{
		Discipline:              0,
		Centre:                  78,
		ReferenceTime:           time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC),
		ParameterCategory:       0,
		ParameterNumber:         0,
		UnitOfTimeRange:         1,
		TypeOfFirstFixedSurface: 103,
		Grid:                    g,
		Values:                  vals,
		NumBits:                 16,
	}
	data, err := writer.Single(f)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	path := writeTemp(t, data)

	msgs := readOrFail(t, path)
	if len(msgs) != 1 {
		t.Fatalf("eccodes saw %d messages, want 1", len(msgs))
	}
	got := msgs[0].Values
	// libeccodes returns the bitmap-applied values where missing points carry
	// the encoded missing value (typically 9.999e20 by default). We just
	// check that the non-NaN positions match — exact missing handling is a
	// libeccodes config knob and not the writer's concern.
	for i, v := range vals {
		if math.IsNaN(v) {
			continue
		}
		if math.Abs(got[i]-v) > 0.01 {
			t.Fatalf("values[%d] = %v, want %v", i, got[i], v)
		}
	}
}

// --- helpers ------------------------------------------------------------

func writeTemp(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "synthetic.grib2")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return p
}

func genValues(ni, nj int, base float64) []float64 {
	out := make([]float64, ni*nj)
	for j := 0; j < nj; j++ {
		for i := 0; i < ni; i++ {
			out[j*ni+i] = base + float64(j)*0.1 + float64(i)*0.01
		}
	}
	return out
}
