package grib

import (
	"math"
	"time"

	"github.com/pspoerri/go-tiled-eccodes/grid"
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
}

// Header returns a snapshot of the message's identifying metadata.
func (m *Message) Header() Header {
	h := Header{
		Discipline:              m.S0.Discipline(),
		Centre:                  m.S1.Centre(),
		SubCentre:               m.S1.SubCentre(),
		ReferenceTime:           m.S1.ReferenceTime(),
		ProductionStatus:        m.S1.ProductionStatus(),
		GridTemplate:            m.S3.TemplateNumber(),
		DataTemplate:            m.S5.TemplateNumber(),
		NumDataPoints:           int(m.S3.NumDataPoints()),
		ParameterCategory:       m.S4.ParameterCategory(),
		ParameterNumber:         m.S4.ParameterNumber(),
		ForecastTime:            m.S4.ForecastTime(),
		UnitOfTimeRange:         m.S4.IndicatorOfUnitOfTimeRange(),
		TypeOfFirstFixedSurface: m.S4.TypeOfFirstFixedSurface(),
		ScaleFactorFirstSurface: m.S4.ScaleFactorOfFirstFixedSurface(),
		ScaledValueFirstSurface: m.S4.ScaledValueOfFirstFixedSurface(),
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
	if h.ScaledValueFirstSurface == 0xffffffff {
		return math.NaN()
	}
	return float64(h.ScaledValueFirstSurface) * math.Pow10(-int(h.ScaleFactorFirstSurface))
}

// Grid returns a strongly-typed grid descriptor for this message. The first
// call parses Section 3 and caches the result; subsequent calls return the
// same instance, which lets callers attach per-grid state (KD-tree for an
// unstructured grid, mutable max-distance, etc.) once and have it persist
// across renders.
//
// Supported templates: 3.0 (regular lat/lon), 3.1 (rotated lat/lon),
// 3.10 (Mercator), 3.20 (polar stereographic), 3.30 (Lambert conformal),
// 3.40 (Gaussian — regular and reduced), 3.101 (general unstructured /
// icosahedral; per-cell coordinates must be attached separately via
// SetGridCoordinates before Locate returns useful results).
func (m *Message) Grid() (grid.Grid, error) {
	m.gridOnce.Do(func() { m.grid, m.gridErr = m.parseGrid() })
	return m.grid, m.gridErr
}

func (m *Message) parseGrid() (grid.Grid, error) {
	t := m.S3.Template()
	switch m.S3.TemplateNumber() {
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
		return grid.ParseGaussian(t, list, listOctets), nil
	case 101:
		return grid.ParseUnstructured(t, int(m.S3.NumDataPoints())), nil
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
