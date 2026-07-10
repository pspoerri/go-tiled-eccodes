package section

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestSection0(t *testing.T) {
	raw := make([]byte, 16)
	copy(raw, "GRIB")
	raw[6] = 7 // discipline = oceanographic
	raw[7] = 2 // edition
	binary.BigEndian.PutUint64(raw[8:], 12345)
	s := Section0{Raw: raw}
	if s.Magic() != "GRIB" {
		t.Errorf("Magic = %q, want GRIB", s.Magic())
	}
	if s.Discipline() != 7 {
		t.Errorf("Discipline = %d, want 7", s.Discipline())
	}
	if s.Edition() != 2 {
		t.Errorf("Edition = %d, want 2", s.Edition())
	}
	if s.TotalLength() != 12345 {
		t.Errorf("TotalLength = %d, want 12345", s.TotalLength())
	}
}

func TestSection1Accessors(t *testing.T) {
	raw := make([]byte, 21)
	binary.BigEndian.PutUint16(raw[5:], 78)  // centre
	binary.BigEndian.PutUint16(raw[7:], 255) // sub-centre
	raw[9] = 27                              // master tables version
	raw[10] = 0                              // local tables version
	raw[11] = 1                              // significance of ref time
	binary.BigEndian.PutUint16(raw[12:], 2026)
	raw[14] = 5  // month
	raw[15] = 8  // day
	raw[16] = 12 // hour
	raw[17] = 30 // minute
	raw[18] = 45 // second
	raw[19] = 0  // production status
	raw[20] = 1  // type of processed data
	s := Section1{Raw: raw}

	if s.Centre() != 78 {
		t.Errorf("Centre = %d", s.Centre())
	}
	if s.SubCentre() != 255 {
		t.Errorf("SubCentre = %d", s.SubCentre())
	}
	if s.MasterTablesVersion() != 27 {
		t.Errorf("MasterTablesVersion = %d", s.MasterTablesVersion())
	}
	if s.LocalTablesVersion() != 0 {
		t.Errorf("LocalTablesVersion = %d", s.LocalTablesVersion())
	}
	if s.SignificanceOfRefTime() != 1 {
		t.Errorf("SignificanceOfRefTime = %d", s.SignificanceOfRefTime())
	}
	if s.ProductionStatus() != 0 {
		t.Errorf("ProductionStatus = %d", s.ProductionStatus())
	}
	if s.TypeOfProcessedData() != 1 {
		t.Errorf("TypeOfProcessedData = %d", s.TypeOfProcessedData())
	}
	want := time.Date(2026, 5, 8, 12, 30, 45, 0, time.UTC)
	if got := s.ReferenceTime(); !got.Equal(want) {
		t.Errorf("ReferenceTime = %v, want %v", got, want)
	}
}

func TestSection3Accessors(t *testing.T) {
	// Build a Section 3 with template number 0 and 100 data points and a
	// trailing optional list.
	raw := make([]byte, 14+8+6) // 8-byte template + 6-byte optional list
	raw[5] = 0                  // source
	binary.BigEndian.PutUint32(raw[6:], 100)
	raw[10] = 2 // octets-for-list
	raw[11] = 1 // list interpretation
	binary.BigEndian.PutUint16(raw[12:], 40)
	for i := 14; i < 14+8; i++ {
		raw[i] = byte(i - 14) // template body
	}
	for i := 14 + 8; i < len(raw); i++ {
		raw[i] = 0xab
	}

	s := Section3{Raw: raw}
	if s.Source() != 0 {
		t.Errorf("Source = %d", s.Source())
	}
	if s.NumDataPoints() != 100 {
		t.Errorf("NumDataPoints = %d", s.NumDataPoints())
	}
	if s.NumOctetsForList() != 2 {
		t.Errorf("NumOctetsForList = %d", s.NumOctetsForList())
	}
	if s.ListInterpretation() != 1 {
		t.Errorf("ListInterpretation = %d", s.ListInterpretation())
	}
	if s.TemplateNumber() != 40 {
		t.Errorf("TemplateNumber = %d", s.TemplateNumber())
	}
	if got := s.Template(); len(got) != 14 || got[0] != 0 || got[7] != 7 {
		t.Errorf("Template length/contents wrong: %v", got)
	}
	if got := s.OptionalList(8); len(got) != 6 || got[0] != 0xab {
		t.Errorf("OptionalList = %v, want 6 bytes of 0xab", got)
	}
	if got := s.OptionalList(100); got != nil {
		t.Errorf("OptionalList past end should be nil, got %v", got)
	}
}

