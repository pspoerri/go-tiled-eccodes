package section

import (
	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// Section3 — Grid Definition.
//
//	octets 1-4   section length
//	octet  5     section number (=3)
//	octet  6     source of grid definition
//	octets 7-10  number of data points
//	octet  11    octets for optional list of numbers (reduced grids)
//	octet  12    interpretation of list of numbers
//	octets 13-14 grid definition template number
//	octets 15..  grid definition template
//	then optional list of numbers (e.g. pl[] for reduced Gaussian)
type Section3 struct {
	Raw []byte
}

func (s Section3) Source() uint8           { return s.Raw[5] }
func (s Section3) NumDataPoints() uint32   { return bswap.U32(s.Raw, 6) }
func (s Section3) NumOctetsForList() uint8 { return s.Raw[10] }
func (s Section3) ListInterpretation() uint8 {
	return s.Raw[11]
}
func (s Section3) TemplateNumber() uint16 { return bswap.U16(s.Raw, 12) }

// Template returns the bytes of the grid definition template (not including
// the optional list of numbers that follows).
func (s Section3) Template() []byte {
	listOctets := int(s.NumOctetsForList()) * int(s.NumDataPoints()) // worst case overshoot
	_ = listOctets
	// The template starts at byte 14 and runs until the optional list region
	// begins. The optional list length is determined by the data structure of
	// the grid (reduced Gaussian provides a list of Nj uint16s, etc.) — we
	// compute it template-by-template, so here we just return everything from
	// byte 14 onward and let template-specific code slice further.
	return s.Raw[14:]
}

// OptionalList returns the trailing list-of-numbers bytes (e.g. the per-row
// pl[] table for reduced Gaussian), or nil if absent. tmplBytes is the number
// of bytes consumed by the grid-definition template body, computed by the
// template parser.
func (s Section3) OptionalList(tmplBytes int) []byte {
	start := 14 + tmplBytes
	if start >= len(s.Raw) {
		return nil
	}
	return s.Raw[start:]
}
