package section

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func TestChemicalIntervalProductDefinition(t *testing.T) {
	raw := make([]byte, 68)
	binary.BigEndian.PutUint16(raw[5:], 2)
	binary.BigEndian.PutUint16(raw[7:], 42)
	raw[9], raw[10] = 20, 1
	raw[13] = 2
	raw[19] = 1
	binary.BigEndian.PutUint32(raw[20:], 6)
	raw[24], raw[25] = 100, 0
	binary.BigEndian.PutUint32(raw[26:], 50000)
	binary.BigEndian.PutUint16(raw[36:], 2026)
	raw[38], raw[39], raw[40] = 7, 10, 12
	raw[41], raw[42] = 30, 0
	raw[43] = 1
	binary.BigEndian.PutUint32(raw[44:], 3)
	raw[48], raw[49], raw[50] = 1, 2, 1
	binary.BigEndian.PutUint32(raw[51:], 6)
	raw[55] = 1
	binary.BigEndian.PutUint32(raw[56:], 1)
	binary.BigEndian.PutUint32(raw[60:], math.Float32bits(1.25))
	binary.BigEndian.PutUint32(raw[64:], math.Float32bits(2.5))

	s := Section4{Raw: raw}
	p := s.ProductDefinition()
	if p.TypeOfGeneratingProcess != 2 || p.ForecastTime != 6 || p.TypeOfFirstFixedSurface != 100 {
		t.Fatalf("shifted chemical fields parsed incorrectly: %+v", p)
	}
	wantEnd := time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC)
	if !p.HasEndOfOverallTimeInterval || !p.EndOfOverallTimeInterval.Equal(wantEnd) {
		t.Errorf("end time = %v, want %v", p.EndOfOverallTimeInterval, wantEnd)
	}
	if !p.HasTimeRange || p.FirstTimeRange.StatisticalProcess != 1 ||
		p.FirstTimeRange.Unit != 1 || p.FirstTimeRange.Length != 6 {
		t.Errorf("time range = %+v", p.FirstTimeRange)
	}
	all, ok := s.TimeRanges(nil)
	if !ok || len(all) != 1 || all[0] != p.FirstTimeRange {
		t.Errorf("TimeRanges = %+v ok=%v", all, ok)
	}
	coords, err := s.CoordinateValues(nil)
	if err != nil || len(coords) != 2 || coords[0] != 1.25 || coords[1] != 2.5 {
		t.Errorf("CoordinateValues = %v err=%v", coords, err)
	}
}

func TestAerosolOffsetsAndProbability(t *testing.T) {
	raw := make([]byte, 72)
	binary.BigEndian.PutUint16(raw[7:], 46)
	raw[24], raw[30] = 4, 1
	binary.BigEndian.PutUint32(raw[31:], 12)
	raw[35] = 103
	binary.BigEndian.PutUint32(raw[37:], 2)
	binary.BigEndian.PutUint16(raw[47:], 2026)
	raw[49], raw[50], raw[51] = 7, 10, 18
	raw[52], raw[53] = 0, 0
	raw[54] = 1
	raw[59], raw[60], raw[61] = 2, 1, 1
	binary.BigEndian.PutUint32(raw[62:], 12)
	p := (Section4{Raw: raw}).ProductDefinition()
	if p.TypeOfGeneratingProcess != 4 || p.ForecastTime != 12 || p.TypeOfFirstFixedSurface != 103 {
		t.Errorf("aerosol offsets parsed incorrectly: %+v", p)
	}
	if !p.HasTimeRange || p.FirstTimeRange.StatisticalProcess != 2 {
		t.Errorf("aerosol time range = %+v", p.FirstTimeRange)
	}

	prob := make([]byte, 60)
	binary.BigEndian.PutUint16(prob[7:], 9)
	prob[34], prob[35], prob[36] = 1, 5, 2
	prob[37] = 1
	binary.BigEndian.PutUint32(prob[38:], 10)
	prob[42] = 1
	binary.BigEndian.PutUint32(prob[43:], 20)
	pp := (Section4{Raw: prob}).ProductDefinition()
	if pp.ProbabilityType != 2 || pp.ScaledValueLowerLimit != 10 || pp.ScaledValueUpperLimit != 20 {
		t.Errorf("probability fields = %+v", pp)
	}
}