func TestSection4Accessors(t *testing.T) {
	// Template 4.0 needs at least 9 + 25 bytes.
	raw := make([]byte, 9+25)
	binary.BigEndian.PutUint16(raw[5:], 0) // num coords
	binary.BigEndian.PutUint16(raw[7:], 0) // template 4.0
	t4 := raw[9:]
	t4[0] = 0 // category: temperature
	t4[1] = 0 // number: air temp
	t4[2] = 2 // generating process: forecast
	t4[8] = 1 // unit of time: hours
	binary.BigEndian.PutUint32(t4[9:], 6)
	t4[13] = 103 // surface: height above ground
	t4[14] = 0   // scale factor
	binary.BigEndian.PutUint32(t4[15:], 2)

	s := Section4{Raw: raw}
	if s.NumCoords() != 0 {
		t.Errorf("NumCoords = %d", s.NumCoords())
	}
	if s.TemplateNumber() != 0 {
		t.Errorf("TemplateNumber = %d", s.TemplateNumber())
	}
	if s.ParameterCategory() != 0 || s.ParameterNumber() != 0 {
		t.Errorf("Param = (%d,%d)", s.ParameterCategory(), s.ParameterNumber())
	}
	if s.TypeOfGeneratingProcess() != 2 {
		t.Errorf("TypeOfGeneratingProcess = %d", s.TypeOfGeneratingProcess())
	}
	if s.IndicatorOfUnitOfTimeRange() != 1 {
		t.Errorf("UnitOfTimeRange = %d", s.IndicatorOfUnitOfTimeRange())
	}
	if s.ForecastTime() != 6 {
		t.Errorf("ForecastTime = %d", s.ForecastTime())
	}
	if s.TypeOfFirstFixedSurface() != 103 {
		t.Errorf("Surface = %d", s.TypeOfFirstFixedSurface())
	}
	if s.ScaleFactorOfFirstFixedSurface() != 0 {
		t.Errorf("ScaleFactor = %d", s.ScaleFactorOfFirstFixedSurface())
	}
	if s.ScaledValueOfFirstFixedSurface() != 2 {
		t.Errorf("ScaledValue = %d", s.ScaledValueOfFirstFixedSurface())
	}
	if got := s.Template(); len(got) != 25 {
		t.Errorf("Template len = %d, want 25", len(got))
	}

	// Truncated section: accessors past the cutoff return safe defaults.
	short := Section4{Raw: make([]byte, 9)}
	if got := short.IndicatorOfUnitOfTimeRange(); got != 0 {
		t.Errorf("short UnitOfTimeRange = %d, want 0", got)
	}
	if got := short.ForecastTime(); got != 0 {
		if got := short.ParameterCategory(); got != 255 {
			t.Errorf("short ParameterCategory = %d, want 255", got)
		}
		if got := short.ParameterNumber(); got != 255 {
			t.Errorf("short ParameterNumber = %d, want 255", got)
		}
		t.Errorf("short ForecastTime = %d, want 0", got)
	}
	if got := short.TypeOfFirstFixedSurface(); got != 255 {
		t.Errorf("short Surface = %d, want 255 (missing)", got)
	}
	if got := short.ScaleFactorOfFirstFixedSurface(); got != 0 {
		t.Errorf("short ScaleFactor = %d, want 0", got)
	}
	if got := short.ScaledValueOfFirstFixedSurface(); got != 0 {
		t.Errorf("short ScaledValue = %d, want 0", got)
	}
}

func TestSection4PerturbationNumber(t *testing.T) {
	// PDS template 4.1 (Individual ensemble forecast): 9-byte section
	// header + 34-byte 4.0-compatible prefix + 3-byte ensemble triplet
	// at octets 35..37.
	raw := make([]byte, 9+34+3)
	binary.BigEndian.PutUint16(raw[7:], 1) // template 4.1
	raw[34] = 3                            // type: positively perturbed
	raw[35] = 7                            // perturbation number 7
	raw[36] = 40                           // total ensemble members

	s := Section4{Raw: raw}
	if got := s.TypeOfEnsembleForecast(); got != 3 {
		t.Errorf("TypeOfEnsembleForecast = %d, want 3", got)
	}
	if got := s.PerturbationNumber(); got != 7 {
		t.Errorf("PerturbationNumber = %d, want 7", got)
	}

	// Non-ensemble template (4.0): the same offsets carry data we must
	// not interpret as a perturbation. PerturbationNumber should report
	// 0 (the documented default) regardless of byte 35's contents.
	rawDet := make([]byte, 9+34+3)
	binary.BigEndian.PutUint16(rawDet[7:], 0) // template 4.0
	rawDet[35] = 99                           // garbage that mustn't leak through
	if got := (Section4{Raw: rawDet}).PerturbationNumber(); got != 0 {
		t.Errorf("PerturbationNumber on non-ensemble template = %d, want 0", got)
	}
	if got := (Section4{Raw: rawDet}).TypeOfEnsembleForecast(); got != 255 {
		t.Errorf("TypeOfEnsembleForecast on non-ensemble template = %d, want 255", got)
	}

	// Truncated ensemble template: defensive default, no panic.
	short := Section4{Raw: make([]byte, 9+30)}
	binary.BigEndian.PutUint16(short.Raw[7:], 1) // claims template 4.1
	if got := short.PerturbationNumber(); got != 0 {
		t.Errorf("short PerturbationNumber = %d, want 0", got)
	}
}

