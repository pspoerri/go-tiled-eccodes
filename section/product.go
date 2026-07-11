package section

import (
	"fmt"
	"time"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// ProductDefinition is a template-aware view of the commonly useful fields
// in GRIB2 Section 4. Missing or inapplicable code-table values are 255.
type ProductDefinition struct {
	TemplateNumber uint16

	ParameterCategory       uint8
	ParameterNumber         uint8
	TypeOfGeneratingProcess uint8
	UnitOfForecastTime      uint8
	ForecastTime            int32

	TypeOfFirstFixedSurface  uint8
	ScaleFactorFirstSurface  int8
	ScaledValueFirstSurface  uint32
	TypeOfSecondFixedSurface uint8
	ScaleFactorSecondSurface int8
	ScaledValueSecondSurface uint32

	DerivedForecast             uint8
	TypeOfEnsembleForecast      uint8
	PerturbationNumber          uint8
	NumberOfForecastsInEnsemble uint8

	ForecastProbabilityNumber  uint8
	TotalForecastProbabilities uint8
	ProbabilityType            uint8
	ScaleFactorLowerLimit      int8
	ScaledValueLowerLimit      int32
	ScaleFactorUpperLimit      int8
	ScaledValueUpperLimit      int32
	PercentileValue            uint8

	EndOfOverallTimeInterval          time.Time
	HasEndOfOverallTimeInterval       bool
	NumberOfTimeRanges                uint8
	NumberMissingInStatisticalProcess uint32
	FirstTimeRange                    TimeRange
	HasTimeRange                      bool
}

// TimeRange is one 12-octet statistical-processing time-range specification.
type TimeRange struct {
	StatisticalProcess uint8
	TypeOfIncrement    uint8
	Unit               uint8
	Length             uint32
	IncrementUnit      uint8
	Increment          uint32
}

type pdtOffsets struct {
	generating    int
	unit          int
	forecast      int
	forecastWidth int
	surface       int
	hasSurface    bool
}

func (s Section4) commonOffsets() (pdtOffsets, bool) {
	switch n := s.TemplateNumber(); {
	case n <= 15, n == 60, n == 61:
		return pdtOffsets{generating: 11, unit: 17, forecast: 18, forecastWidth: 4, surface: 22, hasSurface: true}, true
	case n >= 32 && n <= 34:
		return pdtOffsets{generating: 11, unit: 17, forecast: 18, forecastWidth: 4}, true
	case n >= 40 && n <= 43:
		return pdtOffsets{generating: 13, unit: 19, forecast: 20, forecastWidth: 4, surface: 24, hasSurface: true}, true
	case n == 44:
		// Canonical PDT 4.44 uses a two-octet forecast time and begins
		// its surface descriptors two octets earlier than the legacy form.
		baseLength := len(s.Raw) - 4*int(s.NumCoords())
		if baseLength == 45 {
			return pdtOffsets{generating: 24, unit: 30, forecast: 31, forecastWidth: 2, surface: 33, hasSurface: true}, true
		}
		return pdtOffsets{generating: 24, unit: 30, forecast: 31, forecastWidth: 4, surface: 35, hasSurface: true}, true
	case n >= 45 && n <= 47:
		return pdtOffsets{generating: 24, unit: 30, forecast: 31, forecastWidth: 4, surface: 35, hasSurface: true}, true
	}
	return pdtOffsets{}, false
}

func (s Section4) intervalOffsets() (end, count, missing, firstRange int, ok bool) {
	switch s.TemplateNumber() {
	case 8:
		return 34, 41, 42, 46, true
	case 9:
		return 47, 54, 55, 59, true
	case 10:
		return 35, 42, 43, 47, true
	case 11:
		return 37, 44, 45, 49, true
	case 61:
		return 44, 51, 52, 56, true
	case 12:
		return 36, 43, 44, 48, true
	case 42:
		return 36, 43, 44, 48, true
	case 43:
		return 39, 46, 47, 51, true
	case 46:
		return 47, 54, 55, 59, true
	case 47:
		return 50, 57, 58, 62, true
	}
	return 0, 0, 0, 0, false
}

// ProductDefinition parses the supported product-definition layouts without
// assuming that chemical and aerosol templates share PDT 4.0 byte offsets.
func (s Section4) ProductDefinition() ProductDefinition {
	p := ProductDefinition{
		TemplateNumber:                    s.TemplateNumber(),
		ParameterCategory:                 byteAt(s.Raw, 9, 255),
		ParameterNumber:                   byteAt(s.Raw, 10, 255),
		TypeOfGeneratingProcess:           255,
		UnitOfForecastTime:                0,
		TypeOfFirstFixedSurface:           255,
		TypeOfSecondFixedSurface:          255,
		DerivedForecast:                   255,
		TypeOfEnsembleForecast:            255,
		ForecastProbabilityNumber:         255,
		TotalForecastProbabilities:        255,
		ProbabilityType:                   255,
		ScaleFactorLowerLimit:             -127,
		ScaleFactorUpperLimit:             -127,
		PercentileValue:                   255,
		NumberMissingInStatisticalProcess: 0xffffffff,
		FirstTimeRange: TimeRange{
			StatisticalProcess: 255,
			TypeOfIncrement:    255,
			Unit:               255,
			IncrementUnit:      255,
		},
	}

	if o, ok := s.commonOffsets(); ok {
		p.TypeOfGeneratingProcess = byteAt(s.Raw, o.generating, 255)
		p.UnitOfForecastTime = byteAt(s.Raw, o.unit, 0)
		if o.forecastWidth == 2 {
			p.ForecastTime = int32(u16At(s.Raw, o.forecast, 0))
		} else {
			p.ForecastTime = i32SMAt(s.Raw, o.forecast)
		}
		if o.hasSurface {
			p.TypeOfFirstFixedSurface = byteAt(s.Raw, o.surface, 255)
			p.ScaleFactorFirstSurface = i8SMAt(s.Raw, o.surface+1)
			p.ScaledValueFirstSurface = u32At(s.Raw, o.surface+2, 0)
			p.TypeOfSecondFixedSurface = byteAt(s.Raw, o.surface+6, 255)
			p.ScaleFactorSecondSurface = i8SMAt(s.Raw, o.surface+7)
			p.ScaledValueSecondSurface = u32At(s.Raw, o.surface+8, 0)
		}
	}

	switch p.TemplateNumber {
	case 2, 3, 4, 12, 13, 14:
		p.DerivedForecast = byteAt(s.Raw, 34, 255)
	case 5, 9:
		p.ForecastProbabilityNumber = byteAt(s.Raw, 34, 255)
		p.TotalForecastProbabilities = byteAt(s.Raw, 35, 255)
		p.ProbabilityType = byteAt(s.Raw, 36, 255)
		p.ScaleFactorLowerLimit = i8SMAt(s.Raw, 37)
		p.ScaledValueLowerLimit = i32SMAt(s.Raw, 38)
		p.ScaleFactorUpperLimit = i8SMAt(s.Raw, 42)
		p.ScaledValueUpperLimit = i32SMAt(s.Raw, 43)
	case 6, 10:
		p.PercentileValue = byteAt(s.Raw, 34, 255)
	}

	if off, ok := s.ensembleOffset(); ok {
		p.TypeOfEnsembleForecast = byteAt(s.Raw, off, 255)
		p.PerturbationNumber = byteAt(s.Raw, off+1, 0)
		p.NumberOfForecastsInEnsemble = byteAt(s.Raw, off+2, 0)
	}

	if end, count, missing, first, ok := s.intervalOffsets(); ok {
		p.EndOfOverallTimeInterval, p.HasEndOfOverallTimeInterval = timeAt(s.Raw, end)
		p.NumberOfTimeRanges = byteAt(s.Raw, count, 0)
		p.NumberMissingInStatisticalProcess = u32At(s.Raw, missing, 0xffffffff)
		if p.NumberOfTimeRanges > 0 {
			p.FirstTimeRange, p.HasTimeRange = timeRangeAt(s.Raw, first)
		}
	}
	return p
}

// TimeRanges returns every statistical time-range specification. dst is reused
// when it has enough capacity. ok is false for truncated or non-interval PDTs.
func (s Section4) TimeRanges(dst []TimeRange) ([]TimeRange, bool) {
	_, countOff, _, first, ok := s.intervalOffsets()
	if !ok {
		return nil, false
	}
	n := int(byteAt(s.Raw, countOff, 0))
	if n == 0 || first+n*12 > len(s.Raw) {
		return nil, false
	}
	if cap(dst) < n {
		dst = make([]TimeRange, n)
	} else {
		dst = dst[:n]
	}
	for i := range dst {
		var valid bool
		dst[i], valid = timeRangeAt(s.Raw, first+i*12)
		if !valid {
			return nil, false
		}
	}
	return dst, true
}

// CoordinateValues decodes the optional IEEE-32 vertical-coordinate values
// appended to Section 4 (typically hybrid model-level A/B coefficients).
func (s Section4) CoordinateValues(dst []float64) ([]float64, error) {
	n := int(s.NumCoords())
	if n == 0 {
		return dst[:0], nil
	}
	bytesNeeded := n * 4
	if bytesNeeded/4 != n || len(s.Raw) < 9+bytesNeeded {
		return nil, fmt.Errorf("section 4: truncated coordinate values")
	}
	start := len(s.Raw) - bytesNeeded
	if cap(dst) < n {
		dst = make([]float64, n)
	} else {
		dst = dst[:n]
	}
	for i := range dst {
		dst[i] = float64(bswap.F32(s.Raw, start+i*4))
	}
	return dst, nil
}

func byteAt(b []byte, off int, missing uint8) uint8 {
	if off < 0 || off >= len(b) {
		return missing
	}
	return b[off]
}

func i8SMAt(b []byte, off int) int8 {
	if off < 0 || off >= len(b) {
		return 0
	}
	return bswap.I8SM(b, off)
}

func i32SMAt(b []byte, off int) int32 {
	if off < 0 || off+4 > len(b) {
		return 0
	}
	return bswap.I32SM(b, off)
}

func u16At(b []byte, off int, missing uint16) uint16 {
	if off < 0 || off+2 > len(b) {
		return missing
	}
	return bswap.U16(b, off)
}

func u32At(b []byte, off int, missing uint32) uint32 {
	if off < 0 || off+4 > len(b) {
		return missing
	}
	return bswap.U32(b, off)
}

func timeAt(b []byte, off int) (time.Time, bool) {
	if off < 0 || off+7 > len(b) {
		return time.Time{}, false
	}
	year := int(bswap.U16(b, off))
	month, day := int(b[off+2]), int(b[off+3])
	hour, minute, second := int(b[off+4]), int(b[off+5]), int(b[off+6])
	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 || second > 60 {
		return time.Time{}, false
	}
	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return time.Time{}, false
	}
	return t, true
}

func timeRangeAt(b []byte, off int) (TimeRange, bool) {
	if off < 0 || off+12 > len(b) {
		return TimeRange{}, false
	}
	return TimeRange{
		StatisticalProcess: b[off],
		TypeOfIncrement:    b[off+1],
		Unit:               b[off+2],
		Length:             bswap.U32(b, off+3),
		IncrementUnit:      b[off+7],
		Increment:          bswap.U32(b, off+8),
	}, true
}