func TestTemplate61IntervalOffsets(t *testing.T) {
	raw := make([]byte, 68)
	binary.BigEndian.PutUint16(raw[7:], 61)
	raw[17] = 1
	binary.BigEndian.PutUint32(raw[18:], 6)

	// Model-version timestamp inserted by PDT 4.61.
	binary.BigEndian.PutUint16(raw[37:], 2025)
	raw[39], raw[40], raw[41] = 6, 1, 0
	raw[42], raw[43] = 0, 0

	// End of the overall statistical interval.
	binary.BigEndian.PutUint16(raw[44:], 2026)
	raw[46], raw[47], raw[48] = 7, 10, 18
	raw[49], raw[50] = 30, 0
	raw[51] = 1
	binary.BigEndian.PutUint32(raw[52:], 3)
	raw[56], raw[57], raw[58] = 2, 1, 1
	binary.BigEndian.PutUint32(raw[59:], 12)
	raw[63] = 1
	binary.BigEndian.PutUint32(raw[64:], 6)

	s := Section4{Raw: raw}
	p := s.ProductDefinition()
	wantEnd := time.Date(2026, 7, 10, 18, 30, 0, 0, time.UTC)
	if !p.HasEndOfOverallTimeInterval || !p.EndOfOverallTimeInterval.Equal(wantEnd) {
		t.Errorf("end time = %v, want %v", p.EndOfOverallTimeInterval, wantEnd)
	}
	if p.NumberOfTimeRanges != 1 || p.NumberMissingInStatisticalProcess != 3 {
		t.Errorf("interval metadata = ranges %d, missing %d",
			p.NumberOfTimeRanges, p.NumberMissingInStatisticalProcess)
	}
	if !p.HasTimeRange || p.FirstTimeRange.StatisticalProcess != 2 ||
		p.FirstTimeRange.Unit != 1 || p.FirstTimeRange.Length != 12 ||
		p.FirstTimeRange.IncrementUnit != 1 || p.FirstTimeRange.Increment != 6 {
		t.Errorf("first time range = %+v", p.FirstTimeRange)
	}
	all, ok := s.TimeRanges(nil)
	if !ok || len(all) != 1 || all[0] != p.FirstTimeRange {
		t.Errorf("TimeRanges = %+v ok=%v", all, ok)
	}
}

func TestAerosolTemplate44CanonicalAndLegacy(t *testing.T) {
	t.Run("canonical with coordinate value", func(t *testing.T) {
		raw := make([]byte, 49)
		binary.BigEndian.PutUint32(raw, uint32(len(raw)))
		binary.BigEndian.PutUint16(raw[5:], 1)
		binary.BigEndian.PutUint16(raw[7:], 44)
		raw[24], raw[30] = 4, 1
		binary.BigEndian.PutUint16(raw[31:], 12)
		raw[33] = 103
		binary.BigEndian.PutUint32(raw[35:], 2)
		binary.BigEndian.PutUint32(raw[45:], math.Float32bits(1.25))

		s := Section4{Raw: raw}
		p := s.ProductDefinition()
		if p.TypeOfGeneratingProcess != 4 || p.ForecastTime != 12 ||
			p.TypeOfFirstFixedSurface != 103 || p.ScaledValueFirstSurface != 2 {
			t.Errorf("canonical PDT 4.44 fields = %+v", p)
		}
		coords, err := s.CoordinateValues(nil)
		if err != nil || len(coords) != 1 || coords[0] != 1.25 {
			t.Errorf("CoordinateValues = %v err=%v", coords, err)
		}
	})

	t.Run("legacy", func(t *testing.T) {
		raw := make([]byte, 47)
		binary.BigEndian.PutUint32(raw, uint32(len(raw)))
		binary.BigEndian.PutUint16(raw[7:], 44)
		raw[24], raw[30] = 4, 1
		binary.BigEndian.PutUint32(raw[31:], 12)
		raw[35] = 103
		binary.BigEndian.PutUint32(raw[37:], 2)

		p := (Section4{Raw: raw}).ProductDefinition()
		if p.TypeOfGeneratingProcess != 4 || p.ForecastTime != 12 ||
			p.TypeOfFirstFixedSurface != 103 || p.ScaledValueFirstSurface != 2 {
			t.Errorf("legacy PDT 4.44 fields = %+v", p)
		}
	})
}

func TestCoordinateValuesRejectTruncation(t *testing.T) {
	raw := make([]byte, 12)
	binary.BigEndian.PutUint16(raw[5:], 2)
	if _, err := (Section4{Raw: raw}).CoordinateValues(nil); err == nil {
		t.Fatal("truncated coordinate values returned nil error")
	}
}
