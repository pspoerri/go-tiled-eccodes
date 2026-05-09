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
func (s Section4) ParameterCategory() uint8 { return s.Raw[9] }

// ParameterNumber (octet 11 = template byte 1).
func (s Section4) ParameterNumber() uint8 { return s.Raw[10] }

// TypeOfGeneratingProcess (octet 12 = template byte 2).
func (s Section4) TypeOfGeneratingProcess() uint8 { return s.Raw[11] }

// IndicatorOfUnitOfTimeRange (octet 18 = template byte 8).
func (s Section4) IndicatorOfUnitOfTimeRange() uint8 {
	if len(s.Raw) <= 17 {
		return 0
	}
	return s.Raw[17]
}

// ForecastTime (octets 19-22 = template bytes 9-12, int32 BE).
func (s Section4) ForecastTime() int32 {
	if len(s.Raw) < 22 {
		return 0
	}
	return int32(bswap.U32(s.Raw, 18))
}

// TypeOfFirstFixedSurface (octet 23 = template byte 13).
func (s Section4) TypeOfFirstFixedSurface() uint8 {
	if len(s.Raw) <= 22 {
		return 255
	}
	return s.Raw[22]
}

// ScaleFactorOfFirstFixedSurface (octet 24 = template byte 14, signed).
func (s Section4) ScaleFactorOfFirstFixedSurface() int8 {
	if len(s.Raw) <= 23 {
		return 0
	}
	return int8(s.Raw[23])
}

// ScaledValueOfFirstFixedSurface (octets 25-28 = template bytes 15-18).
func (s Section4) ScaledValueOfFirstFixedSurface() uint32 {
	if len(s.Raw) < 28 {
		return 0
	}
	return bswap.U32(s.Raw, 24)
}

// hasEnsembleFields reports whether the PDS template carries the
// 3-octet ensemble triplet (type, perturbation #, total #) at octets
// 35-37, immediately after the surface fields shared with template 4.0.
// Used by PerturbationNumber and friends to bail safely on non-ensemble
// templates rather than reading garbage.
func (s Section4) hasEnsembleFields() bool {
	switch s.TemplateNumber() {
	case 1, 2, 3, 4, 11, 12, 13, 14, 41, 43, 45, 47, 60, 61:
		return true
	}
	return false
}

// PerturbationNumber returns the ensemble member number (octet 36 = template
// byte 27) for ensemble PDS templates (4.1, 4.11, 4.41, 4.61, etc.).
// 0 is the control member by convention; 1..N are the perturbed members.
// For non-ensemble templates the field doesn't exist; the method returns 0.
func (s Section4) PerturbationNumber() uint8 {
	if !s.hasEnsembleFields() || len(s.Raw) <= 35 {
		return 0
	}
	return s.Raw[35]
}

// TypeOfEnsembleForecast (octet 35 = template byte 26) classifies the
// ensemble member: 0 = unperturbed high-resolution control, 1 = unperturbed
// low-resolution control, 2 = negatively perturbed, 3 = positively perturbed,
// 4 = multi-model. 255 (missing) for non-ensemble templates.
func (s Section4) TypeOfEnsembleForecast() uint8 {
	if !s.hasEnsembleFields() || len(s.Raw) <= 34 {
		return 255
	}
	return s.Raw[34]
}
