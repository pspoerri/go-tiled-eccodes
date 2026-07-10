package section

import "github.com/pspoerri/go-tiled-eccodes/internal/bswap"

// Section4 — Product Definition.
//
//	octets 1-4   section length
//	octet  5     section number (=4)
//	octets 6-7   number of coordinate values after template
//	octets 8-9   product definition template number
//	octets 10..  template
type Section4 struct {
	Raw []byte
}

func (s Section4) NumCoords() uint16      { return bswap.U16(s.Raw, 5) }
func (s Section4) TemplateNumber() uint16 { return bswap.U16(s.Raw, 7) }
func (s Section4) Template() []byte       { return s.Raw[9:] }

// Common Product Definition Template fields (templates 4.0..4.15 share the
// same first ~25 octets — parameter category, parameter number, generating
// process, hours/minutes of forecast offset, surface type/values).

// ParameterCategory (octet 10 of section 4 = template byte 0).
func (s Section4) ParameterCategory() uint8 { return s.ProductDefinition().ParameterCategory }

// ParameterNumber (octet 11 = template byte 1).
func (s Section4) ParameterNumber() uint8 { return s.ProductDefinition().ParameterNumber }

// TypeOfGeneratingProcess (octet 12 = template byte 2).
func (s Section4) TypeOfGeneratingProcess() uint8 {
	return s.ProductDefinition().TypeOfGeneratingProcess
}

// IndicatorOfUnitOfTimeRange (octet 18 = template byte 8).
func (s Section4) IndicatorOfUnitOfTimeRange() uint8 { return s.ProductDefinition().UnitOfForecastTime }

// ForecastTime (octets 19-22 = template bytes 9-12, signed
// sign-magnitude int32 BE).
func (s Section4) ForecastTime() int32 { return s.ProductDefinition().ForecastTime }

// TypeOfFirstFixedSurface (octet 23 = template byte 13).
func (s Section4) TypeOfFirstFixedSurface() uint8 {
	return s.ProductDefinition().TypeOfFirstFixedSurface
}

// ScaleFactorOfFirstFixedSurface (octet 24 = template byte 14, signed).
func (s Section4) ScaleFactorOfFirstFixedSurface() int8 {
	return s.ProductDefinition().ScaleFactorFirstSurface
}

// ScaledValueOfFirstFixedSurface (octets 25-28 = template bytes 15-18).
func (s Section4) ScaledValueOfFirstFixedSurface() uint32 {
	return s.ProductDefinition().ScaledValueFirstSurface
}

// ensembleOffset returns the raw offset of the ensemble triplet. Its
// position varies across product definition templates.
func (s Section4) ensembleOffset() (int, bool) {
	switch s.TemplateNumber() {
	case 1, 11, 60, 61:
		return 34, true
	case 41, 43:
		return 36, true
	case 45, 47:
		return 47, true
	case 33, 34:
		if len(s.Raw) <= 22 || s.Raw[22] == 0 {
			return 0, false
		}
		return 34 + 11*(int(s.Raw[22])-1), true
	}
	return 0, false
}

// PerturbationNumber returns the ensemble member number from any supported
// individual-ensemble product definition template.
// 0 is the control member by convention; 1..N are the perturbed members.
// For non-ensemble templates the field doesn't exist; the method returns 0.
func (s Section4) PerturbationNumber() uint8 {
	off, ok := s.ensembleOffset()
	if !ok || len(s.Raw) <= off+1 {
		return 0
	}
	return s.Raw[off+1]
}

// TypeOfEnsembleForecast classifies the
// ensemble member: 0 = unperturbed high-resolution control, 1 = unperturbed
// low-resolution control, 2 = negatively perturbed, 3 = positively perturbed,
// 4 = multi-model. 255 (missing) for non-ensemble templates.
func (s Section4) TypeOfEnsembleForecast() uint8 {
	off, ok := s.ensembleOffset()
	if !ok || len(s.Raw) <= off {
		return 255
	}
	return s.Raw[off]
}

// NumberOfForecastsInEnsemble returns the ensemble size for individual
// ensemble templates and 0 for deterministic or derived products.
func (s Section4) NumberOfForecastsInEnsemble() uint8 {
	off, ok := s.ensembleOffset()
	if !ok || len(s.Raw) <= off+2 {
		return 0
	}
	return s.Raw[off+2]
}
