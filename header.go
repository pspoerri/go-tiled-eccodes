package grib

import (
	"math"
	"time"

	"github.com/pspoerri/go-tiled-eccodes/grid"
	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// Header is the lightweight, pre-decoded summary for a Message. Field values
// are extracted on demand from the underlying section bytes; constructing a
// Header is cheap (a few uint reads) and never allocates.
type Header struct {
	Discipline       uint8
	Centre           uint16
	SubCentre        uint16
	ReferenceTime    time.Time
	ProductionStatus uint8
	GridTemplate     uint16
	ProductTemplate  uint16
	DataTemplate     uint16
	Ni, Nj           int
	NumDataPoints    int

	ParameterCategory uint8
	ParameterNumber   uint8
	ForecastTime      int32
	UnitOfTimeRange   uint8

	TypeOfFirstFixedSurface uint8
	ScaleFactorFirstSurface int8
	ScaledValueFirstSurface uint32

	TypeOfGeneratingProcess uint8

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

	EndOfOverallTimeInterval             time.Time
	HasEndOfOverallTimeInterval          bool
	NumberOfTimeRanges                   uint8
	NumberMissingInStatisticalProcess    uint32
	TypeOfStatisticalProcessing          uint8
	TypeOfTimeIncrement                  uint8
	UnitOfTimeRangeForStatisticalProcess uint8
	LengthOfTimeRange                    uint32
	UnitOfTimeIncrement                  uint8
	TimeIncrement                        uint32

	NumCoordinates uint16
}

// Header returns a snapshot of the message's identifying metadata.
func (m *Message) Header() Header {
	p := m.S4.ProductDefinition()
	h := Header{
		Discipline:                           m.S0.Discipline(),
		Centre:                               m.S1.Centre(),
		SubCentre:                            m.S1.SubCentre(),
		ReferenceTime:                        m.S1.ReferenceTime(),
		ProductionStatus:                     m.S1.ProductionStatus(),
		GridTemplate:                         m.S3.TemplateNumber(),
		ProductTemplate:                      p.TemplateNumber,
		DataTemplate:                         m.S5.TemplateNumber(),
		NumDataPoints:                        int(m.S3.NumDataPoints()),
		ParameterCategory:                    p.ParameterCategory,
		ParameterNumber:                      p.ParameterNumber,
		TypeOfGeneratingProcess:              p.TypeOfGeneratingProcess,
		ForecastTime:                         p.ForecastTime,
		UnitOfTimeRange:                      p.UnitOfForecastTime,
		TypeOfFirstFixedSurface:              p.TypeOfFirstFixedSurface,
		ScaleFactorFirstSurface:              p.ScaleFactorFirstSurface,
		ScaledValueFirstSurface:              p.ScaledValueFirstSurface,
		TypeOfSecondFixedSurface:             p.TypeOfSecondFixedSurface,
		ScaleFactorSecondSurface:             p.ScaleFactorSecondSurface,
		ScaledValueSecondSurface:             p.ScaledValueSecondSurface,
		DerivedForecast:                      p.DerivedForecast,
		TypeOfEnsembleForecast:               p.TypeOfEnsembleForecast,
		PerturbationNumber:                   p.PerturbationNumber,
		NumberOfForecastsInEnsemble:          p.NumberOfForecastsInEnsemble,
		ForecastProbabilityNumber:            p.ForecastProbabilityNumber,
		TotalForecastProbabilities:           p.TotalForecastProbabilities,
		ProbabilityType:                      p.ProbabilityType,
		ScaleFactorLowerLimit:                p.ScaleFactorLowerLimit,
		ScaledValueLowerLimit:                p.ScaledValueLowerLimit,
		ScaleFactorUpperLimit:                p.ScaleFactorUpperLimit,
		ScaledValueUpperLimit:                p.ScaledValueUpperLimit,
		PercentileValue:                      p.PercentileValue,
		EndOfOverallTimeInterval:             p.EndOfOverallTimeInterval,
		HasEndOfOverallTimeInterval:          p.HasEndOfOverallTimeInterval,
		NumberOfTimeRanges:                   p.NumberOfTimeRanges,
		NumberMissingInStatisticalProcess:    p.NumberMissingInStatisticalProcess,
		TypeOfStatisticalProcessing:          p.FirstTimeRange.StatisticalProcess,
		TypeOfTimeIncrement:                  p.FirstTimeRange.TypeOfIncrement,
		UnitOfTimeRangeForStatisticalProcess: p.FirstTimeRange.Unit,
		LengthOfTimeRange:                    p.FirstTimeRange.Length,
		UnitOfTimeIncrement:                  p.FirstTimeRange.IncrementUnit,
		TimeIncrement:                        p.FirstTimeRange.Increment,
		NumCoordinates:                       m.S4.NumCoords(),
	}
	if g, err := m.Grid(); err == nil {
		h.Ni, h.Nj = g.Size()
	}
	return h
}

// SurfaceLevel returns the first-fixed-surface value as a float64
// (scaled_value × 10^-scale_factor). Returns NaN if scale_factor or
// scaled_value indicate "missing" (per WMO convention, all bits set).
func (h Header) SurfaceLevel() float64 {
	if h.ScaleFactorFirstSurface == -127 || h.ScaledValueFirstSurface == 0xffffffff {
		return math.NaN()
	}
	return float64(h.ScaledValueFirstSurface) * math.Pow10(-int(h.ScaleFactorFirstSurface))
}

func (h Header) SecondSurfaceLevel() float64 {
	return scaledUnsigned(h.ScaleFactorSecondSurface, h.ScaledValueSecondSurface)
}

func (h Header) ProbabilityLowerLimit() float64 {
	return scaledSigned(h.ScaleFactorLowerLimit, h.ScaledValueLowerLimit)
}

func (h Header) ProbabilityUpperLimit() float64 {
	return scaledSigned(h.ScaleFactorUpperLimit, h.ScaledValueUpperLimit)
}

// ValidTime returns the interval end for statistically processed products and
// reference time plus forecast offset for instantaneous products.
func (h Header) ValidTime() (time.Time, bool) {
	if h.HasEndOfOverallTimeInterval {
		return h.EndOfOverallTimeInterval, true
	}
	return addGRIBTime(h.ReferenceTime, h.UnitOfTimeRange, h.ForecastTime)
}

// HybridCoordinateValues returns the optional Section 4 vertical-coordinate
// values, typically the A/B coefficients for ECMWF hybrid model levels.
func (m *Message) HybridCoordinateValues(dst []float64) ([]float64, error) {
	return m.S4.CoordinateValues(dst)
}

func scaledUnsigned(scale int8, value uint32) float64 {
	if scale == -127 || value == 0xffffffff {
		return math.NaN()
	}
	return float64(value) * math.Pow10(-int(scale))
}

func scaledSigned(scale int8, value int32) float64 {
	if scale == -127 || value == -0x7fffffff {
		return math.NaN()
	}
	return float64(value) * math.Pow10(-int(scale))
}

func addGRIBTime(t time.Time, unit uint8, value int32) (time.Time, bool) {
	v := int64(value)
	switch unit {
	case 0:
		return time.Unix(t.Unix()+v*60, int64(t.Nanosecond())).UTC(), true
	case 1:
		return time.Unix(t.Unix()+v*3600, int64(t.Nanosecond())).UTC(), true
	case 2:
		return t.AddDate(0, 0, int(value)), true
	case 3:
		return t.AddDate(0, int(value), 0), true
	case 4:
		return t.AddDate(int(value), 0, 0), true
	case 5:
		return t.AddDate(10*int(value), 0, 0), true
	case 6:
		return t.AddDate(30*int(value), 0, 0), true
	case 7:
		return t.AddDate(100*int(value), 0, 0), true
	case 10:
		return time.Unix(t.Unix()+v*3*3600, int64(t.Nanosecond())).UTC(), true
	case 11:
		return time.Unix(t.Unix()+v*6*3600, int64(t.Nanosecond())).UTC(), true
	case 12:
		return time.Unix(t.Unix()+v*12*3600, int64(t.Nanosecond())).UTC(), true
	case 13:
		return time.Unix(t.Unix()+v, int64(t.Nanosecond())).UTC(), true
	case 14:
		return time.Unix(t.Unix()+v*15*60, int64(t.Nanosecond())).UTC(), true
	case 15:
		return time.Unix(t.Unix()+v*30*60, int64(t.Nanosecond())).UTC(), true
	}
	return time.Time{}, false
}

// Grid returns a strongly-typed grid descriptor for this message. The first
// call parses Section 3 and caches the result; subsequent calls return the
// same instance, which lets callers attach per-grid state (KD-tree for an
// unstructured grid, mutable max-distance, etc.) once and have it persist
// across renders.
//
// Supported templates: 3.0 (regular lat/lon), 3.1 (rotated lat/lon),
// 3.10 (Mercator), 3.20 (polar stereographic), 3.30 (Lambert conformal),
// 3.40–3.43 (Gaussian variants — regular and reduced), 3.50 (spherical
// harmonic coefficients, not geographically locatable), and 3.101 (general
// unstructured / icosahedral; per-cell coordinates must be attached
// separately via SetGridCoordinates before Locate returns useful results).
func (m *Message) Grid() (grid.Grid, error) {
	m.gridOnce.Do(func() { m.grid, m.gridErr = m.parseGrid() })
	return m.grid, m.gridErr
}

func (m *Message) parseGrid() (grid.Grid, error) {
	t := m.S3.Template()
	templateNumber := m.S3.TemplateNumber()
	minimum := 0
	switch templateNumber {
	case 0, 40:
		minimum = 58
	case 41:
		minimum = 70
	case 42:
		minimum = 70
	case 43:
		minimum = 82
	case 1:
		minimum = 70
	case 10:
		minimum = 58
	case 20:
		minimum = 51
	case 30:
		minimum = 67
	case 50:
		minimum = 14
	case 101:
		// Shape (1) + grid number (3) + reference number (1) + UUID (16).
		minimum = 21
	default:
		return nil, ErrUnsupportedGrid
	}
	if len(t) < minimum {
		return nil, ErrTruncated
	}
	switch templateNumber {
	case 0:
		return grid.ParseLatLon(t), nil
	case 1:
		return grid.ParseRotatedLatLon(t), nil
	case 10:
		return grid.ParseMercator(t), nil
	case 20:
		return grid.ParsePolar(t), nil
	case 30:
		return grid.ParseLambert(t), nil
	case 40:
		// Reduced Gaussian appends a per-row pl[] table after the template.
		// Template body for 3.40 occupies 58 bytes (template offsets 0..57);
		// the optional list (if any) follows.
		listOctets := int(m.S3.NumOctetsForList())
		var list []byte
		if listOctets > 0 {
			list = m.S3.OptionalList(58)
		}
		if bswap.U32(t, 16) == 0xffffffff {
			nj := uint64(bswap.U32(t, 20))
			required := nj * uint64(listOctets)
			if listOctets <= 0 || required > uint64(len(list)) {
				return nil, ErrTruncated
			}
		}
		return grid.ParseGaussian(t, list, listOctets), nil
	case 41:
		listOctets := int(m.S3.NumOctetsForList())
		var list []byte
		if listOctets > 0 {
			list = m.S3.OptionalList(70)
		}
		if bswap.U32(t, 16) == 0xffffffff {
			nj := uint64(bswap.U32(t, 20))
			required := nj * uint64(listOctets)
			if listOctets <= 0 || required > uint64(len(list)) {
				return nil, ErrTruncated
			}
		}
		return grid.ParseRotatedGaussian(t, list, listOctets), nil
	case 42:
		listOctets := int(m.S3.NumOctetsForList())
		var list []byte
		if listOctets > 0 {
			list = m.S3.OptionalList(70)
		}
		if bswap.U32(t, 16) == 0xffffffff {
			nj := uint64(bswap.U32(t, 20))
			required := nj * uint64(listOctets)
			if listOctets <= 0 || required > uint64(len(list)) {
				return nil, ErrTruncated
			}
		}
		return grid.ParseStretchedGaussian(t, list, listOctets), nil
	case 43:
		listOctets := int(m.S3.NumOctetsForList())
		var list []byte
		if listOctets > 0 {
			list = m.S3.OptionalList(82)
		}
		if bswap.U32(t, 16) == 0xffffffff {
			nj := uint64(bswap.U32(t, 20))
			required := nj * uint64(listOctets)
			if listOctets <= 0 || required > uint64(len(list)) {
				return nil, ErrTruncated
			}
		}
		return grid.ParseStretchedRotatedGaussian(t, list, listOctets), nil
	case 101:
		return grid.ParseUnstructured(t, int(m.S3.NumDataPoints())), nil
	case 50:
		return grid.ParseSpectral(t, int(m.S3.NumDataPoints())), nil
	default:
		return nil, ErrUnsupportedGrid
	}
}

// SetGridCoordinates attaches per-cell (lat, lon) arrays to the message's
// unstructured grid (template 3.101). Without this call, Locate returns
// out-of-bounds for every query and tile renders are uniformly NaN. The
// arrays come from the matching horizontal_constants companion file shipped
// with the model — typically two messages whose data values *are* the cell
// latitudes and longitudes in degrees.
//
// Returns ErrUnsupportedGrid if this message is not on an unstructured grid.
// Returns an error from grid.Unstructured.SetCoordinates if the lengths do
// not match the number of cells, or if coordinates were already set.
func (m *Message) SetGridCoordinates(lats, lons []float64) error {
	g, err := m.Grid()
	if err != nil {
		return err
	}
	u, ok := g.(*grid.Unstructured)
	if !ok {
		return ErrUnsupportedGrid
	}
	return u.SetCoordinates(lats, lons)
}