func TestSection5Accessors(t *testing.T) {
	raw := make([]byte, 11+10)
	binary.BigEndian.PutUint32(raw[5:], 42) // num data points
	binary.BigEndian.PutUint16(raw[9:], 41) // template 5.41
	for i := 11; i < len(raw); i++ {
		raw[i] = byte(i)
	}
	s := Section5{Raw: raw}
	if s.NumDataPoints() != 42 {
		t.Errorf("NumDataPoints = %d", s.NumDataPoints())
	}
	if s.TemplateNumber() != 41 {
		t.Errorf("TemplateNumber = %d", s.TemplateNumber())
	}
	if got := s.Template(); len(got) != 10 {
		t.Errorf("Template len = %d, want 10", len(got))
	}
}

func TestSection6Accessors(t *testing.T) {
	// No bitmap: indicator 255, no trailing bytes.
	raw := []byte{0, 0, 0, 6, 6, 255}
	s := Section6{Raw: raw}
	if s.Indicator() != 255 {
		t.Errorf("Indicator = %d, want 255", s.Indicator())
	}
	if s.Bits() != nil {
		t.Errorf("Bits should be nil when no bitmap present")
	}

	// Bitmap present.
	raw = []byte{0, 0, 0, 9, 6, 0, 0xff, 0x80, 0x00}
	s = Section6{Raw: raw}
	if s.Indicator() != 0 {
		t.Errorf("Indicator = %d, want 0", s.Indicator())
	}
	got := s.Bits()
	if len(got) != 3 || got[0] != 0xff {
		t.Errorf("Bits = %v", got)
	}
}

func TestSection7Accessors(t *testing.T) {
	raw := []byte{0, 0, 0, 9, 7, 1, 2, 3, 4}
	s := Section7{Raw: raw}
	got := s.Payload()
	if len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Errorf("Payload = %v, want [1 2 3 4]", got)
	}
}

func TestSection4SignedValues(t *testing.T) {
	raw := make([]byte, 37)
	binary.BigEndian.PutUint16(raw[7:], 1)
	binary.BigEndian.PutUint32(raw[18:], 0x80000001)
	raw[23] = 0x82
	s := Section4{Raw: raw}
	if got := s.ForecastTime(); got != -1 {
		t.Errorf("ForecastTime = %d, want -1", got)
	}
	if got := s.ScaleFactorOfFirstFixedSurface(); got != -2 {
		t.Errorf("surface scale factor = %d, want -2", got)
	}
}

func TestSection4EnsembleOffsets(t *testing.T) {
	tests := []struct {
		name     string
		template uint16
		offset   int
		nbands   byte
	}{
		{name: "individual", template: 1, offset: 34},
		{name: "interval", template: 11, offset: 34},
		{name: "chemical", template: 41, offset: 36},
		{name: "aerosol", template: 45, offset: 47},
		{name: "satellite-two-bands", template: 33, offset: 45, nbands: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := make([]byte, tc.offset+3)
			binary.BigEndian.PutUint16(raw[7:], tc.template)
			if tc.nbands != 0 {
				raw[22] = tc.nbands
			}
			raw[tc.offset] = 3
			raw[tc.offset+1] = 17
			raw[tc.offset+2] = 51
			s := Section4{Raw: raw}
			if got := s.TypeOfEnsembleForecast(); got != 3 {
				t.Errorf("type = %d, want 3", got)
			}
			if got := s.PerturbationNumber(); got != 17 {
				t.Errorf("perturbation = %d, want 17", got)
			}
			if got := s.NumberOfForecastsInEnsemble(); got != 51 {
				t.Errorf("ensemble size = %d, want 51", got)
			}
		})
	}
}

func TestSection4DerivedForecastIsNotIndividualMember(t *testing.T) {
	raw := make([]byte, 37)
	binary.BigEndian.PutUint16(raw[7:], 2)
	raw[34] = 4
	raw[35] = 51
	s := Section4{Raw: raw}
	if got := s.TypeOfEnsembleForecast(); got != 255 {
		t.Errorf("derived forecast ensemble type = %d, want 255", got)
	}
	if got := s.PerturbationNumber(); got != 0 {
		t.Errorf("derived forecast perturbation = %d, want 0", got)
	}
	if got := s.NumberOfForecastsInEnsemble(); got != 0 {
		t.Errorf("derived forecast ensemble size = %d, want 0", got)
	}
}
