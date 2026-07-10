package grib

import (
	"math"
	"testing"
	"time"
)

func TestHeaderValidTimeAndScaledMetadata(t *testing.T) {
	ref := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	h := Header{
		ReferenceTime:            ref,
		UnitOfTimeRange:          1,
		ForecastTime:             6,
		ScaleFactorSecondSurface: 2,
		ScaledValueSecondSurface: 150,
		ScaleFactorLowerLimit:    1,
		ScaledValueLowerLimit:    -25,
		ScaleFactorUpperLimit:    1,
		ScaledValueUpperLimit:    40,
	}
	if got, ok := h.ValidTime(); !ok || !got.Equal(ref.Add(6*time.Hour)) {
		t.Errorf("ValidTime = %v ok=%v", got, ok)
	}
	if got := h.SecondSurfaceLevel(); got != 1.5 {
		t.Errorf("SecondSurfaceLevel = %v, want 1.5", got)
	}
	if got := h.ProbabilityLowerLimit(); got != -2.5 {
		t.Errorf("ProbabilityLowerLimit = %v, want -2.5", got)
	}
	if got := h.ProbabilityUpperLimit(); got != 4 {
		t.Errorf("ProbabilityUpperLimit = %v, want 4", got)
	}

	end := ref.Add(12 * time.Hour)
	h.HasEndOfOverallTimeInterval = true
	h.EndOfOverallTimeInterval = end
	if got, ok := h.ValidTime(); !ok || !got.Equal(end) {
		t.Errorf("interval ValidTime = %v ok=%v, want %v", got, ok, end)
	}

	h.ScaledValueSecondSurface = 0xffffffff
	if got := h.SecondSurfaceLevel(); !math.IsNaN(got) {
		t.Errorf("missing second surface = %v, want NaN", got)
	}
}

func TestHeaderValidTimeCalendarUnits(t *testing.T) {
	ref := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
	h := Header{ReferenceTime: ref, UnitOfTimeRange: 3, ForecastTime: 1}
	got, ok := h.ValidTime()
	if !ok || !got.Equal(ref.AddDate(0, 1, 0)) {
		t.Errorf("monthly ValidTime = %v ok=%v", got, ok)
	}
	h.UnitOfTimeRange = 255
	if _, ok := h.ValidTime(); ok {
		t.Error("unknown time unit reported valid")
	}
}
