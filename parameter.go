package grib

// ParameterKey contains the complete GRIB2 identity needed to resolve a
// parameter through WMO or originating-centre local concept tables.
type ParameterKey struct {
	Centre              uint16
	SubCentre           uint16
	MasterTablesVersion uint8
	LocalTablesVersion  uint8
	Discipline          uint8
	Category            uint8
	Number              uint8
	ProductTemplate     uint16
}

// Parameter describes a resolved parameter name. ID is resolver-defined (for
// example, an ecCodes paramId); zero is valid.
type Parameter struct {
	ID        int64
	Name      string
	ShortName string
	Units     string
}

// ParameterResolver is implemented by applications that load WMO/ecCodes or
// centre-local parameter tables.
type ParameterResolver interface {
	ResolveParameter(ParameterKey) (Parameter, bool)
}

// ParameterKey returns the raw lookup identity without consulting any tables.
func (m *Message) ParameterKey() ParameterKey {
	return ParameterKey{
		Centre:              m.S1.Centre(),
		SubCentre:           m.S1.SubCentre(),
		MasterTablesVersion: m.S1.MasterTablesVersion(),
		LocalTablesVersion:  m.S1.LocalTablesVersion(),
		Discipline:          m.S0.Discipline(),
		Category:            m.S4.ParameterCategory(),
		Number:              m.S4.ParameterNumber(),
		ProductTemplate:     m.S4.TemplateNumber(),
	}
}

// ResolveParameter resolves this message through a caller-supplied table.
func (m *Message) ResolveParameter(resolver ParameterResolver) (Parameter, bool) {
	if resolver == nil {
		return Parameter{}, false
	}
	return resolver.ResolveParameter(m.ParameterKey())
}
